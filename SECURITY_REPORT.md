# Atlas - OWASP Security Assessment Report

**Date:** April 6, 2026 (re-scanned 7:07 PM IST)
**Application:** Atlas Kubernetes Dashboard
**Version:** latest (docker image `atlas:latest`)
**Base Image:** Alpine 3.19.9
**Scanner:** Trivy v0.68 + Manual Code Review
**Scope:** Full stack — Go backend, static UI, Docker image, configuration, dependencies

---

## Verdict: NOT READY for Cloud Production Deployment

Atlas has **4 Critical**, **4 High**, **5 Medium**, and **4 Low** findings. The critical issues (no authentication, SSRF endpoint, hardcoded secrets) **must be resolved** before any cloud deployment.

---

## Executive Summary

| Severity | Count | Cloud Blocker? |
|----------|-------|----------------|
| Critical | 4     | YES            |
| High     | 4     | YES            |
| Medium   | 5     | Recommended    |
| Low      | 4     | No             |
| Info     | 3     | No             |
| **Total**| **20**|                |

### Scan Results Summary

| Scan Type                  | Tool   | Findings                                                 |
|----------------------------|--------|----------------------------------------------------------|
| Docker Image CVEs          | Trivy  | 6 (0 Critical, 0 High, 3 Medium, 3 Low)                 |
| Go Dependencies (go.mod)   | Trivy  | 0 vulnerabilities                                        |
| Go Binary (compiled)       | Trivy  | 0 vulnerabilities                                        |
| Python Packages (awscli)   | Trivy  | 0 vulnerabilities                                        |
| Dockerfile Misconfig       | Trivy  | 1 (missing HEALTHCHECK instruction)                     |
| Embedded Secrets (image)   | Trivy  | 0 detected in image layers                               |
| Manual Code Review (OWASP) | Manual | 4 Critical, 4 High, 5 Medium, 4 Low, 3 Info             |

---

## OWASP Top 10 (2021) Mapping

| OWASP Category                              | Status     | Findings |
|---------------------------------------------|------------|----------|
| A01 — Broken Access Control                 | **FAIL**   | No authentication or authorization on any endpoint. Full K8s API access exposed to any caller. |
| A02 — Cryptographic Failures                | **FAIL**   | No TLS in app binary. Redis without TLS. Plaintext password in committed `config.yaml`. |
| A03 — Injection                             | WARN       | Low server-side risk (no SQL/exec). UI uses `innerHTML` with API error messages — potential DOM XSS. |
| A04 — Insecure Design                       | **FAIL**   | Dashboard with full cluster access exposed without auth. SSRF endpoint by design. |
| A05 — Security Misconfiguration             | **FAIL**   | Missing security headers. Redis port exposed on host. No CORS policy. |
| A06 — Vulnerable and Outdated Components    | PASS       | Go deps clean. Alpine has 6 low/medium busybox CVEs (fixable). |
| A07 — Identification & Authentication Fail  | **FAIL**   | No auth. `X-User-ID` header trusted without verification. Session cookie never set. |
| A08 — Software and Data Integrity Failures  | PASS       | Multi-stage Docker build. No unsigned dependency concerns found. |
| A09 — Security Logging & Monitoring Fail    | WARN       | Basic request logging present. No alerting on abuse of sensitive endpoints. |
| A10 — Server-Side Request Forgery (SSRF)    | **FAIL**   | `/api/network/test` accepts user-supplied hostnames for DNS/TCP/HTTP probes with no restrictions. |

---

## Detailed Findings

### CRITICAL Findings (Must Fix Before Deployment)

#### C-01: No Authentication or Authorization
- **OWASP:** A01 Broken Access Control, A07 Identification & Authentication Failures
- **Location:** `internal/httpapi/routes.go` — all routes
- **Description:** The entire API and UI are exposed without any authentication. No API keys, JWT, OAuth, or basic auth. Anyone who can reach port 8080 gets full read access to the Kubernetes cluster (namespaces, pods, deployments, secrets metadata, configmaps, etc.).
- **Impact:** Complete unauthorized access to cluster information.
- **Remediation:** Implement authentication middleware (JWT/OAuth2/OIDC). Add RBAC for namespace-level authorization.

#### C-02: Server-Side Request Forgery (SSRF)
- **OWASP:** A10 SSRF
- **Location:** `POST /api/network/test` → `internal/network/test.go`
- **Description:** Accepts user-supplied hostnames and performs DNS lookups, TCP connections, and HTTP(S) requests from the server. No allowlist, no blocklist for internal IPs (169.254.169.254, 10.x, 172.x, 192.168.x), no metadata service protection.
- **Impact:** Attacker can probe internal network, access cloud metadata services (AWS IMDSv1), and scan internal infrastructure.
- **Remediation:** Remove endpoint or add strict allowlist. Block RFC1918 addresses and cloud metadata IPs. Require authentication.

#### C-03: Hardcoded Secrets in Configuration
- **OWASP:** A02 Cryptographic Failures
- **Location:** `config.yaml` line 13 — `password: "Amdocs@123"`
- **Description:** Redis password is committed in plaintext in the configuration file.
- **Impact:** Credential exposure in version control.
- **Remediation:** Use environment variables or a secrets manager (AWS Secrets Manager, HashiCorp Vault). Remove password from config file. Add `config.yaml` to `.gitignore` or use a template.

#### C-04: Trusted User Identity Header Without Verification
- **OWASP:** A07 Identification & Authentication Failures
- **Location:** `internal/httpapi/handlers_clusters.go` — `getUserID()` function
- **Description:** The `X-User-ID` header is trusted directly for multi-cluster session management. Any client can set this header to impersonate any user and switch their cluster context.
- **Impact:** Session hijacking, unauthorized cluster switching.
- **Remediation:** Implement proper session management with signed tokens. Never trust client-supplied identity headers.

---

### HIGH Findings (Should Fix Before Deployment)

#### H-01: No TLS Support in Application
- **OWASP:** A02 Cryptographic Failures
- **Location:** `cmd/atlas/main.go` — uses `http.ListenAndServe` only
- **Description:** The application only serves HTTP. No TLS configuration exists in code despite `.env` commenting on TLS paths. The nginx reverse proxy in docker-compose is commented out.
- **Impact:** All traffic (including Kubernetes data) transmitted in plaintext.
- **Remediation:** Deploy behind a TLS-terminating reverse proxy (nginx, ALB, Istio). Enable the nginx service in docker-compose with valid certificates.

#### H-02: Redis Connection Without TLS
- **OWASP:** A02 Cryptographic Failures
- **Location:** `internal/cache/redis.go` — `redis.NewClient()`
- **Description:** Redis client connects with password authentication but no TLS encryption. Cache data (including Kubernetes resource information) travels in plaintext between Atlas and Redis.
- **Impact:** Data interception on the network between Atlas and Redis.
- **Remediation:** Enable Redis TLS (`tls.Config` in go-redis client). Use AWS ElastiCache with in-transit encryption.

#### H-03: No Rate Limiting
- **OWASP:** A04 Insecure Design
- **Location:** `internal/httpapi/routes.go` — all endpoints
- **Description:** No rate limiting on any endpoint including expensive operations (`/api/health/{ns}`, `/api/pods/{ns}`) and the SSRF-capable `/api/network/test`.
- **Impact:** Denial of service. Amplified SSRF abuse.
- **Remediation:** Add rate limiting middleware (e.g., `golang.org/x/time/rate`). Especially critical for `/api/network/test` and `/api/cache/clear`.

#### H-04: Unauthenticated Cache Destruction
- **OWASP:** A01 Broken Access Control
- **Location:** `POST /api/cache/clear`
- **Description:** Any caller can clear the entire Redis cache without authentication, causing performance degradation and forcing re-fetches from the Kubernetes API.
- **Impact:** Denial of service via cache flushing.
- **Remediation:** Require authentication. Restrict to admin role.

---

### MEDIUM Findings

#### M-01: Missing Security Headers
- **OWASP:** A05 Security Misconfiguration
- **Location:** `internal/httpapi/routes.go` — no security header middleware
- **Description:** No `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, `Strict-Transport-Security`, or `Referrer-Policy` headers set.
- **Impact:** Clickjacking, MIME sniffing, content injection attacks.
- **Remediation:** Add security headers middleware:
  ```
  X-Content-Type-Options: nosniff
  X-Frame-Options: DENY
  Content-Security-Policy: default-src 'self'
  Referrer-Policy: strict-origin-when-cross-origin
  ```

#### M-02: Redis Port Exposed on Host
- **OWASP:** A05 Security Misconfiguration
- **Location:** `docker-compose.yaml` line 15 — `"6379:6379"`
- **Description:** Redis is published on the host network. If the host has lax firewall rules, Redis is accessible externally.
- **Impact:** Unauthorized Redis access (password-protected but still exposed).
- **Remediation:** Remove host port mapping. Use Docker network-only communication between Atlas and Redis. If external access needed, bind to 127.0.0.1 only.

#### M-03: Potential DOM XSS via innerHTML
- **OWASP:** A03 Injection
- **Location:** `ui/script.js`, `ui/detail-panels.js` — various `innerHTML` assignments
- **Description:** UI uses `innerHTML` with data from API responses including error messages. If attacker-controlled data reaches these paths, XSS is possible.
- **Impact:** Cross-site scripting in the dashboard.
- **Remediation:** Use `textContent` instead of `innerHTML` for user/API data. Implement Content-Security-Policy header.

#### M-04: Error Messages Leak Internal Details
- **OWASP:** A04 Insecure Design, A05 Security Misconfiguration
- **Location:** `internal/httpapi/handlers.go`, `handlers_workloads.go`, `handlers_health.go`
- **Description:** Handlers return `err.Error()` directly to clients, which may contain Kubernetes API server URLs, internal hostnames, and authentication error details.
- **Impact:** Information disclosure aiding further attacks.
- **Remediation:** Return generic error messages to clients. Log detailed errors server-side only.

#### M-05: Alpine Base Image CVEs (busybox)
- **OWASP:** A06 Vulnerable and Outdated Components
- **Source:** Trivy image scan
- **Description:** 6 vulnerabilities in busybox packages (3 Medium, 3 Low):
  - **CVE-2024-58251** (Medium) — netstat local users can launch network operations
  - **CVE-2025-46394** (Low) — tar archive filename handling issue
- **Fixed Version:** busybox 1.36.1-r21
- **Remediation:** Update base image: `FROM alpine:3.19` → use a more recent tag, or add `RUN apk upgrade --no-cache` in Dockerfile.

---

### LOW Findings

#### L-01: Missing HEALTHCHECK in Dockerfile
- **Source:** Trivy config scan (AVD-DS-0026)
- **Location:** `Dockerfile`
- **Description:** No HEALTHCHECK instruction in the Dockerfile. Docker/orchestrators won't know container health without external health checks.
- **Remediation:** Add `HEALTHCHECK CMD wget -q --spider http://localhost:8080/healthz || exit 1`

#### L-02: No CORS Policy
- **OWASP:** A05 Security Misconfiguration
- **Location:** `internal/httpapi/routes.go`
- **Description:** No CORS headers are set. Browser same-origin policy applies by default, but cross-origin API access is uncontrolled.
- **Remediation:** Add explicit CORS middleware with allowed origins whitelist.

#### L-03: Session Cookie Never Set
- **OWASP:** A07 Identification & Authentication Failures
- **Location:** `internal/httpapi/handlers_clusters.go`
- **Description:** Code reads an `atlas_session` cookie but never sets it. Generates random IDs as fallback, making sessions unreliable.
- **Remediation:** If sessions are needed, set the cookie with `Secure`, `HttpOnly`, `SameSite=Strict` flags.

#### L-04: Duplicate Route Registration
- **Location:** `internal/httpapi/routes.go`
- **Description:** `GET /api/cache/stats` is registered twice.
- **Impact:** Minimal — last handler wins. May indicate copy-paste error.
- **Remediation:** Remove duplicate registration.

---

### INFORMATIONAL

#### I-01: Kubeconfig Path Logged at Startup
- **Location:** `cmd/atlas/main.go`
- **Description:** Cluster kubeconfig file paths are logged at INFO level during startup.
- **Recommendation:** Log at DEBUG level in production.

#### I-02: No Panic Recovery Middleware
- **Location:** `internal/httpapi/routes.go`
- **Description:** No panic recovery middleware observed. Unhandled panics could crash the process and potentially leak stack traces.
- **Recommendation:** Add `net/http` recovery middleware.

#### I-03: Export Handler Dead Code
- **Location:** `internal/httpapi/handlers_export.go`
- **Description:** `getExport` handler exists but is not registered in routes. Dead code increases maintenance burden.
- **Recommendation:** Remove or wire it up.

---

## Trivy Scan Details

### Docker Image Vulnerabilities (atlas:latest)

| Package       | CVE            | Severity | Installed   | Fixed       |
|---------------|----------------|----------|-------------|-------------|
| busybox       | CVE-2024-58251 | MEDIUM   | 1.36.1-r20  | 1.36.1-r21  |
| busybox       | CVE-2025-46394 | LOW      | 1.36.1-r20  | 1.36.1-r21  |
| busybox-binsh | CVE-2024-58251 | MEDIUM   | 1.36.1-r20  | 1.36.1-r21  |
| busybox-binsh | CVE-2025-46394 | LOW      | 1.36.1-r20  | 1.36.1-r21  |
| ssl_client    | CVE-2024-58251 | MEDIUM   | 1.36.1-r20  | 1.36.1-r21  |
| ssl_client    | CVE-2025-46394 | LOW      | 1.36.1-r20  | 1.36.1-r21  |

### Go Dependencies (go.mod): CLEAN — 0 vulnerabilities
### Go Binary (app/atlas): CLEAN — 0 vulnerabilities
### Python Packages (awscli stack): CLEAN — 0 vulnerabilities
### Embedded Secrets in Image: CLEAN — 0 secrets detected
### Dockerfile Misconfiguration: 1 finding (missing HEALTHCHECK — Low)

---

## Cloud Deployment Readiness Checklist

| Requirement                            | Status | Notes                                           |
|----------------------------------------|--------|--------------------------------------------------|
| Authentication on all endpoints        | FAIL   | No auth implemented                              |
| Authorization / RBAC                   | FAIL   | No authorization layer                           |
| TLS / HTTPS                            | FAIL   | HTTP only; no TLS in app or proxy                |
| Secrets management                     | FAIL   | Hardcoded in config.yaml                         |
| No Critical/High CVEs                  | PASS   | Image CVEs are Medium/Low only                   |
| Input validation                       | WARN   | Minimal; relies on K8s API validation            |
| Security headers                       | FAIL   | None configured                                  |
| Rate limiting                          | FAIL   | None implemented                                 |
| SSRF protection                        | FAIL   | /api/network/test is unrestricted                |
| Logging & monitoring                   | WARN   | Basic logging; no abuse alerting                 |
| Non-root container                     | PASS   | Runs as appuser (UID 1000)                       |
| Read-only filesystem                   | PASS   | Configured in docker-compose (tmpfs for /tmp)    |
| Resource limits                        | PASS   | CPU/memory limits set in docker-compose          |
| Health checks                          | WARN   | In docker-compose but missing from Dockerfile    |
| Dependency vulnerabilities             | PASS   | Go modules and Python packages clean             |
| No privilege escalation                | PASS   | `no-new-privileges:true` set in docker-compose   |

---

## Recommended Remediation Priority

| Priority | Action                                                  | Effort   |
|----------|---------------------------------------------------------|----------|
| 1        | Add authentication middleware (OAuth2/OIDC/JWT)         | High     |
| 2        | Remove or restrict `/api/network/test` endpoint         | Low      |
| 3        | Remove hardcoded password from `config.yaml`            | Low      |
| 4        | Deploy behind TLS-terminating proxy (ALB/nginx)         | Medium   |
| 5        | Add security response headers middleware                | Low      |
| 6        | Add rate limiting middleware                             | Medium   |
| 7        | Enable Redis TLS                                        | Medium   |
| 8        | Remove Redis host port exposure                         | Low      |
| 9        | Replace `innerHTML` with `textContent` in UI            | Low      |
| 10       | Sanitize error messages returned to clients             | Medium   |
| 11       | Update Alpine packages (`apk upgrade`)                  | Low      |
| 12       | Add HEALTHCHECK to Dockerfile                           | Low      |

---

*Report generated by Trivy v0.68 automated scans + manual OWASP Top 10 code review.*
*This report is based on static analysis and does not replace dynamic penetration testing.*
