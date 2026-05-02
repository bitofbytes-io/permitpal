-- +goose Up
alter table app_profile
  add constraint app_profile_total_hours_max check (total_hours <= 60),
  add constraint app_profile_night_hours_max check (night_hours <= 10);

alter table requirement_items
  add constraint requirement_items_notes_length check (char_length(notes) <= 1000);

-- +goose Down
alter table requirement_items
  drop constraint if exists requirement_items_notes_length;

alter table app_profile
  drop constraint if exists app_profile_night_hours_max,
  drop constraint if exists app_profile_total_hours_max;
