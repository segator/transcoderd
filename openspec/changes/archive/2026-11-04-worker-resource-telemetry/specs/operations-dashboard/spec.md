## ADDED Requirements

### Requirement: Worker hardware summary and drill-down
Overview worker rows SHALL show concise measured CPU, GPU and encoder usage and link to that worker in the Workers view. Details SHALL retain worker controls and show available CPU/RAM history, per-device GPU/encoder history, VRAM, temperature, power, identities and sample freshness. Host/device scope and shared usage SHALL be explicit; encoder slots SHALL remain distinct from hardware utilization.

#### Scenario: Open worker from overview
- **WHEN** an operator clicks or keyboard-activates a worker summary
- **THEN** the Workers view selects that exact worker, shows its resource details and offers a route back to all workers

#### Scenario: Missing or stale sample
- **WHEN** measurements are missing, invalid or more than twenty seconds old, or the worker is offline
- **THEN** the UI identifies unavailable or last-reported readings instead of presenting zero or stale data as live usage

#### Scenario: Bounded resource history
- **WHEN** dashboard polling receives distinct resource samples
- **THEN** charts retain at most 120 samples/ten minutes in page memory, ignore duplicates, preserve gaps for missing data and write no private resource history to persistent browser storage
