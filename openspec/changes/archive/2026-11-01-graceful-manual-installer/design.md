# Design

## Context
Existing `prepare-upgrade` checks CPU before GPU, returning early on active CPU work. The installer invokes the old installed helper. Both defeat the operator's attempt to install safely without manually stopping jobs.

## Goals / Non-Goals
Wait for complete accepted work with visible progress and retain configuration. Do not force terminate jobs, reboot, arm power-off or install on uncertain worker state. Uninstall keeps its existing immediate refusal policy.

## Decisions
Setup runs its own extracted helper with `--prepare-upgrade-wait` through live installer log output. The helper first requests legacy preparation to cancel any pending on-demand PC shutdown, tolerating only its known busy response. It then polls authoritative desktop status, sends the existing `hold-admission` command to both profiles, and waits for zero jobs, queued entries, encodes and reserved requests. This works with installed older workers using their existing control protocol. Only then does it invoke legacy preparation and stop the service. The wait is bounded to 24 hours; cancellation/timeouts fail without stopping work or replacing files. If preparation aborts after holding work, admission stays closed and accepted work can finish; settings remain saved for later restart. Automatic pending updates retain precedence.

Service preparation also checks/drains every profile before reporting busy, and missing configured profiles cannot imply idle. No new persisted setting or network endpoint is needed.

## Risks / Trade-offs
Setup may remain open for a long encode; its details show remaining work. The old service need not implement new commands because the new helper coordinates its existing controls. An unavailable profile fails safely rather than guessing completion.
