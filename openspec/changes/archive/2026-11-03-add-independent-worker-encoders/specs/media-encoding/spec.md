# Spec Delta

## ADDED Requirements

### Requirement: Immutable per-job CPU and GPU profiles
The worker SHALL use separate validated CPU and GPU profiles, with SVT-AV1 and NVENC approved defaults for an unspecified secondary class. Explicit class-specific YAML profiles SHALL be supported. Before encoding, the worker SHALL persist its selected class and profile in the job context. Resumed work SHALL use that snapshot. Encoder-specific quality and extraArgs SHALL NOT leak into the other class.

#### Scenario: Simultaneous CPU and GPU jobs
- **WHEN** a GPU-primary worker starts one job on CPU and another on GPU
- **THEN** CPU uses its CPU profile and GPU uses its GPU profile concurrently without shared profile mutation

#### Scenario: Restart after profile selection
- **WHEN** a worker restarts after persisting a selected encoder profile
- **THEN** it resumes that encode with the saved class and parameters even if startup settings changed

#### Scenario: Invalid secondary profile
- **WHEN** a CPU profile selects NVENC or a GPU profile selects a CPU codec
- **THEN** worker startup fails with a configuration error
