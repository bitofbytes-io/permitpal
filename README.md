# PermitPal

PermitPal is a Go/HTMX dashboard for tracking NC learner permit exam readiness.

Tagline: **Your permit pal—less yelling, more tracking.**

## Local preview without a database

```bash
make dev
```

Open `http://localhost:4600` and sign in with your configured local password.

Set `PERMITPAL_PASSWORD` or `PERMITPAL_PASSWORD_HASH` and `SESSION_SECRET` in
your untracked `local.mk` first. See `local.mk.example` for the expected keys.

The default development mode uses `DATA_STORE=memory`, so dashboard changes persist only while the process is running.

## Production-like local run

```bash
make migrate
make run-postgres
```

Set `DATABASE_URL` in `local.mk` or the environment before running Postgres-backed targets.

## Production database and Swarm setup

PermitPal uses Postgres in production. Create a dedicated database and role before deploying:

```sql
create user permitpal with password '<strong-password>';
create database permitpal owner permitpal;
grant all privileges on database permitpal to permitpal;
```

Apply the schema once from a machine with `goose` and network access to the database:

```bash
make migrate
```

The Makefile normalizes the username and password in `DATABASE_URL` before
passing it to `goose`, so raw reserved password characters such as `^`, `*`,
`@`, `:`, `/`, `?`, `#`, `%`, and `&` are accepted.

The Swarm deployment expects these external secrets:

```bash
docker secret create permitpal_database_url /path/to/database-url

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
