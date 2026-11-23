## ADDED Requirements

### Requirement: Focus the activity dashboard on local work
The dashboard SHALL provide mutually exclusive local activity filters, including Prefetch for locally queued jobs. It SHALL show the source file size for encodes when known. It SHALL retain the server queue count without displaying a detailed server queue list, giving local activity the available width.

#### Scenario: Prefetched job
- **WHEN** a locally accepted job is queued
- **THEN** it appears under Prefetch and not under Encoding

#### Scenario: Selecting an activity filter
- **WHEN** a user selects an activity filter
- **THEN** the previous filter is deselected and only the selected category is shown

#### Scenario: Encoding statistics
- **WHEN** an encode has a known source file size
- **THEN** its activity statistics include that size

#### Scenario: Server queue
- **WHEN** server queue information is available
- **THEN** its aggregate count remains visible without a server queue job list
