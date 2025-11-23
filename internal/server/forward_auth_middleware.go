package server

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type ForwardAuthMiddleware struct {
	config     ForwardAuthConfig
	httpClient *http.Client
	next       http.Handler
}

func WithForwardAuthMiddleware(config ForwardAuthConfig, next http.Handler) http.Handler {
	return &ForwardAuthMiddleware{
		config: config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - let the auth service decide
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				MaxIdleConnsPerHost: MaxIdleConnsPerHost,
				DialContext: (&net.Dialer{
					Timeout:   config.RequestTimeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
		next: next,
	}
}

func (h *ForwardAuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create auth request
	authReq, err := http.NewRequest(http.MethodGet, h.config.URL, nil)
	if err != nil {
		slog.Error("Failed to create forward auth request", "error", err, "url", h.config.URL)
		SetErrorResponse(w, r, http.StatusInternalServerError, nil)
		return
	}

	// Copy original request context
	authReq = authReq.WithContext(r.Context())

	// Forward allowed headers from original request
	h.forwardHeaders(authReq, r)

	// Add X-Forwarded-* headers
	h.addForwardedHeaders(authReq, r)

	// Send auth request
	slog.Debug("Sending forward auth request", "url", h.config.URL, "original_path", r.URL.Path)
	authResp, err := h.httpClient.Do(authReq)
	if err != nil {
		slog.Error("Forward auth request failed", "error", err, "url", h.config.URL)
		SetErrorResponse(w, r, http.StatusBadGateway, nil)
		return
	}
	defer authResp.Body.Close()

	// Check auth response status
	if authResp.StatusCode >= 200 && authResp.StatusCode < 300 {
		// Auth successful - copy configured headers and continue
		h.copyAuthHeaders(r, authResp)
		slog.Debug("Forward auth succeeded", "status", authResp.StatusCode, "original_path", r.URL.Path)
		h.next.ServeHTTP(w, r)
		return
	}

	// Auth failed - return auth service response
	slog.Info("Forward auth denied", "status", authResp.StatusCode, "original_path", r.URL.Path)
	h.returnAuthResponse(w, authResp)
}

func (h *ForwardAuthMiddleware) forwardHeaders(authReq *http.Request, originalReq *http.Request) {
	allowedMap := make(map[string]bool)
	for _, header := range h.config.AllowedHeaders {
		allowedMap[http.CanonicalHeaderKey(header)] = true
	}

	for header, values := range originalReq.Header {
		if allowedMap[header] {
			for _, value := range values {
				authReq.Header.Add(header, value)
			}
		}
	}
}

func (h *ForwardAuthMiddleware) addForwardedHeaders(authReq *http.Request, originalReq *http.Request) {
	// Add X-Forwarded-Method
	authReq.Header.Set("X-Forwarded-Method", originalReq.Method)

	// Add X-Forwarded-Uri
	authReq.Header.Set("X-Forwarded-Uri", originalReq.RequestURI)

	// Add X-Forwarded-Host
	authReq.Header.Set("X-Forwarded-Host", originalReq.Host)

	// Add X-Forwarded-Proto
	proto := "http"
	if originalReq.TLS != nil {
		proto = "https"
	}
	authReq.Header.Set("X-Forwarded-Proto", proto)

	// Add or forward X-Forwarded-For
	if h.config.TrustForwardHeader {
		if prior := originalReq.Header.Get("X-Forwarded-For"); prior != "" {
			authReq.Header.Set("X-Forwarded-For", prior)
		}
	}
	if authReq.Header.Get("X-Forwarded-For") == "" {
		clientIP, _, err := net.SplitHostPort(originalReq.RemoteAddr)
		if err != nil {
			clientIP = originalReq.RemoteAddr
		}
		authReq.Header.Set("X-Forwarded-For", clientIP)
	}
}

func (h *ForwardAuthMiddleware) copyAuthHeaders(originalReq *http.Request, authResp *http.Response) {
	copyMap := make(map[string]bool)
	for _, header := range h.config.CopyHeaders {
		copyMap[http.CanonicalHeaderKey(header)] = true
	}

	for header, values := range authResp.Header {
		if copyMap[header] {
			// Remove existing header first
			originalReq.Header.Del(header)
			// Add all values from auth response
			for _, value := range values {
				originalReq.Header.Add(header, value)
			}
		}
	}
}

func (h *ForwardAuthMiddleware) returnAuthResponse(w http.ResponseWriter, authResp *http.Response) {
	// Copy headers from auth response
	for header, values := range authResp.Header {
		// Skip headers that are hop-by-hop
		headerLower := strings.ToLower(header)
		if headerLower == "connection" || headerLower == "keep-alive" ||
			headerLower == "proxy-authenticate" || headerLower == "proxy-authorization" ||
			headerLower == "te" || headerLower == "trailer" ||
			headerLower == "transfer-encoding" || headerLower == "upgrade" {
			continue
		}

		for _, value := range values {
			w.Header().Add(header, value)
		}
	}

	// Set status code
	w.WriteHeader(authResp.StatusCode)

	// Copy body
	io.Copy(w, authResp.Body)
}
