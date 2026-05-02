# PermitPal

PermitPal is a Go/HTMX dashboard for tracking NC learner permit exam readiness.

Tagline: **Your permit pal—less yelling, more tracking.**

## Local preview without a database

```bash
make dev
```

Open `http://localhost:4600` and sign in with the default local password:

```text
permitpal
```

The default development mode uses `DATA_STORE=memory`, so dashboard changes persist only while the process is running.

## Production-like local run

```bash
make migrate DATABASE_URL="postgres://permitpal:permitpal@localhost:5432/permitpal?sslmode=disable"
make run-postgres
```

## Production database and Swarm setup

PermitPal uses Postgres in production. Create a dedicated database and role before deploying:

```sql
create user permitpal with password '<strong-password>';
create database permitpal owner permitpal;
grant all privileges on database permitpal to permitpal;
```

Apply the schema once from a machine with `goose` and network access to the database:

```bash
make migrate DATABASE_URL="postgres://permitpal:<password>@<postgres-host>:8432/permitpal?sslmode=disable"
```

The Makefile normalizes the username and password in `DATABASE_URL` before
passing it to `goose`, so raw reserved password characters such as `^`, `*`,
`@`, `:`, `/`, `?`, `#`, `%`, and `&` are accepted.

The Swarm deployment expects these external secrets:

```bash
printf 'postgres://permitpal:<url-encoded-password>@192.168.1.2:8432/permitpal?sslmode=disable' \
  | docker secret create permitpal_database_url -

htpasswd -bnBC 12 "" '<app-password>' | tr -d ':\n' \
  | docker secret create permitpal_password_hash -

openssl rand -base64 48 | docker secret create permitpal_session_secret -
```

Production deploys through `home_swarm` as `proxy_permitpal` behind Traefik at `permitpal.bitofbytes.io`. The `permitpal/post-receive` hook in `home_swarm` rolls the service after CI pushes a new image tag to `registry.bitofbytes.io/permitpal`.

## Build and test

```bash
make test
make build
```
