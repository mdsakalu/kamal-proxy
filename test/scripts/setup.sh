#!/bin/sh
set -e

echo "========================================"
echo "Kamal Proxy ForwardAuth Test Setup"
echo "========================================"
echo ""

# Wait for kamal-proxy to be ready
echo "Waiting for kamal-proxy to be ready..."
until nc -z kamal-proxy 80 2>/dev/null; do
    sleep 1
done
echo "✓ Kamal Proxy is ready"

# Wait for authelia to be ready
echo "Waiting for Authelia to be ready..."
until nc -z authelia 9091 2>/dev/null; do
    sleep 1
done
sleep 5  # Give Authelia extra time to fully initialize
echo "✓ Authelia is ready"

# Wait for backend services
echo "Waiting for backend services..."
until nc -z whoami 80 2>/dev/null; do
    sleep 1
done
until nc -z app 80 2>/dev/null; do
    sleep 1
done
echo "✓ Backend services are ready"
echo ""

# Deploy public whoami service (no auth)
echo "Deploying public whoami service (no authentication)..."
kamal-proxy deploy public-whoami \
    --target whoami:80 \
    --host public.localhost
echo "✓ Public whoami deployed at http://public.localhost"
echo ""

# Deploy protected whoami service (with auth)
echo "Deploying protected whoami service (with ForwardAuth)..."
kamal-proxy deploy protected-whoami \
    --target whoami:80 \
    --host protected.localhost \
    --forward-auth-url http://authelia:9091/api/authz/forward-auth \
    --forward-auth-timeout 10s
echo "✓ Protected whoami deployed at http://protected.localhost"
echo ""

# Deploy protected app (with auth)
echo "Deploying protected app (with ForwardAuth)..."
kamal-proxy deploy protected-app \
    --target app:80 \
    --host app.localhost \
    --forward-auth-url http://authelia:9091/api/authz/forward-auth \
    --forward-auth-timeout 10s \
    --forward-auth-copy-headers "Remote-User,Remote-Name,Remote-Email,Remote-Groups"
echo "✓ Protected app deployed at http://app.localhost"
echo ""

# List all deployed services
echo "========================================"
echo "Deployment Summary"
echo "========================================"
kamal-proxy list
echo ""

echo "========================================"
echo "Setup Complete!"
echo "========================================"
echo ""
echo "Test the deployment:"
echo ""
echo "1. Public endpoint (no auth required):"
echo "   curl -H 'Host: public.localhost' http://localhost"
echo ""
echo "2. Protected endpoint (auth required, should redirect):"
echo "   curl -v -H 'Host: protected.localhost' http://localhost"
echo ""
echo "3. Protected app (auth required, should redirect):"
echo "   curl -v -H 'Host: app.localhost' http://localhost"
echo ""
echo "For browser testing, add to /etc/hosts:"
echo "   127.0.0.1 public.localhost protected.localhost app.localhost"
echo ""
echo "Login credentials:"
echo "   Username: testuser"
echo "   Password: password"
echo ""
echo "========================================"

# Keep container running for manual testing
tail -f /dev/null
