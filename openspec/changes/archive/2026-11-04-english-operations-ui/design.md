# Design

## Context
The server embeds static HTML/JavaScript and exposes library eligibility and operation messages through its authenticated API. The shared worker UI is already English.

## Goals / Non-Goals
Translate application-owned copy consistently without altering scheduling, media selection, identifiers or user data. Localization infrastructure and user-selectable languages are outside this change.

## Decisions
Use direct English copy and an explicit en-US locale to match the existing single-language product. Translate library/API explanations at their source because the browser renders them directly. Keep filenames, configured root/worker names and external tool diagnostics intact. Existing content-derived PWA versioning publishes translated offline assets through the normal explicit update flow.

## Risks / Trade-offs
Missed dynamic or accessibility strings are addressed by source inventory and browser checks across login, all views, playback and offline pages. Existing assertions for translated text must be updated while retaining their behavioral checks.
