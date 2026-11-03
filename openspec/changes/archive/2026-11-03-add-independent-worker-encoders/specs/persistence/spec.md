# Spec Delta

## ADDED Requirements

### Requirement: Atomic per-worker encoder limit pairs
The server SHALL persist optional non-negative `cpuJobs` and `gpuJobs` together by stable worker name. Incomplete pairs, negative values, mixed single/pair inputs, and integer-overflowing sums SHALL be rejected without mutation. Legacy writes SHALL clear the pair. Legacy clients without independent-encoder capability SHALL read aggregate zero when a pair exists.

#### Scenario: Pair survives server restart
- **WHEN** CPU 2 and GPU 3 are saved for a worker and read through a fresh repository
- **THEN** both values are restored together and another worker remains unchanged

#### Scenario: Invalid pair
- **WHEN** a request contains only one class limit, a negative value, or an overflowing sum
- **THEN** the server returns an input error and keeps the previously stored settings

#### Scenario: Old worker reads a dual override
- **WHEN** a client without the independent-encoder capability reads a stored pair
- **THEN** it receives aggregate zero and drains accepted work instead of assigning all slots to its only encoder
