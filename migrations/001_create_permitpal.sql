-- +goose Up
create table if not exists app_profile (
  id integer primary key default 1,
  permit_issue_date date,
  total_hours numeric(5, 1) not null default 0,
  night_hours numeric(5, 1) not null default 0,
  updated_at timestamptz not null default now(),
  constraint app_profile_singleton check (id = 1),
  constraint app_profile_total_hours_nonnegative check (total_hours >= 0),
  constraint app_profile_night_hours_nonnegative check (night_hours >= 0)
);

create table if not exists requirement_items (
  key text primary key,
  title text not null,
  description text not null default '',
  status text not null default 'needs_practice',
  mastered_date date,
  notes text not null default '',
  sort_order integer not null unique,
  updated_at timestamptz not null default now(),
  constraint requirement_items_status_valid check (status in ('needs_practice', 'mastered'))
);

insert into app_profile (id, permit_issue_date, total_hours, night_hours)
values (1, '2025-07-24', 34.5, 6.0)
on conflict (id) do nothing;

insert into requirement_items (key, title, description, status, mastered_date, notes, sort_order)
values
  ('starting-the-car', 'Starting the car', 'Adjust seat, buckle seat belt, adjust mirrors, start smoothly.', 'mastered', '2026-03-08', 'Smooth startup routine.', 1),
  ('posture', 'Posture', 'Sit at least 10 inches from the wheel with clear visibility.', 'mastered', '2026-03-11', 'Check mirrors before moving.', 2),
  ('forward-movement', 'Forward movement', 'Signal before pulling into traffic, accelerate smoothly, hold lane position.', 'mastered', '2026-03-18', '', 3),
  ('traffic-lights', 'Traffic lights', 'Observe signals, stop smoothly behind the line, start promptly on green.', 'mastered', '2026-03-25', '', 4),
  ('stop-signs', 'Stop signs', 'Observe signs early, stop completely, check traffic before proceeding.', 'mastered', '2026-04-02', '', 5),
  ('yield-caution-lights', 'Yield signs and caution lights', 'Adjust speed, check traffic flow, yield to vehicles with right of way.', 'needs_practice', null, 'Needs calmer speed adjustment.', 6),
  ('lane-changes', 'Lane changes', 'Signal in advance, check mirrors, look over shoulder, change smoothly.', 'needs_practice', null, 'Practice on multilane roads.', 7),
  ('turn-lanes', 'Use of turn lanes', 'Signal in advance, check mirrors and shoulder, enter turn lane properly.', 'needs_practice', null, '', 8),
  ('left-right-turns', 'Left and right turns', 'Signal, check traffic, leave space, stay in correct lane, avoid cutting corners.', 'needs_practice', null, 'Good right turns; left turns need consistency.', 9),
  ('backing', 'Backing', 'Look over shoulder through rear window, back slowly and smoothly in a straight line.', 'needs_practice', null, 'Practice driveway backing.', 10),
  ('parking', 'Parking', 'Signal into space, park between lines, pull completely into space.', 'mastered', '2026-04-12', '', 11),
  ('three-point-turn', 'Three point turn / turn about', 'Signal, stop, check mirrors and shoulders, complete turn safely without rushing.', 'needs_practice', null, 'Next practice focus.', 12),
  ('driver-courtesy', 'Driver courtesy', 'Show patience, maintain distance, avoid aggressive driving.', 'mastered', '2026-04-20', 'Calm and steady.', 13)
on conflict (key) do nothing;

-- +goose Down
drop table if exists requirement_items;
drop table if exists app_profile;
