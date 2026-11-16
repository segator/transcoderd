# Design

## Context

The Windows desktop receives installed and authenticated pending versions from the service. Release notes are GitHub release metadata, not signed installation metadata. The dashboard's browser-hosted mode has a different update lifecycle.

## Goals / Non-Goals

**Goals:** Show relevant notes without affecting worker admission or installation, and report missing data honestly.

**Non-Goals:** Change eligibility, delay installation, or interpret notes as authenticated installer metadata.

## Decisions

The desktop WebView requests public GitHub releases only when the user opens or refreshes the panel. It sends no credentials or referrer, filters stable semantic versions between installed (exclusive) and pending (inclusive), and displays release bodies as text. With no pending target it selects the installed release. The CSP permits only the GitHub API origin in addition to the existing local origin. The browser dashboard's server-side CSP remains unchanged. An unknown target or missing history is explicitly reported. Pagination and request time are bounded; failures never alter update state. Status changes reload an open panel and stale responses are ignored.

Alternatives considered: embedding the changelog cannot describe future releases; fetching from the service would couple nonessential metadata to privileged update processing. Showing only the latest release misses intermediate changes.

## Risks / Trade-offs

- GitHub rate limits, offline operation or bounded history can leave notes unavailable or partial; the panel reports this and can be refreshed.
- Release bodies are untrusted and not signed by the update manifest; render as text and never use them for installation decisions.
