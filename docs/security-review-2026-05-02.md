# Security Review (2026-05-02)

## Scope and assumptions

- Application: PermitPal (single-user app).
- Threat model emphasis requested:
  - Host takeover risk by remote/foreign actor.
  - Bogus input causing database problems.
  - Single points of failure around password/authentication.
- Context adjustment: this is not a public identity system, so account-enumeration and multi-user isolation risks are deprioritized.

## Executive summary

Overall, the app is small and has relatively limited attack surface. The most meaningful risks are around:

1. **Weak/default secrets in non-production or misconfigured production** (password and session secret).
2. **No CSRF defense on state-changing routes** (relevant if user is logged in and visits hostile site).
3. **No login rate limiting / lockout** (brute-force risk).
4. **Potentially unbounded notes size and missing DB-side guardrails** (risk of oversized writes and performance/availability issues).

There are no obvious SQL injection paths in current write/read queries; parameterized SQL is used.

## Findings

### 1) Default password fallback can create easy compromise if deployed as-is

**Severity:** High (if internet-reachable)

If no `PERMITPAL_PASSWORD` or `PERMITPAL_PASSWORD_HASH` is set, config falls back to `permitpal`. This is safe for local dev but dangerous if mistakenly exposed.

- Evidence: `internal/config/config.go` sets default password when both secrets are empty.
- Impact: attacker can log in trivially, then modify all data.

**Recommendation:**
- Remove hardcoded default entirely, or allow only in explicit `APP_ENV=development` with loud startup warning.
- Fail startup unless password hash is configured for any non-local deployment profile.

### 2) Default session secret fallback enables cookie forgery if left unchanged

**Severity:** High (if internet-reachable)

If `SESSION_SECRET` is missing, app uses static fallback `permitpal-local-dev-session-secret-change-me`. Session uses HMAC signature; a known secret allows forged authenticated cookies.

- Evidence: `internal/config/config.go` fallback; `internal/auth/session.go` uses HMAC(session secret) for auth cookie validation.
- Impact: full auth bypass (attacker can mint valid cookie).

**Recommendation:**
- No default in runtime config except maybe unit tests.
- Hard fail startup when secret absent, except optionally explicit development mode.
- Consider secret length/entropy validation.

### 3) No CSRF protection on POST endpoints

**Severity:** Medium

`POST /profile`, `POST /requirements/{key}`, and `POST /logout` rely only on cookie auth. Cookies are `SameSite=Lax`, which helps but is not a complete CSRF control for all browser/navigation edge-cases and same-site contexts.

- Evidence: handlers accept POST without CSRF token checks.
- Impact: logged-in user can be induced to submit unwanted state changes.

**Recommendation:**
- Add synchronizer token (hidden form token validated server-side) or double-submit cookie token.
- Keep `SameSite` and `HttpOnly`; consider `SameSite=Strict` if UX allows.

### 4) No brute-force throttling on login

**Severity:** Medium

Login endpoint has no rate limiting, delay, or lockout.

- Evidence: `internal/handler/auth.go` checks password directly and returns error on mismatch.
- Impact: online guessing attacks against weak password.

**Recommendation:**
- Add per-IP and/or global attempt throttling.
- Add small constant delay on failed auth.
- Require strong password hash config (bcrypt hash path already exists).

### 5) Input validation is decent for numeric fields; SQL injection risk appears low

**Severity:** Low (positive finding)

`total_hours` and `night_hours` are bounded and formatted constraints are enforced. Database writes use query parameters (`$1..$n`), not string concatenation.

- Evidence: `parseHours` validation and parameterized queries in `internal/repository/postgres.go`.
- Impact: low likelihood of SQL injection from current code paths.

**Recommendation:**
- Keep parameterized SQL pattern.
- Add DB constraints mirroring handler limits (defense in depth).

### 6) Notes field appears unbounded in app layer; could allow oversized payloads / DB bloat

**Severity:** Medium-Low

`notes` is trimmed but no length limit is enforced before DB update.

- Evidence: `internal/handler/dashboard.go` sets `existing.Notes` from form input; DB update writes it directly.
- Impact: potential large row growth, slow queries, storage bloat, or memory pressure depending on request size limits and proxy config.

**Recommendation:**
- Enforce max note length in handler (e.g., 1k–4k chars).
- Add DB column length/check constraint.
- Add request body size cap via middleware (`http.MaxBytesReader`).

### 7) Single-user password is a single point of failure by design

**Severity:** Architectural risk (expected)

One credential gates all data/operations. Compromise, loss, or weak management of this secret is total compromise.

**Recommendation:**
- Treat password + session secret as tier-0 secrets.
- Use hashed password only (`PERMITPAL_PASSWORD_HASH`), disable plaintext password path in production.
- Add rotation process: change password, invalidate sessions by rotating session secret.
- Backup and secret recovery procedure should be documented.

## Host takeover risk assessment

From code review alone, there is **no direct remote code execution primitive** apparent. The app mostly performs form parsing and parameterized DB updates. Main path to “control of system” is likely operational misconfiguration (weak default secrets, exposed service, missing TLS/reverse-proxy hardening), not an obvious code injection bug.

## Recommended priority plan

1. **Block insecure defaults in non-dev** (password + session secret).
2. **Add CSRF tokens** for all mutating routes.
3. **Add login throttling**.
4. **Constrain input sizes** (`notes`, request body max, DB check constraints).
5. **Document secret rotation + recovery** for the single-user model.

## Quick checklist for your deployment

- [ ] `APP_ENV=production`
- [ ] `PERMITPAL_PASSWORD_HASH` set (bcrypt), plaintext password unset
- [ ] `SESSION_SECRET` strong random value (>=32 bytes entropy)
- [ ] `SECURE_COOKIES=true` behind HTTPS only
- [ ] Service not exposed publicly unless intentional (IP allowlist/VPN)
- [ ] Regular backups of database
- [ ] Reverse proxy request-size limit configured


## Secret rotation runbook (single-user)

1. Generate a new bcrypt hash for a new password and store as `PERMITPAL_PASSWORD_HASH`.
2. Generate a new random `SESSION_SECRET` (32+ bytes entropy).
3. Deploy both values together.
4. Restart application; all existing sessions are invalidated because cookie signatures no longer verify.
5. Verify login with the new password and confirm old session cookies fail.
6. Keep previous secrets only in rollback vault window, then retire them.
