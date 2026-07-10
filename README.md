# PermitPal

PermitPal is a self-hosted Go dashboard for tracking progress toward the North Carolina learner permit exam. It supports an in-memory preview and persistent PostgreSQL deployments protected by a shared login.

## Requirements

- Docker 24+
- PostgreSQL 15+ for persistent deployments
- Apache `htpasswd` for generating a production bcrypt password hash
- Goose for database migrations

## Build the image

```bash
make tail-prod
docker build -t permitpal:local .
```

## Configure the application

Generate a bcrypt password hash and a session secret:

```bash
htpasswd -bnBC 12 "" 'choose-a-strong-password' | tr -d ':\n'
openssl rand -base64 48
```

Create an untracked `permitpal.env` file and paste the generated values:

```dotenv
APP_ENV=production
DATA_STORE=postgres
DATABASE_URL=postgres://permitpal:change-me@db:5432/permitpal?sslmode=disable
PERMITPAL_PASSWORD_HASH=replace-with-generated-bcrypt-hash
SESSION_SECRET=replace-with-generated-session-secret
PERMITPAL_USERNAME=driver
PORT=4600
SECURE_COOKIES=false
```

Do not commit this file. Set `SECURE_COOKIES=true` when the application is served over HTTPS.

| Setting | Required | Purpose |
| --- | --- | --- |
| `APP_ENV` | No | Docker defaults to `production`; local runs default to `development` |
| `DATA_STORE` | No | Docker defaults to `postgres`; development defaults to `memory` |
| `DATABASE_URL` | With Postgres | PostgreSQL connection string |
| `PERMITPAL_PASSWORD_HASH` | In production | Bcrypt hash for the shared login |
| `PERMITPAL_PASSWORD` | Development only | Plain-text alternative for local preview |
| `SESSION_SECRET` | Yes | Session-signing secret of at least 32 characters |
| `PERMITPAL_USERNAME` | No | Login username; defaults to `driver` |
| `SESSION_COOKIE` | No | Cookie name; defaults to `permitpal_session` |
| `SECURE_COOKIES` | No | Defaults to `true` in production and `false` in development |
| `PORT` | No | HTTP port; defaults to `4600` |
| `LOG_LEVEL` | No | Application log level; defaults to `info` |

The database URL, password, password hash, and session secret support corresponding `*_FILE` variables. The image defaults to files under `/run/secrets/permitpal_*` for the production secrets.

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
```

Apply migrations before starting PermitPal:

```bash
export DATABASE_URL='postgres://permitpal:change-me@localhost:5432/permitpal?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
```

## Run with Docker

```bash
docker run --rm --name permitpal --network permitpal \
  --env-file permitpal.env \
  -p 4600:4600 \
  permitpal:local
```

Open <http://localhost:4600>. The health endpoint is <http://localhost:4600/health>.

For a disposable preview, override the image's production database secret path:

```bash
docker run --rm -p 4600:4600 \
  -e APP_ENV=development \
  -e DATA_STORE=memory \
  -e DATABASE_URL_FILE= \
  -e PERMITPAL_PASSWORD_HASH_FILE= \
  -e PERMITPAL_PASSWORD=local-password \
  -e SESSION_SECRET=replace-with-a-32-character-or-longer-secret \
  permitpal:local
```

Postgres and migrations are not needed in this mode.

## Development

```bash
cp local.mk.example local.mk
make dev
make test
```

Use `make run-postgres` for a persistent local run and the `make migrate*` targets for database maintenance.
