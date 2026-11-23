## ADDED Requirements

### Requirement: Recover stalled video encoding
The worker SHALL detect FFmpeg video progress stalls after 15 minutes, excluding intentional pauses. It SHALL terminate and restart the affected encoding once; on a second stall, it SHALL report failure and release the slot. The server SHALL apply its existing bounded retry policy. Job cancellation SHALL NOT cause a retry.

#### Scenario: Encoder stalls
- **WHEN** a running encoder makes no video frame or output timestamp progress for 15 minutes
- **THEN** the worker terminates that encoder and starts the same encoding again

#### Scenario: Encoder is intentionally paused
- **WHEN** an encoding process is paused by the worker or individually
- **THEN** the paused time does not trigger stalled encoding recovery

#### Scenario: Encoder stalls again
- **WHEN** the restarted encoding also makes no video progress for 15 minutes
- **THEN** the worker reports a descriptive failure, releases the encoding slot, and the server applies its existing bounded retry policy

#### Scenario: Other encodes are running
- **WHEN** an encoder is terminated due to stalled progress
- **THEN** unrelated encoding processes continue running

#### Scenario: Encoding is cancelled
- **WHEN** the job is cancelled
- **THEN** the watchdog does not retry the cancelled encoding
