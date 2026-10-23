# Design

## Context

The release workflow invokes `mage publish:app` separately for the full, minor, and major versions. Mage injects `PROJECT_VERSION` into the binaries and Docker labels, so the `v2` alias is currently a separate build whose worker binary reports `v2`.

## Goals / Non-Goals

**Goals:**

- Build each release image with its full semantic version and publish the release aliases from that same image definition.
- Keep ordinary branch publishing and the full-version release tag intact.

**Non-Goals:**

- Change application self-update behavior or the image's entrypoint.
- Add automatic image pulls to deployed Docker containers.

## Decisions

- Pass release aliases to a single Mage publication invocation while `PROJECT_VERSION` remains the full release version. Mage will publish the version tag, branch tag, and configured aliases from the same Dagger container for each role.
- Represent aliases as a comma-separated environment variable used only by the release workflow. This avoids deriving release versions from Git state inside Mage and keeps normal builds unchanged.
- Add unit coverage for tag list construction so a release version and its aliases are all emitted for both image roles without replacing the canonical version.

## Risks / Trade-offs

- **A malformed alias can produce an unintended tag** → Validate, trim, and ignore empty alias entries; workflow values come directly from Release Please.
- **Registry publication could stop partway through the aliases** → Preserve the full-version tag as the first publication and fail the target on any publish error; rerunning the workflow is safe and idempotent.

## Migration Plan

New release runs publish aliases from the same image content as the full-version tag. Existing moving tags are not rewritten until the next release. Rollback is a workflow revert followed by a release; already published tags remain usable.
