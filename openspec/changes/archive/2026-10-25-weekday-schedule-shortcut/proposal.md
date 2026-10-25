# Proposal

## Why

Operators need a compact CLI schedule that can keep workers available during cheap weekend electricity while limiting new weekday work to selected hours. The worker already supports weekday selectors through a lower-level setting, but the everyday `--schedule` shortcut only accepts daily windows.

## What Changes

- Extend repeatable `--schedule` values to accept optional weekday selectors in `Days@HH:MM-HH:MM` form while preserving existing daily values.
- Document weekday and weekend scheduling through the CLI and use it in the Ryzen worker GitOps deployment.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration-and-runtime`: define day-selective CLI schedule windows and their compatibility with existing schedule settings.

## Impact

The worker CLI maps `--schedule` values into its existing weekly admission-window configuration. The Ryzen worker GitOps arguments will allow weekday jobs from 22:00 through 10:00 and all-day admission on Saturday and Sunday, using `Europe/Madrid`.
