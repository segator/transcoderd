## Context

A worker could hold its event publication mutex while retrying an obsolete event. Neither profile could progress past that step.

## Goals / Non-Goals

Restore current assignments safely and automatically recover from boot-time outages.

## Decisions

Use the existing authenticated job lookup and saved assignment event ID. Resume the saved phase using the current server event sequence. A server failure leaves local work intact and is retried before startup proceeds. Remove expired artifacts only from the matching job workspace.

## Risks / Trade-offs

Recovery waits for the server before starting the job pipeline. Requests and retry delays honor shutdown.
