# Agent Guidance

- Edit Templ sources rather than generated `*_templ.go` output; regenerate affected artifacts before committing. `static/styles.css` is hand-written and served as-is, with no CSS build step.
- Keep the memory store as an ephemeral preview path and use Postgres behavior as the production reference.
- The Docker image sets production database and session-secret file paths; memory-mode container runs must clear `DATABASE_URL_FILE` and pass `SESSION_SECRET` directly.
- Accounts come only from `PERMITPAL_USERS` or `PERMITPAL_USERS_FILE` as `username:bcrypthash` lines; sessions need a secret of at least 32 characters.
- Preserve secure-cookie defaults: enabled in production and disabled only for local HTTP development.
- Run `make test` and exercise both storage modes when changing repository or configuration behavior.
