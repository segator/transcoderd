# Design

## Context
The current queue stop holds queued encodes. Maintenance drain permanently holds admission. Shutdown needs accepted queued work to finish and a reversible admission hold.

## Goals / Non-Goals
Provide a one-time Windows shutdown request. No scheduled shutdown, shutdown on ordinary idle or browser-hosted power management.

## Decisions
Use a dedicated desktop command `stop-after-current` carrying `shutdownAfterCurrent`. Only a positive answer arms a transient supervisor flag. An explicit reversible worker finishing control closes admission and resumes accepted work including queued and individually paused encodes. Both profiles must acknowledge the hold and report no active/reserved jobs before a fixed Windows shutdown command runs once. Missing status never means idle. Resume queue and other lifecycle transitions cancel the flag. Shutdown errors disarm and surface a message. No forced application termination. Confirmation uses a native dialog in WebView2 and tray, avoiding suppressed browser dialogs. Browser-hosted controls retain ordinary queue behavior.

## Risks / Trade-offs
A worker exit or lost control cancels shutdown rather than guessing completion. Other applications can prevent normal Windows shutdown. Tests inject power operations and never shut down the test machine.
