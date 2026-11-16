-- +goose Up
-- The optional road-test appointment date. Existing rows keep no appointment,
-- so the previous release keeps working on the migrated schema.
alter table app_profile add column road_test_date date;

-- +goose Down
alter table app_profile drop column road_test_date;
