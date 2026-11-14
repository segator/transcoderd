# Proposal

## Why

The fleet Workers view shows every worker's tasks, charts and settings at once. On phones, an operator has to scroll through full cards to find a machine or compare available encoding capacity.

## What Changes

- Show a compact expandable row for each machine, with its state and reported CPU and GPU encode counts.
- Add a worker name search and All, Active, Paused and Offline filters.
- Keep tasks, resource charts and per-profile controls inside the row, with direct resource links still opening the relevant worker.

## Capabilities

### Modified Capabilities

- `operations-dashboard` covers compact worker summaries, filtering and expanded worker details.

## Impact

The fleet frontend and browser regression suite.
