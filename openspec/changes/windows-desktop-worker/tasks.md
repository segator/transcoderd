# Tasks

- [x] Add thread-safe local progress snapshots and transient drain/graceful shutdown to the existing worker control, with focused tests.
- [ ] Implement the Windows SCM supervisor, owner-scoped pipe, protected configuration, validation, graceful reconfiguration, and tests.
- [x] Implement an attractive web-based Windows tray application for status, active and prefetched jobs, server queue, settings, and admission controls.
- [x] Expose and test the local pipeline queue and an authenticated, bounded read-only server queue view, with graceful fallback for older servers.
- [ ] Implement a guided installer with service registration, safe upgrade/uninstall and per-user tray startup.
- [ ] Integrate the desktop build, installer validation and release asset into Dagger and GitHub CI.
- [ ] Run Go and native Windows validation, strict OpenSpec validation, document deployment, and archive the completed change.
