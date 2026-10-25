# Design

## Context

The worker already parses day selectors in `worker.schedule.windows`; the repeatable `--schedule` shortcut passes its values through to that same parser. The limitation is that the shortcut help and operational docs describe only daily windows, and no test covers day selectors through the shortcut.

## Goals / Non-Goals

**Goals:** Make day-selected windows an explicit, tested, documented part of `--schedule`, and configure Ryzen for weekday nights plus all-day weekends.

**Non-Goals:** Change the admission evaluator, alter existing daily window behavior, or add tariff/holiday lookup.

## Decisions

- Reuse the existing `Days@HH:MM-HH:MM` grammar and admission evaluator rather than introducing a second parser. The selector is already supported by the underlying schedule configuration.
- Keep `--schedule` repeatable and retain local process timezone behavior. This preserves current CLI usage and lets operators express weekday/weekend policies without YAML configuration.
- Add a regression test at the CLI-to-config boundary to prevent the shortcut from accidentally rejecting or rewriting day selectors.

## Risks / Trade-offs

- A malformed selector continues to fail worker configuration validation before it requests jobs; tests cover the accepted schedule policy.
- Existing workers keep their current schedule until their deployment arguments are updated and reconciled.
