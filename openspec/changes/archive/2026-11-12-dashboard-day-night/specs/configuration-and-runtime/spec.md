# Configuration and Runtime Delta

## ADDED Requirements

### Requirement: Local worker dashboard appearance
The shared local worker dashboard SHALL provide Day and Night themes in the browser and Windows tray. Day SHALL remain the first-visit default. The dashboard SHALL remember the selected theme when reopened in the same browser or tray profile.

#### Scenario: Return with Night selected
- **WHEN** a user switches to Night and reloads or reopens the local worker dashboard
- **THEN** the dashboard renders Night and allows switching back to Day
