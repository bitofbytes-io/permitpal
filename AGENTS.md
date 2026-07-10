# Agent Guidance

- Edit Templ and CSS sources rather than generated `*_templ.go` or `static/styles.css` output; regenerate affected artifacts before committing.
- Keep the memory store as an ephemeral preview path and use Postgres behavior as the production reference.
- The Docker image sets production database and password-hash secret paths; memory-mode container runs must explicitly clear both.
- Production authentication must use a bcrypt password hash and a session secret of at least 32 characters.
- Preserve secure-cookie defaults: enabled in production and disabled only for local HTTP development.
- Run `make test` and exercise both storage modes when changing repository or configuration behavior.
