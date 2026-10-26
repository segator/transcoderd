## ADDED Requirements

### Requirement: Portable Windows tool discovery
The Windows worker SHALL discover bundled FFmpeg, ffprobe, mkvextract, PgsToSrt, and OCR data relative to its executable independently of the current directory. Explicit OCR executable and data settings SHALL override their defaults.

#### Scenario: Launch outside the bundle directory
- **WHEN** the worker is launched from a different directory with spaces or non-ASCII characters in its installation or workspace paths
- **THEN** it can execute bundled tools and access bundled OCR data

### Requirement: Windows local worker control
The Windows worker SHALL expose the existing control commands through a named pipe restricted to its current user and SYSTEM. Linux SHALL retain its existing Unix socket and permissions.

#### Scenario: Change Windows admission
- **WHEN** the same user invokes control set-max-jobs or control status
- **THEN** the running Windows worker applies the change or returns its current status

### Requirement: Windows subprocess cancellation
Canceling a worker command SHALL terminate its Windows process tree so encoders and OCR processes cannot continue after the worker's supervising command exits.

#### Scenario: Cancel a command with descendants
- **WHEN** the command context is canceled
- **THEN** the command and its descendants stop and the command returns
