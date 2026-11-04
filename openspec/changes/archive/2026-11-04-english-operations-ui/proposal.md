# Proposal

## Why
The server dashboard currently mixes Spanish and English, while the worker UI is English. The operator requires a consistent English experience throughout the application.

## What Changes
- Translate server dashboard navigation, charts, worker controls, job states, media intake and playback, login and offline/PWA copy into English.
- Use English formatting and page/manifest language metadata; translate server-generated media explanations and errors.
- Preserve user-provided filenames, paths, worker names and external diagnostic output.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `operations-dashboard`: require English throughout the operations UI, including the media library and PWA.

## Impact
Embedded dashboard assets, media API/library messages and their existing assertions. No new dependencies, protocol fields, authentication or scheduling changes.
