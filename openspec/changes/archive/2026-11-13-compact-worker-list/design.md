# Design

## Context

The fleet renderer already combines matching Windows profiles and keeps the original worker names for controls, resource history and links. The Workers view polls live data every five seconds and paginates at 12 workers.

## Decisions

- Render each machine as a compact disclosure. Show observed encode counts in the summary and render the existing profile details when opened. Offline rows retain last-seen and reported version information.
- Search and filter the period-scoped list before paginating. An online profile with accepted work can make the machine match Active and a queue or RAM hold on another profile can make the same machine match Paused. A selected resource link uses the existing profile name and takes precedence over list filters.
- Keep expansion and form drafts in the current page so polling can update telemetry without closing details or interrupting an edit. Use the existing grouped fleet data and authenticated APIs.
