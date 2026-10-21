# PermitPal

PermitPal is a self-hosted Go dashboard for tracking progress toward the North Carolina Level 2 road test. Each driver has a separate login and tracker for the Level 1 to Level 2 requirement of 60 total driving hours, including 10 night hours. It supports an ephemeral in-memory preview and persistent PostgreSQL deployments.

Ready estimates retain the observed pace of manually entered cumulative total and night hours since the permit issue date, and apply a 9-month permit-hold floor. Keep using the existing current-total inputs, including totals beyond 60 total or 10 night hours when earlier hours were excluded. Both recorded totals accept up to 1,000 hours with one decimal place, with night hours within total hours; the credited requirements stay 60 total and 10 night. Each changed save journals a cumulative snapshot with its server entry timestamp and `APP_TIMEZONE` calendar date. Positive increments count in that recorded Monday–Sunday ISO week, capped at 10 total hours; night hours count within that shared limit and are selected first. The projection applies the same cap to future weeks and names the weekly cap when it delays the date. These are **entry dates, not actual drive dates**; this is an estimate, not verification of an official driving log.

Repeated unchanged submissions and permit-date-only changes create no hour entry. Corrections reverse the newest matching day/night hours first, then undated opening hours; moving daytime hours to night records the new night classification on the save date. Previously saved totals remain an explicitly undated, unverified opening balance without invented historical weeks. Writes by an older binary during rollout are likewise reconciled as undated changes. Migration 007 adds this journal while preserving cumulative profile totals; apply it before deploying the new binary. Reverting the binary retains current totals, but rolling back migration 007 discards entry-date history while retaining cumulative totals and their expanded input bounds. An older binary still enforces its old 60/10 input limits; do not reduce recorded totals to satisfy them.

## Requirements

- Docker 24+
- PostgreSQL 15+ for persistent deployments
- Apache `htpasswd` for generating bcrypt users entries
- Goose for database migrations

## Build the image

```bash
docker build -t permitpal:local .
```

## Configure the application

Create separate bcrypt credentials. These commands prompt for each password:

```bash
umask 077
htpasswd -cB -C 12 permitpal_users caleb
htpasswd -B -C 12 permitpal_users aiden
openssl rand -base64 48
```

Use `-c` only for the first account because it replaces the file. Keep the users file private and outside Git. A new username gets a fresh tracker on first login, with zero hours, no permit date, and the 17 road-test skills. Caleb's migrated tracker retains his original 13 skills and history. Migration 001 also seeds that tracker into a new database; migration 006 removes it again when the same goose run created the database and nobody has saved to it, so a new install starts with no drivers.

Mount `permitpal_users` at `/run/secrets/permitpal_users`. With Docker Swarm, create an external secret using `docker secret create permitpal_users permitpal_users` and mount it on every PermitPal replica. Swarm distributes the secret to the hosts running those replicas. Restart or redeploy after credential changes. The application reads credentials at startup; a removed account's cookie is rejected by the updated replicas. Session cookies last 30 days. Logging out signs that driver out on every device, because it advances a per-driver session generation that every cookie must match; cookies issued before generations existed count as generation 0 and stay valid until the driver's next logout. Replicas of a release from before session generations do not check them, so during that rolling update a logout takes full effect once every replica runs the new release.

Create an untracked `permitpal.env` file:

```dotenv
APP_ENV=production
DATA_STORE=postgres
DATABASE_URL=postgres://permitpal:change-me@db:5432/permitpal?sslmode=disable
PERMITPAL_USERS_FILE=/run/secrets/permitpal_users
SESSION_SECRET=replace-with-a-32-character-or-longer-session-secret
PORT=4600
SECURE_COOKIES=true
```

Accounts come only from `PERMITPAL_USERS` or the users file. An explicitly empty `*_FILE` setting disables the default file fallback; an unset setting allows the optional default file to load. Keep secure cookies enabled behind HTTPS. For local HTTP development, use `APP_ENV=development` and `SECURE_COOKIES=false`.

| Setting | Required | Purpose |
| --- | --- | --- |
| `APP_ENV` | No | `development` or `production`; any other value stops startup. Docker defaults to `production`; local runs default to `development` |
| `DATA_STORE` | No | Docker defaults to `postgres`; development defaults to `memory` |
| `DATABASE_URL` | With Postgres | PostgreSQL connection string |
| `PERMITPAL_USERS` / `PERMITPAL_USERS_FILE` | Yes | Newline-separated `username:bcrypthash` entries; the optional default file is `/run/secrets/permitpal_users` |
| `SESSION_SECRET` | Yes | Session-signing secret of at least 32 characters |
| `SESSION_COOKIE` | No | Cookie name; defaults to `permitpal_session` |
| `SECURE_COOKIES` | No | Defaults to `true` in production and `false` in development |
| `PORT` | No | HTTP port; defaults to `4600` |
| `LOG_LEVEL` | No | Application log level; defaults to `info` |
| `APP_TIMEZONE` | No | IANA time zone that decides "today" for rating dates and pace estimates; defaults to `America/New_York`. Time zone data is built into the binary |
| `METRICS_ADDR` | No | Listen address of the internal Prometheus `/metrics` listener; defaults to `:9464`. `off` disables it |
| `TRUSTED_PROXY_CIDRS` | Behind a reverse proxy | Comma-separated CIDRs or IPs whose `X-Forwarded-For` header is trusted; defaults to empty, which ignores forwarded headers and disables the per-IP login limit |

Failed logins are limited to 20 per username and, when the real client IP is known, 20 per client IP in a 15-minute window; further attempts get HTTP 429 with `Retry-After` until the window ends, and a successful login clears that username's failures. Counters are in memory per replica and reset on restart. The per-IP limit applies only when `TRUSTED_PROXY_CIDRS` is set: a peer outside those CIDRs is a direct client keyed by its TCP address, and a peer inside them is keyed by the rightmost `X-Forwarded-For` address that is not a trusted proxy. With `TRUSTED_PROXY_CIDRS` empty, or when a trusted proxy sends no usable `X-Forwarded-For`, only the username limit applies. The username limit lets anyone lock a known username out for 15 minutes. A username that cannot exist under the rule below fails without a password check and counts only toward the per-IP limit. The limiter tracks at most 10,000 keys and evicts the oldest window beyond that. Behind Traefik on a Docker Swarm overlay network, set `TRUSTED_PROXY_CIDRS` to that network's subnet, for example the output of `docker network inspect proxy --format '{{range .IPAM.Config}}{{.Subnet}} {{end}}'`.

Usernames must match `^[a-z0-9][a-z0-9_-]{0,31}$`; login trims whitespace and ignores username case. Duplicate usernames are rejected. Bcrypt hashes with `$2a$`, `$2b$`, or `$2y$` prefixes are accepted. The database URL, users list, and session secret support corresponding `*_FILE` variables. Explicit file paths must exist; a missing implicit default users file is allowed.

## Database and migrations

```bash
docker network create permitpal

docker run -d --name db --network permitpal \
  -e POSTGRES_DB=permitpal \
  -e POSTGRES_USER=permitpal \
  -e POSTGRES_PASSWORD=change-me \
  -p 5432:5432 \
  -v permitpal-postgres:/var/lib/postgresql/data \
  postgres:17

until docker exec db pg_isready -U permitpal -d permitpal >/dev/null 2>&1; do sleep 1; done
```

Apply migrations before starting PermitPal:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
export DATABASE_URL='postgres://permitpal:change-me@localhost:5432/permitpal?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
```

With `DATA_STORE=postgres`, PermitPal reads the applied goose version from `goose_db_version` at startup and exits with an error if it is older than the newest migration the binary was built with. CI deploys new images automatically on pushes to `main`, so apply migrations before merging a change that adds one; otherwise the new replicas refuse to start and Swarm keeps or rolls back to the previous version. When adding a migration, bump `repository.SchemaVersion`; a test fails until it matches `migrations/`. Migrations 004 to 006 only add a column, add a constraint and remove the unused seed, so the previous release keeps working on the migrated schema while new replicas roll out.

## Run with Docker

```bash
docker run --rm --name permitpal --network permitpal \
  --env-file permitpal.env \
  --mount type=bind,src="$(pwd)/permitpal_users",dst=/run/secrets/permitpal_users,readonly \
  -p 4600:4600 \
  permitpal:local
```

Open <http://localhost:4600>. Two unauthenticated probes report status:

- `/health` returns `200 ok` whenever the process is up. It checks nothing else.
- `/ready` pings the database with a 2-second timeout. It returns `200 {"status":"ok","db":"ok"}`, or `503 {"status":"degraded","db":"error"}` when the database does not answer; the cause goes to the log, not the response. With `DATA_STORE=memory` it always reports ok.

`METRICS_ADDR` is trimmed and `off` is case-insensitive. Other values must be a valid IP address or hostname plus a numeric port from 0 to 65535 (IPv6 addresses need brackets). Invalid syntax stops startup; validation does not resolve hostnames. Keep the metrics port private and never publish it through Docker or the public proxy.

Prometheus metrics are served at `GET /metrics` on a separate internal listener, `METRICS_ADDR` (default `:9464`), and never on the public port, where `/metrics` returns 404. The Dockerfile does not publish that port, so only peers on the container's network, such as a Prometheus job scraping over the Swarm overlay, can reach it. If the address cannot be bound, PermitPal logs the error and keeps serving without metrics; `METRICS_ADDR=off` turns the listener off. The metrics are:

- `http_requests_total` and `http_request_duration_seconds`, labelled by `method`, chi route pattern (`route`, such as `/requirements/{key}`; requests that match no route, or that a middleware refuses before routing, use `unmatched`) and status `code`. `/health`, `/ready` and `/metrics` are not counted.
- `pgxpool_*` connection pool statistics, such as `pgxpool_acquired_conns`, `pgxpool_idle_conns`, `pgxpool_total_conns`, `pgxpool_max_conns` and `pgxpool_empty_acquire_wait_seconds_total`, with `DATA_STORE=postgres`.
- `permitpal_login_failures_total{reason}`, where `reason` is `invalid_request`, `rate_limited`, `invalid_username` or `invalid_password`.
- The Go runtime (`go_*`) and process (`process_*`) collectors.

For a disposable preview, clear the production database secret path and pass a users line:

```bash
docker run --rm -p 4600:4600 \
  -e APP_ENV=development \
  -e DATA_STORE=memory \
  -e DATABASE_URL_FILE= \
  -e PERMITPAL_USERS="$(htpasswd -nB driver)" \
  -e SESSION_SECRET=replace-with-a-32-character-or-longer-secret \
  permitpal:local
```

Postgres and migrations are not needed in this mode. `htpasswd` prompts for the preview password. Log in as `driver` with that password; all preview data disappears on restart.

## Development

```bash
cp local.mk.example local.mk
make dev
make test
```

Use `make run-postgres` for a persistent local run and the `make migrate*` targets for database maintenance.

## Upgrade an existing tracker

Back up Postgres before applying migration 003. Stop the old application during the schema change because the old binary cannot read the new schema. Run `DATABASE_URL=... make migrate`, then deploy the new image with the users secret. PermitPal no longer reads the old single-account settings (`PERMITPAL_PASSWORD`, `PERMITPAL_PASSWORD_HASH`, their `*_FILE` variants, and `PERMITPAL_USERNAME`), so remove them and the old password-hash secret mount. Migrations remain a manual deploy step.

Only accounts in the users file can sign in, so the old `driver` login and its cookies stop working unless `driver` is added there. Caleb logs in as `caleb` with his existing password; Aiden logs in as `aiden` and sets his permit issue date. Preserve Caleb's current bcrypt hash in the users file if his password should remain unchanged. His saved mastered ratings become Good, and needs-practice ratings become Fair. Rated dates and notes are preserved.

Migration 003 can be reversed on a scratch database with `make migrate-down` then `make migrate`. A downgrade deliberately deletes all drivers except Caleb and maps Good to mastered and other ratings to needs-practice. Do not downgrade production without a backup and an explicit decision to discard other drivers' data.

## Verification

```bash
make check-generated
make lint
make test
PERMITPAL_TEST_DATABASE_URL='postgres://user:password@localhost/permitpal_test?sslmode=disable' go test -race ./internal/...
go vet ./...
make build
```

`make lint` regenerates templ output, checks all Go files (including generated files) with `gofmt` without rewriting them, and runs staticcheck. Staticcheck is pinned as a Go tool dependency in `go.mod`, so no global installation is needed and Renovate can track its version. Fix formatting in source files with `gofmt -w`; edit `.templ` sources and run `make templ` for generated output.

CI also runs the dashboard feedback browser test in Chromium against both memory and Postgres stores. To run the same test locally with Node 24, set `PERMITPAL_TEST_DATABASE_URL` to a disposable database, then run from the repository root:

```bash
npm ci --prefix test-support/browser
test-support/browser/node_modules/.bin/playwright install --with-deps chromium
export PERMITPAL_PLAYWRIGHT_MODULE="$PWD/test-support/browser/node_modules/playwright"
: "${PERMITPAL_TEST_DATABASE_URL:?Set a disposable Postgres test database}"
go test -race -count=1 -v ./internal/server -run '^TestDashboardSaveFeedbackInBrowser$'
```

The test uses its own `aiden`/`test-password` fixture and temporary database schema; it needs no production credentials. Leave `PERMITPAL_PLAYWRIGHT_MODULE` unset for ordinary Go test runs to avoid running Chromium twice.

Database tests use isolated temporary schemas and verify migration 003 up/down/up. Use only a disposable database for the test URL. For two-account browser checks, set `PERMITPAL_USERS_FILE` and `SESSION_SECRET` in `local.mk`, then run `make dev`. Sign in as each driver, save hours and a rating, and confirm that the other driver's tracker is unchanged.
