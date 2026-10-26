## ADDED Requirements

### Requirement: Platform-specific worker updates
Windows x64 workers SHALL select transcoderd-worker-windows-amd64.exe for binary updates and SHALL never apply the Linux executable. Existing Linux amd64 release asset names SHALL remain compatible. Binary self-updates SHALL preserve bundled tools; upgrading the toolchain SHALL require replacing the complete bundle.

#### Scenario: Mixed-platform release
- **WHEN** a release contains Linux and Windows worker executables
- **THEN** a Windows x64 worker selects only the Windows x64 executable

#### Scenario: Windows asset absent
- **WHEN** the newer release has no matching Windows executable
- **THEN** no incompatible executable is installed

#### Scenario: Repeated updates with a running supervisor
- **WHEN** the Windows supervisor applies another binary update while its original executable remains mapped
- **THEN** the new executable is installed without colliding with the previous update's backup

## MODIFIED Requirements

### Requirement: Supervisor replacement
With updates enabled, the supervisor SHALL select the release asset corresponding to the running application and platform, download it to temporary storage, and apply it to the executable before running the application.

#### Scenario: Matching asset exists
- **WHEN** a newer release contains the corresponding binary asset
- **THEN** the supervisor downloads and applies it before starting the service

#### Scenario: Matching asset absent
- **WHEN** the release has no matching asset
- **THEN** the update reports an error instead of applying an unrelated asset
