# Design

## Context
The completed transaction is retained to protect recovery evidence. On each Step the manager loads it before processing the current pending candidate. Version equality with the running installation alone does not establish that it belongs to that candidate.

## Goals / Non-Goals
Prevent stale completion from releasing a newer admission barrier. Preserve recovery after an interrupted commit and retain existing transaction evidence. No installed journal rewriting or worker interruption.

## Decisions
Compare the completed signed manifest version with the pending target and, when present, require exact signed candidate bytes/signature equality before ClearCompleted. Candidate URLs may change without changing identity. A mismatch continues the normal pending flow. Reproduce with two successive signed fixture releases, a completed first transaction and an accepted job while the second downloads.

## Risks / Trade-offs
An already installed old coordinator needs a bootstrap upgrade or a controlled idle check; a later release cannot retroactively fix its running code. Production diagnostics use only the restricted status channel, without exposing tokens.
