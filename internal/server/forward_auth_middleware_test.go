package server

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardAuthMiddleware_Success(t *testing.T) {
	// Create mock auth server that returns 200
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request received expected headers
		assert.Equal(t, "GET", r.Header.Get("X-Forwarded-Method"))
		assert.NotEmpty(t, r.Header.Get("X-Forwarded-Uri"))
		assert.NotEmpty(t, r.Header.Get("X-Forwarded-Host"))
		assert.NotEmpty(t, r.Header.Get("X-Forwarded-Proto"))
		assert.NotEmpty(t, r.Header.Get("X-Forwarded-For"))

		// Return auth headers to be copied
		w.Header().Set("Authorization", "Bearer token123")
		w.Header().Set("Remote-User", "testuser")
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create next handler that verifies auth headers were copied
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
		assert.Equal(t, "testuser", r.Header.Get("Remote-User"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Create middleware
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{"Authorization", "Remote-User"},
		AllowedHeaders: []string{"Cookie", "Authorization"},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("Cookie", "session=abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify
	assert.True(t, nextCalled, "Next handler should be called")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())
}

func TestForwardAuthMiddleware_Unauthorized(t *testing.T) {
	// Create mock auth server that returns 401
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"test\"")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer authServer.Close()

	// Create next handler
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{"Authorization"},
		AllowedHeaders: []string{"Cookie"},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify
	assert.False(t, nextCalled, "Next handler should not be called")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "Bearer realm=\"test\"", w.Header().Get("WWW-Authenticate"))
	assert.Equal(t, "unauthorized", w.Body.String())
}

func TestForwardAuthMiddleware_Forbidden(t *testing.T) {
	// Create mock auth server that returns 403
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer authServer.Close()

	// Create next handler
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify
	assert.False(t, nextCalled, "Next handler should not be called")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "forbidden", w.Body.String())
}

func TestForwardAuthMiddleware_Redirect(t *testing.T) {
	// Create mock auth server that returns redirect
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://auth.example.com/login")
		w.WriteHeader(http.StatusFound)
	}))
	defer authServer.Close()

	// Create next handler
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify
	assert.False(t, nextCalled, "Next handler should not be called")
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "http://auth.example.com/login", w.Header().Get("Location"))
}

func TestForwardAuthMiddleware_ForwardsAllowedHeaders(t *testing.T) {
	// Create mock auth server that checks headers
	receivedHeaders := make(http.Header)
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create next handler
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware with specific allowed headers
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{"Cookie", "Authorization", "User-Agent"},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request with various headers
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("User-Agent", "TestClient/1.0")
	req.Header.Set("X-Custom-Header", "should-not-forward")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify allowed headers were forwarded
	assert.Equal(t, "session=abc123", receivedHeaders.Get("Cookie"))
	assert.Equal(t, "Bearer token", receivedHeaders.Get("Authorization"))
	assert.Equal(t, "TestClient/1.0", receivedHeaders.Get("User-Agent"))
	// Verify disallowed header was not forwarded
	assert.Empty(t, receivedHeaders.Get("X-Custom-Header"))
}

func TestForwardAuthMiddleware_TrustForwardHeader(t *testing.T) {
	// Test with TrustForwardHeader = true
	t.Run("TrustEnabled", func(t *testing.T) {
		receivedHeaders := make(http.Header)
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		defer authServer.Close()

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		config := ForwardAuthConfig{
			URL:                authServer.URL,
			RequestTimeout:     5 * time.Second,
			CopyHeaders:        []string{},
			AllowedHeaders:     []string{},
			TrustForwardHeader: true,
		}
		handler := WithForwardAuthMiddleware(config, next)

		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Should preserve the existing X-Forwarded-For
		assert.Equal(t, "1.2.3.4, 5.6.7.8", receivedHeaders.Get("X-Forwarded-For"))
	})

	// Test with TrustForwardHeader = false
	t.Run("TrustDisabled", func(t *testing.T) {
		receivedHeaders := make(http.Header)
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		defer authServer.Close()

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		config := ForwardAuthConfig{
			URL:                authServer.URL,
			RequestTimeout:     5 * time.Second,
			CopyHeaders:        []string{},
			AllowedHeaders:     []string{},
			TrustForwardHeader: false,
		}
		handler := WithForwardAuthMiddleware(config, next)

		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Should NOT preserve the existing X-Forwarded-For, use client IP instead
		assert.NotEqual(t, "1.2.3.4, 5.6.7.8", receivedHeaders.Get("X-Forwarded-For"))
		assert.NotEmpty(t, receivedHeaders.Get("X-Forwarded-For"))
	})
}

func TestForwardAuthMiddleware_Timeout(t *testing.T) {
	// Create mock auth server that delays response
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create next handler
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware with very short timeout
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 10 * time.Millisecond,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify timeout results in error response
	assert.False(t, nextCalled, "Next handler should not be called on timeout")
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestForwardAuthMiddleware_NetworkError(t *testing.T) {
	// Create next handler
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware with invalid URL (connection will fail)
	config := ForwardAuthConfig{
		URL:            "http://localhost:1", // Port 1 should be unreachable
		RequestTimeout: 1 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify network error results in 502
	assert.False(t, nextCalled, "Next handler should not be called on network error")
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestForwardAuthMiddleware_CopiesMultipleHeaderValues(t *testing.T) {
	// Create mock auth server that returns multiple values for a header
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Remote-Groups", "admins")
		w.Header().Add("Remote-Groups", "developers")
		w.Header().Add("Remote-Groups", "users")
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create next handler that verifies multiple values
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		groups := r.Header.Values("Remote-Groups")
		assert.Len(t, groups, 3)
		assert.Contains(t, groups, "admins")
		assert.Contains(t, groups, "developers")
		assert.Contains(t, groups, "users")
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{"Remote-Groups"},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Make request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify
	assert.True(t, nextCalled, "Next handler should be called")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestForwardAuthMiddleware_PreservesRequestMethod(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			receivedMethod := ""
			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Header.Get("X-Forwarded-Method")
				w.WriteHeader(http.StatusOK)
			}))
			defer authServer.Close()

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			config := ForwardAuthConfig{
				URL:            authServer.URL,
				RequestTimeout: 5 * time.Second,
				CopyHeaders:    []string{},
				AllowedHeaders: []string{},
			}
			handler := WithForwardAuthMiddleware(config, next)

			req := httptest.NewRequest(method, "http://example.com/test", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, method, receivedMethod)
		})
	}
}

func TestForwardAuthMiddleware_DoesNotFollowRedirects(t *testing.T) {
	// Track how many times the auth server is called
	callCount := 0
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: redirect
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(http.StatusFound)
		} else {
			// Should never get here if redirects aren't followed
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer authServer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify only called once (redirect not followed)
	assert.Equal(t, 1, callCount)
	assert.Equal(t, http.StatusFound, w.Code)
}

func TestForwardAuthMiddleware_StripsHopByHopHeaders(t *testing.T) {
	// Create mock auth server that returns hop-by-hop headers
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("X-Custom-Header", "should-be-preserved")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authServer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify hop-by-hop headers were stripped
	assert.Empty(t, w.Header().Get("Connection"))
	assert.Empty(t, w.Header().Get("Keep-Alive"))
	assert.Empty(t, w.Header().Get("Transfer-Encoding"))
	assert.Empty(t, w.Header().Get("Upgrade"))
	// Verify custom header was preserved
	assert.Equal(t, "should-be-preserved", w.Header().Get("X-Custom-Header"))
}

func TestForwardAuthMiddleware_HandlesBodyInAuthResponse(t *testing.T) {
	expectedBody := "You are not authorized to access this resource"

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(expectedBody))
	}))
	defer authServer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Verify body was copied
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))

	body, err := io.ReadAll(w.Body)
	require.NoError(t, err)
	assert.Equal(t, expectedBody, string(body))
}

func TestForwardAuthMiddleware_HTTPSRequest(t *testing.T) {
	receivedProto := ""
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := ForwardAuthConfig{
		URL:            authServer.URL,
		RequestTimeout: 5 * time.Second,
		CopyHeaders:    []string{},
		AllowedHeaders: []string{},
	}
	handler := WithForwardAuthMiddleware(config, next)

	// Create HTTPS request (with TLS)
	req := httptest.NewRequest("GET", "https://example.com/test", nil)
	// Simulate TLS connection
	req.TLS = &tls.ConnectionState{}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "https", receivedProto)
}
