# Design

## Context

The upload attempt is created with restrictive permissions, then linked through staging to the final media path. The final path therefore retains the upload inode's original mode.

## Goals / Non-Goals

**Goals:** Ensure finalized media files are readable by SMB users and writable by the owning group.

**Non-Goals:** Change directory ownership, directory permissions, SMB configuration, or permissions on source media.

## Decisions

- Set mode `0664` on the final path after no-clobber publication and before updating the job record. This also covers legacy staged uploads and idempotent completion retries without changing staging security while a body is still being received.
- Treat a chmod failure as a finalization failure. Preserve the source and do not persist the completed event; a retry can re-apply permissions.
- Do not grant `0666`: world-writable media files would allow any local user to modify library content. Group write requires the media directory/container to provide the intended shared group ownership.

## Risks / Trade-offs

- [The server process may lack permission to change a file's mode] → Finalization returns an error and retains the source rather than reporting success with inaccessible output.
- [Mode bits alone cannot grant access through a non-traversable parent directory or override restrictive ACLs] → Directory/ACL configuration remains an operator responsibility and is outside this change.

## Migration Plan

Newly finalized files receive the shared mode. Existing completed outputs are not modified retroactively. Rollback consists of reverting the server change; it does not require a data migration.
