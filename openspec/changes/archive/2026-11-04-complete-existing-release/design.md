# Design

## Context

See proposal.md. The existing pipeline owns Linux and Windows validation, but its publication gate depends on a new Release Please result. Existing release tags must remain untouched.

## Goals / Non-Goals

Goals: recover incomplete stable Windows distributions using the existing validation and protected signing key. Non-goals: replace authenticated Windows payloads, retag source, rebuild existing Linux distributions, or change runtime update behavior.

## Decisions

Add release_tag to workflow_dispatch instead of duplicating the Windows pipeline. A read-only source-resolution job validates main dispatch, stable published release, tag ancestry and version.txt, then passes an immutable commit SHA to every build checkout. Normal events preserve default checkout semantics. Offline tests exercise rejected release sources.

The existing Linux and native Windows validation gates run on the selected source with the exact stable version. Before publication, recheck the tag identity and stable release. Refuse an already signed Windows payload. Upload validated executable/ZIP/installer first, manifest next and signature last. This keeps a partially uploaded release ineligible for automatic updates. Unsigned partial assets can be replaced on retry; authenticated ones cannot.

## Risks / Trade-offs

- Rebuilding installers can change their bytes → refuse mutation after a signature is published.
- Tag moves during the build → re-resolve and compare immediately before publication.
- Recovery takes a complete build and native test run → retain all ordinary gates and shared Dagger cache.

## Migration Plan

Merge the automation change, dispatch from main with the existing tag, validate uploaded asset digests and signed metadata, and update release notes. Workflow failure leaves the release unsigned and safely retryable. Never move the release tag.
