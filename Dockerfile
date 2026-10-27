# syntax=docker/dockerfile:1

FROM golang:1.26.10-alpine3.23 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go tool templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/permitpal ./cmd/permitpal

FROM alpine:3.23
WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/permitpal ./permitpal
COPY --from=builder /src/static ./static
COPY --from=builder /src/migrations ./migrations

RUN addgroup -S permitpal \
	&& adduser -S -G permitpal permitpal \
	&& chown -R permitpal:permitpal /app

ENV PORT=4600
ENV APP_ENV=production
ENV DATA_STORE=postgres
ENV DATABASE_URL_FILE=/run/secrets/permitpal_database_url
ENV SESSION_SECRET_FILE=/run/secrets/permitpal_session_secret

USER permitpal
EXPOSE 4600
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -q -O - http://127.0.0.1:4600/health || exit 1
CMD ["./permitpal"]
