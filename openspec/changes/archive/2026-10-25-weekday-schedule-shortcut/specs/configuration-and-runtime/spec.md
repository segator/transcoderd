# Spec Delta

## ADDED Requirements

### Requirement: Day-selected CLI admission windows
The worker SHALL accept repeatable `--schedule` values in daily `HH:MM-HH:MM` or day-selected `Days@HH:MM-HH:MM` form. Values SHALL use the configured process timezone, combine with OR semantics, and replace YAML admission windows when the flag is supplied.

#### Scenario: Weekday overnight window and full weekends
- **WHEN** the worker is configured with `--schedule=Mon-Fri@22:00-10:00` and `--schedule=Sat-Sun@00:00-24:00`
- **THEN** it accepts new jobs on weekday nights through 10:00 and all day Saturday and Sunday, and rejects new jobs during the remaining weekday hours

#### Scenario: Daily shorthand remains supported
- **WHEN** the worker is configured with `--schedule=22:00-10:00`
- **THEN** the daily overnight window retains its existing behavior

#### Scenario: Repeated windows combine
- **WHEN** several daily or day-selected `--schedule` values are supplied
- **THEN** a new job request is allowed when any supplied window matches the current local day and time
