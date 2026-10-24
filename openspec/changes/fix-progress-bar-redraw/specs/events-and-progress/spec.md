# Spec Delta

## ADDED Requirements

### Requirement: Worker console progress redraw
When writing progress to an interactive terminal, the worker SHALL redraw each active progress tracker in place and SHALL remove completed trackers from the active display.

#### Scenario: Active tracker updates
- **WHEN** an action reports another progress update while its tracker is active
- **THEN** the worker updates the existing terminal line instead of appending a new line

#### Scenario: Tracker completes
- **WHEN** an action completes or fails
- **THEN** its tracker is removed from the active progress display
