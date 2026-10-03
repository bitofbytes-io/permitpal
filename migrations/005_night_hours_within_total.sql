-- +goose Up
-- Night hours are part of total hours. NOT VALID enforces the rule on every
-- insert and update without rejecting a row saved before it existed, which
-- keeps its values until the driver next saves progress. When no such row
-- exists, the block below validates the constraint for existing rows too.
alter table app_profile
  add constraint app_profile_night_hours_within_total check (night_hours <= total_hours) not valid;

-- +goose StatementBegin
do $$
begin
  if not exists (select 1 from app_profile where night_hours > total_hours) then
    alter table app_profile validate constraint app_profile_night_hours_within_total;
  end if;
end
$$;
-- +goose StatementEnd

-- +goose Down
alter table app_profile drop constraint if exists app_profile_night_hours_within_total;
