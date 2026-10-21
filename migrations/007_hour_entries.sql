-- +goose Up
-- Recorded totals can exceed the 60/10 credited requirements when a week
-- excludes hours. Retain a separate finite ceiling within numeric(5,1).
alter table app_profile
  drop constraint app_profile_total_hours_max,
  drop constraint app_profile_night_hours_max,
  add constraint app_profile_total_hours_max check (total_hours <= 1000),
  add constraint app_profile_night_hours_max check (night_hours <= 1000);

-- Existing totals have no known entry date. Never infer one from updated_at.
create table hour_entries (
  id bigint generated always as identity primary key,
  driver_id bigint not null references drivers(id) on delete cascade,
  recorded_at timestamptz not null,
  recorded_on date,
  total_tenths integer not null check (total_tenths between 0 and 10000),
  night_tenths integer not null check (night_tenths between 0 and 10000),
  check (recorded_on is null or night_tenths <= total_tenths)
);
create index hour_entries_driver_id_id on hour_entries (driver_id, id);
insert into hour_entries (driver_id, recorded_at, recorded_on, total_tenths, night_tenths)
select driver_id, now(), null, round(total_hours * 10), round(night_hours * 10)
from app_profile;

-- +goose Down
-- Cumulative profile totals and expanded bounds remain intact; restoring the
-- old 60/10 ceilings would reject hours legitimately saved by this release.
-- Only entry-date accounting is removed. Reapplying Up replaces max checks.
drop table hour_entries;
