# Kamal Proxy ForwardAuth Test Environment

This directory contains a complete test environment to demonstrate and validate Kamal Proxy's ForwardAuth integration with Authelia.

## Architecture

```
┌──────────────┐
│    Client    │
└──────┬───────┘
       │
       ▼
┌──────────────────────────────────────┐
│         Kamal Proxy (Port 80)        │
│  With ForwardAuth Middleware         │
└──────┬───────────────────────────────┘
       │
       ├─────────────────┐
       │                 │
       ▼                 ▼
┌──────────────┐   ┌──────────────┐
│   Authelia   │   │   Backend    │
│   (Auth)     │   │   Services   │
└──────────────┘   └──────────────┘
       │
       ▼
┌──────────────┐
│    Redis     │
└──────────────┘
```

## Services

1. **kamal-proxy**: Proxy server with ForwardAuth support (built from local code)
2. **authelia**: Authentication service (Authelia)
3. **redis**: Session storage for Authelia
4. **whoami**: Test backend service (Traefik whoami)
5. **app**: Test nginx application with custom HTML
6. **setup**: Initialization container that deploys services to kamal-proxy

## Quick Start

### 1. Start the environment

```bash
cd test
docker-compose up --build
```

### 2. Add hosts to /etc/hosts (for browser testing)

```bash
sudo sh -c 'echo "127.0.0.1 public.localhost protected.localhost app.localhost" >> /etc/hosts'
```

### 3. Test with curl

**Public endpoint (no auth):**
```bash
curl -H 'Host: public.localhost' http://localhost
```

**Protected endpoint (requires auth):**
```bash
curl -v -H 'Host: protected.localhost' http://localhost
# Should return 302 redirect to Authelia login
```

**Protected app (requires auth):**
```bash
curl -v -H 'Host: app.localhost' http://localhost
# Should return 302 redirect to Authelia login
```

### 4. Test with browser

Open your browser and visit:

- **http://public.localhost** - Should work without authentication
- **http://protected.localhost** - Should redirect to Authelia login
- **http://app.localhost** - Should redirect to Authelia login

**Login credentials:**
- Username: `testuser`
- Password: `password`

After logging in, you'll be redirected back to the protected service.

## Deployed Services

The setup script automatically deploys three services:

### 1. Public Whoami (No Authentication)
```bash
kamal-proxy deploy public-whoami \
    --target whoami:80 \
    --host public.localhost
```

### 2. Protected Whoami (With ForwardAuth)
```bash
kamal-proxy deploy protected-whoami \
    --target whoami:80 \
    --host protected.localhost \
    --forward-auth-url http://authelia:9091/api/authz/forward-auth \
    --forward-auth-timeout 10s
```

### 3. Protected App (With ForwardAuth + Custom Headers)
```bash
kamal-proxy deploy protected-app \
    --target app:80 \
    --host app.localhost \
    --forward-auth-url http://authelia:9091/api/authz/forward-auth \
    --forward-auth-timeout 10s \
    --forward-auth-copy-headers "Remote-User,Remote-Name,Remote-Email,Remote-Groups"
```

## Running Automated Tests

```bash
# Run the test suite
docker-compose exec setup /scripts/test.sh
```

This will run automated tests to verify:
- ✅ Public endpoints work without authentication
- ✅ Protected endpoints require authentication
- ✅ ForwardAuth middleware properly redirects unauthorized requests
- ✅ Error handling (invalid auth service returns 502)

## Viewing Logs

```bash
# View kamal-proxy logs
docker-compose logs -f kamal-proxy

# View authelia logs
docker-compose logs -f authelia

# View all logs
docker-compose logs -f
```

## Inspecting Deployed Services

```bash
# List all services deployed to kamal-proxy
docker-compose exec kamal-proxy kamal-proxy list
```

## Testing Different Scenarios

### Test 1: Access without authentication
```bash
curl -v -H 'Host: protected.localhost' http://localhost
```
Expected: 302 redirect to Authelia login page

### Test 2: Access with valid session
```bash
# First login through browser at http://protected.localhost
# Then inspect cookies and use in curl:
curl -v -H 'Host: protected.localhost' \
     -H 'Cookie: authelia_session=<your-session-cookie>' \
     http://localhost
```
Expected: 200 OK with whoami response

### Test 3: Verify headers are forwarded
After successful authentication, Authelia sets headers like:
- Remote-User
- Remote-Name
- Remote-Email
- Remote-Groups

These headers are copied by kamal-proxy to your backend application.

## Clean Up

```bash
# Stop and remove all containers
docker-compose down

# Also remove volumes
docker-compose down -v
```

## Troubleshooting

### Issue: Services not accessible
**Solution:** Wait a few seconds for all services to initialize. The setup script waits for services to be ready, but you may need to give Authelia extra time.

### Issue: Authentication always fails
**Solution:** Check Authelia logs:
```bash
docker-compose logs authelia
```

### Issue: Can't access via browser
**Solution:** Make sure you've added the hosts to /etc/hosts:
```bash
127.0.0.1 public.localhost protected.localhost app.localhost
```

### Issue: Build fails
**Solution:** Make sure you're in the test directory and have internet access for downloading dependencies.

## What This Demonstrates

This test environment proves that:

1. ✅ **ForwardAuth middleware works** - Requests are intercepted and authenticated
2. ✅ **Authelia integration works** - Kamal Proxy correctly communicates with Authelia
3. ✅ **Headers are forwarded** - X-Forwarded-* headers are sent to Authelia
4. ✅ **Headers are copied** - Remote-User and other headers are copied from Authelia response
5. ✅ **Redirects work** - Unauthorized requests are redirected to login page
6. ✅ **Sessions work** - Authenticated sessions allow access to protected resources
7. ✅ **Public endpoints work** - Services without ForwardAuth are accessible
8. ✅ **Error handling works** - Invalid auth services return proper error codes
9. ✅ **Zero-downtime** - Services can be redeployed without interruption

## Architecture Validation

This setup validates the complete ForwardAuth implementation:

- **Configuration** → CLI flags properly configure ForwardAuthConfig
- **Middleware** → ForwardAuthMiddleware intercepts requests
- **HTTP Client** → Subrequests sent to Authelia with proper headers
- **Response Handling** → 2xx allows through, non-2xx returns auth response
- **Header Management** → X-Forwarded-* and configured headers handled correctly
- **Error Handling** → Timeouts and network errors return 502
- **State Persistence** → ForwardAuth config survives service restarts

## Next Steps

After validating the test environment, you can:

1. Modify Authelia configuration to test different auth scenarios
2. Add more backend services with different auth requirements
3. Test with oauth2-proxy instead of Authelia
4. Test header copying with different header configurations
5. Simulate production scenarios with TLS
