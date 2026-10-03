-- +goose Up
-- Migration 001 seeds Caleb's original tracker into every new database, and
-- 003 assigns it to the caleb driver. Released migrations cannot change, so
-- remove that seed here, but only while creating a database: goose applied
-- 001 within the last minute, in this same run, so nobody can have signed in
-- to the seeded tracker yet. As a second check, no seeded row may have been
-- saved since 001 wrote them all in one transaction. Databases migrated
-- earlier, including the original install, keep their data, as does SQL
-- applied without goose_db_version.
-- +goose StatementBegin
do $$
declare
  caleb_id bigint;
begin
  if to_regclass('goose_db_version') is null then
    return;
  end if;
  if not coalesce((
    select min(tstamp) > localtimestamp - interval '1 minute'
    from goose_db_version
    where version_id = 1 and is_applied
  ), false) then
    return;
  end if;
  select id into caleb_id from drivers where username = 'caleb';
  if caleb_id is null then
    return;
  end if;
  if exists (
    select 1 from app_profile
    where driver_id = caleb_id and permit_issue_date = '2025-07-24' and total_hours = 34.5 and night_hours = 6.0
  ) and (
    select count(distinct updated_at) from (
      select updated_at from app_profile where driver_id = caleb_id
      union all
      select updated_at from requirement_items where driver_id = caleb_id
    ) seeded
  ) = 1 then
    -- Cascades to the seeded profile and checklist; a caleb login then starts fresh.
    delete from drivers where id = caleb_id;
  end if;
end
$$;
-- +goose StatementEnd

-- +goose Down
-- Removing the unused seed is not undone.
