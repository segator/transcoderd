# Spec Delta

## ADDED Requirements

### Requirement: Observe worker UI changes and pause states
Worker heartbeats SHALL include observed queue/RAM holds, suspended encode count, individual job pause state and finishing/draining state. The fleet dashboard SHALL distinguish these from offline, schedule and desired-limit acknowledgement states. Observed updates SHALL appear through its normal refresh without manually reloading. Paused encodes SHALL retain progress without a live-running ETA claim.

#### Scenario: Individual encode paused locally
- **WHEN** an operator pauses an active encode in the worker UI
- **THEN** the next heartbeat and fleet refresh identify that encode as paused without treating it as failed or completed

#### Scenario: RAM pause differs from queue stop
- **WHEN** an operator toggles RAM hold while positive limits remain configured
- **THEN** the dashboard identifies RAM pause and suspended encodes independently of the desired CPU/GPU values

#### Scenario: Remote change reaches local worker UI
- **WHEN** the fleet dashboard saves new limits and the worker synchronizes them
- **THEN** local worker controls and Windows profile status show the observed limits without restarting
