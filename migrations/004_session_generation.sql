-- +goose Up
-- Logout bumps the driver's generation, which ends every session cookie
-- issued before it. Cookies from before this migration carry no generation
-- and count as generation 0, so they stay valid until the driver logs out.
alter table drivers add column session_generation bigint not null default 0 check (session_generation >= 0);

-- +goose Down
alter table drivers drop column session_generation;
