## ADDED Requirements

### Requirement: English operations experience
The operations dashboard SHALL use English for application-provided navigation, labels, charts, controls, login, media exploration and playback, errors, accessibility text and PWA/offline content. Pages and the manifest SHALL declare English, and application-formatted numbers and dates SHALL use an English locale. User-provided names, paths and external diagnostics SHALL remain unchanged.

#### Scenario: Navigate and operate the dashboard
- **WHEN** an operator signs in, monitors jobs, changes capacity or browses and queues media
- **THEN** application-provided labels, feedback and media eligibility or error messages are in English

#### Scenario: Installed or offline dashboard
- **WHEN** an operator installs the dashboard or opens it without a connection
- **THEN** installation metadata, reconnection content and update controls are in English

#### Scenario: User media names
- **WHEN** a filename or worker name contains another language
- **THEN** the UI preserves that value while presenting surrounding application text in English
