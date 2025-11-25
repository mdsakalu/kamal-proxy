# Testing Kamal Proxy ForwardAuth Integration

This document provides comprehensive testing instructions for the ForwardAuth feature.

## Prerequisites

- Docker and Docker Compose (or Docker with Compose plugin)
- curl (for command-line testing)
- A web browser (for interactive testing)
- Host file modification access (sudo/admin)

## Quick Test (5 minutes)

### 1. Start the Environment

```bash
cd test

# Using docker-compose
docker-compose up --build -d

# OR using docker compose (newer syntax)
docker compose up --build -d
```

Wait for all services to start (approximately 30-60 seconds).

### 2. Verify Services Are Running

```bash
# Check container status
docker-compose ps

# You should see:
# - kamal-proxy (running)
# - authelia (running)
# - redis (running)
# - whoami (running)
# - app (running)
# - setup (exited 0, after deploying services)
```

### 3. Run Automated Tests

```bash
docker-compose exec kamal-proxy /scripts/test.sh
```

Expected output:
```
✓ PASS: Public endpoint accessible without authentication
✓ PASS: Protected endpoint returns 302 (redirect or unauthorized)
✓ PASS: Protected app returns 302 (redirect or unauthorized)
✓ PASS: ForwardAuth middleware is active
All Tests Completed!
```

### 4. Manual curl Tests

```bash
# Test 1: Public endpoint (should return whoami data)
curl -H 'Host: public.localhost' http://localhost

# Test 2: Protected endpoint (should return 302 redirect)
curl -v -H 'Host: protected.localhost' http://localhost 2>&1 | grep "< HTTP"

# Test 3: Protected app (should return 302 redirect)
curl -v -H 'Host: app.localhost' http://localhost 2>&1 | grep "< HTTP"
```

### 5. Browser Testing

**Add to /etc/hosts:**
```bash
sudo sh -c 'echo "127.0.0.1 public.localhost protected.localhost app.localhost" >> /etc/hosts'
```

**Visit in browser:**
1. http://public.localhost - Should show whoami data immediately
2. http://protected.localhost - Should redirect to Authelia login
3. http://app.localhost - Should redirect to Authelia login

**Login credentials:**
- Username: `testuser`
- Password: `password`

After login, you should be redirected back and see the protected content.

## Detailed Test Scenarios

### Test 1: Verify ForwardAuth Middleware Active

```bash
# Protected endpoint without auth should redirect
RESPONSE=$(curl -s -i -H 'Host: protected.localhost' http://localhost)
echo "$RESPONSE" | grep "HTTP/1.1 302"
echo "$RESPONSE" | grep "Location:"

# Should contain redirect to Authelia
```

### Test 2: Verify Public Endpoints Work

```bash
# Public endpoint should work without auth
RESPONSE=$(curl -s -H 'Host: public.localhost' http://localhost)
echo "$RESPONSE" | grep "Hostname: whoami"

# Should show whoami data
```

### Test 3: Verify Headers Are Forwarded to Authelia

```bash
# Check authelia logs for X-Forwarded-* headers
docker-compose logs authelia | grep "X-Forwarded"

# You should see:
# - X-Forwarded-Method
# - X-Forwarded-Uri
# - X-Forwarded-Host
# - X-Forwarded-Proto
# - X-Forwarded-For
```

### Test 4: Verify Session-Based Access

```bash
# First, get a session by logging in through browser
# Then extract the session cookie and test:

curl -H 'Host: protected.localhost' \
     -H 'Cookie: authelia_session=YOUR_SESSION_COOKIE_HERE' \
     http://localhost

# Should return whoami data (200 OK)
```

### Test 5: Verify Header Copying

The protected app is configured to copy these headers from Authelia:
- Remote-User
- Remote-Name
- Remote-Email
- Remote-Groups

After authentication, these headers should be available to the backend application.

### Test 6: Verify Error Handling

```bash
# Deploy a service with invalid auth URL
docker-compose exec kamal-proxy kamal-proxy deploy test-error \
    --target whoami:80 \
    --host error.localhost \
    --forward-auth-url http://invalid:9999/auth \
    --forward-auth-timeout 2s

# Request should return 502 Bad Gateway
curl -s -o /dev/null -w "%{http_code}" -H 'Host: error.localhost' http://localhost

# Should print: 502
```

### Test 7: Verify Timeout Handling

```bash
# The setup already includes a reasonable timeout (10s for protected services)
# If Authelia takes too long, kamal-proxy should return 502

# You can test this by temporarily stopping Authelia:
docker-compose stop authelia

# Then try accessing protected endpoint:
curl -v -H 'Host: protected.localhost' http://localhost

# Should return 502 Bad Gateway after timeout

# Restart Authelia:
docker-compose start authelia
```

## Inspecting the Deployment

### View Deployed Services

```bash
docker-compose exec kamal-proxy kamal-proxy list
```

Expected output shows three services:
1. `public-whoami` - No ForwardAuth
2. `protected-whoami` - With ForwardAuth
3. `protected-app` - With ForwardAuth + custom headers

### View Kamal Proxy Logs

```bash
# Real-time logs
docker-compose logs -f kamal-proxy

# Look for:
# - "Using forward auth" when service starts
# - "Sending forward auth request" for each authenticated request
# - "Forward auth succeeded" or "Forward auth denied"
```

### View Authelia Logs

```bash
docker-compose logs -f authelia

# Look for:
# - Incoming auth requests from kamal-proxy
# - Session validation
# - Authentication decisions
```

## Advanced Testing

### Test Different Authentication States

```bash
# 1. No session (should redirect to login)
curl -v -H 'Host: protected.localhost' http://localhost

# 2. With valid session (should allow through)
# Get session cookie from browser after login, then:
curl -v -H 'Host: protected.localhost' \
     -H 'Cookie: authelia_session=VALID_SESSION' \
     http://localhost

# 3. With expired session (should redirect to login)
curl -v -H 'Host: protected.localhost' \
     -H 'Cookie: authelia_session=expired_or_invalid' \
     http://localhost
```

### Test Different HTTP Methods

```bash
# ForwardAuth should work for all HTTP methods
for METHOD in GET POST PUT DELETE PATCH; do
    echo "Testing $METHOD"
    curl -X $METHOD -s -o /dev/null -w "%{http_code}" \
         -H 'Host: protected.localhost' \
         http://localhost
    echo ""
done

# All should return 302 (redirect to login)
```

### Test Header Forwarding

```bash
# Create a simple test to verify headers are sent to Authelia
# Enable debug logging and check authelia logs:

docker-compose logs authelia | grep -A 10 "forward-auth"

# Should show X-Forwarded-Method, X-Forwarded-Uri, etc.
```

### Performance Testing

```bash
# Test multiple concurrent requests
for i in {1..100}; do
    curl -s -o /dev/null -H 'Host: protected.localhost' http://localhost &
done
wait

# Check kamal-proxy and authelia logs for any errors
docker-compose logs kamal-proxy | grep -i error
docker-compose logs authelia | grep -i error
```

## Troubleshooting

### Issue: Setup container exits with error

**Check logs:**
```bash
docker-compose logs setup
```

**Common causes:**
- kamal-proxy not ready yet (wait longer)
- Authelia not ready yet (wait longer)
- Network connectivity issues

**Solution:** Restart setup container:
```bash
docker-compose restart setup
```

### Issue: Authentication always fails

**Check Authelia status:**
```bash
docker-compose logs authelia | tail -50
```

**Check Redis:**
```bash
docker-compose logs redis
```

**Solution:** Restart Authelia and Redis:
```bash
docker-compose restart authelia redis
sleep 10
```

### Issue: Can't access via browser

**Verify hosts file:**
```bash
cat /etc/hosts | grep localhost
```

**Should contain:**
```
127.0.0.1 public.localhost protected.localhost app.localhost
```

**Re-add if missing:**
```bash
sudo sh -c 'echo "127.0.0.1 public.localhost protected.localhost app.localhost" >> /etc/hosts'
```

### Issue: Service deployment fails

**Check kamal-proxy status:**
```bash
docker-compose logs kamal-proxy
```

**Manually deploy:**
```bash
docker-compose exec kamal-proxy kamal-proxy deploy test \
    --target whoami:80 \
    --host test.localhost \
    --forward-auth-url http://authelia:9091/api/authz/forward-auth
```

### Issue: Build fails

**Clean and rebuild:**
```bash
docker-compose down -v
docker-compose build --no-cache
docker-compose up -d
```

## Validation Checklist

Use this checklist to validate the ForwardAuth implementation:

- [ ] Public endpoints work without authentication
- [ ] Protected endpoints require authentication (302/401 without auth)
- [ ] Authelia login page is shown for unauthorized requests
- [ ] After login, protected resources are accessible
- [ ] Session cookies persist across requests
- [ ] Logout works (clears session)
- [ ] X-Forwarded-* headers are sent to Authelia
- [ ] Configured headers are copied from Authelia response
- [ ] Invalid auth service URLs return 502 error
- [ ] Timeout configuration works
- [ ] Multiple services with different auth configs work
- [ ] ForwardAuth config persists across kamal-proxy restarts

## Clean Up

```bash
# Stop all containers
docker-compose down

# Remove volumes (caution: removes all data)
docker-compose down -v

# Remove hosts file entries
sudo sed -i '/public.localhost protected.localhost app.localhost/d' /etc/hosts
```

## Expected Results Summary

| Endpoint | Auth Required | Expected Result |
|----------|---------------|-----------------|
| http://public.localhost | No | 200 OK - whoami data |
| http://protected.localhost | Yes | 302 redirect to login |
| http://app.localhost | Yes | 302 redirect to login |
| After login | Yes | 200 OK - protected content |

## Integration Points Validated

This test environment validates:

1. ✅ **CLI Integration** - Flags correctly configure ForwardAuthConfig
2. ✅ **Middleware Chain** - ForwardAuth executes before backend
3. ✅ **HTTP Client** - Subrequests sent to Authelia
4. ✅ **Header Forwarding** - X-Forwarded-* headers sent correctly
5. ✅ **Header Copying** - Auth headers copied to backend request
6. ✅ **Response Handling** - 2xx allows, non-2xx blocks
7. ✅ **Error Handling** - Network errors return 502
8. ✅ **Timeout Handling** - Respects configured timeout
9. ✅ **State Persistence** - Config survives restart
10. ✅ **Concurrent Requests** - Handles multiple requests correctly

## Success Criteria

The test is considered successful when:

1. Public endpoint returns 200 without authentication
2. Protected endpoints return 302/401 without authentication
3. Authelia login page is accessible
4. After authentication, protected endpoints return 200
5. Headers are correctly forwarded to Authelia
6. Headers are correctly copied from Authelia response
7. Invalid auth services return 502
8. No errors in kamal-proxy or Authelia logs
9. Multiple services with different configs work independently
10. Performance is acceptable (< 100ms overhead per request)
