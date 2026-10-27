# Windows desktop worker

## Mandatory complete updates

The service checks stable releases at startup and hourly, independently of the
tray and job schedule. A complete release must include signed Windows update
metadata and its matching installer. Existing installations need one manual
upgrade to the first installer containing this updater; subsequent complete
releases are installed automatically without confirmation or deferral.

As soon as a complete update is authenticated, both profiles stop initiating job
requests. The installer downloads while all accepted downloads, subtitle work,
prefetched encodes, verification and uploads finish. A persistent amber update
panel appears on every dashboard page. Configuration fields, save, token display
and pause/resume are locked in both the UI and service, including across page
reloads or service disconnects. Job progress stays visible. No client can choose
an arbitrary update URL or executable through the local control API.

The public `update` status contains `pending`, `phase`, `installedVersion`,
`targetVersion`, `downloaded`, `total`, `lastCheck`, `nextRetry`, `attempts` and an
English `message`; it contains no credentials. The restricted `check-update`
command requests a check without changing the mandatory policy. `configure`
and `set-max-jobs` return a maintenance error while `update.pending` is true.

The helper stages files under `%ProgramData%\TranscoderD\updates`, protected for
SYSTEM and administrators, keeps a verified installation backup and coordinates
silent Setup, service restart and the original user's tray. It preserves the
token, limits, schedule, pause and working directory. Failures restore the
previous installation in maintenance and retry after 15 minutes, one hour and
then at most six hours; new work remains blocked until the update succeeds.
The protected recovery task handles interrupted installations. Windows is never
rebooted automatically; restart-required state remains visible until recovery
can verify the installation after a restart. Recovery diagnostics are retained
in `updates\recovery.log` and the staged `installer.log`.

Release metadata is Ed25519 signed; this is separate from Authenticode code
signing of the installer. The pinned public key is in
`update/managed/release-key.pub`. The matching private key exists only in the
GitHub encrypted `WINDOWS_UPDATE_SIGNING_KEY` secret. Missing or mismatched keys
fail update publication. Rotation requires a reviewed transitional release that
accepts both keys before switching signing keys; replacing the single pinned key
directly would leave older clients unable to authenticate an update.

The Windows setup package installs the same worker and media tools as the portable bundle, an automatic Windows service, and a tray application whose interface runs locally in WebView2. Install as the Windows user who will use the tray. The application is installed under `%ProgramFiles%\TranscoderD`, where standard users cannot replace the service binaries. The installer requires administrator permission for service registration and installs the Microsoft Visual C++ runtime and, if missing, WebView2 Runtime (which may require internet access). The user can launch TranscoderD from the Start menu; the tray application also starts at login. Closing the window leaves the service and accepted jobs running.

Enter the server URL, access token, a worker name, an optional working directory, and optional admission windows. The working directory must be an absolute path on a local Windows drive accessible to the service; by default it is `%ProgramData%\TranscoderD\work`. Each worker profile creates its own workspace beneath that directory. Changing it waits for accepted work to finish before restarting the workers. CPU and GPU encoders have separate concurrency controls: for example, select SVT-AV1 with 1 CPU encode and AV1 NVENC with 4 GPU encodes. Set either profile to zero to disable it. Both profiles draw from the same server queue, so either may receive any title depending on which requests it first; the server does not route by encoder. Schedule syntax is documented in [worker-scheduling.md](worker-scheduling.md). The Pause button stops new admissions for both profiles while accepted jobs finish. Connection, codec, directory, or schedule changes wait for accepted jobs to finish before restarting the workers. An upgrade or uninstall refuses to stop the service while jobs are active; retry after they finish.

The dashboard preserves each assigned source filename and shows local pipeline activity with its CPU or GPU profile, separately from the server's unassigned queue. Downloads show received and total bytes, average transfer speed, elapsed time and ETA. FFmpeg encodes show frame count, encoding FPS and speed, elapsed time and ETA. Subtitle track extraction and PGS OCR appear as distinct phases. Encode counts are aggregated across both workers. The global queue requires a server containing `GET /api/v1/jobs/queued`; with older servers the worker remains usable and the dashboard marks the global queue unavailable. The endpoint uses the existing bearer token, returns at most 100 filenames and an aggregate count, and does not expose server paths.

Settings, the token, temporary files and the worker log are under `%ProgramData%\TranscoderD`, restricted to SYSTEM and administrators. The tray verifies that its local pipe is owned by SYSTEM before sending settings. The tray never receives the saved token back; leaving the token field empty retains it. Uninstall keeps this directory to preserve settings and accepted job data. The tray can be closed independently; the service runs without a logged-in user. Installer and binaries are unsigned development builds unless a release explicitly states otherwise.

For isolated development validation, `scripts/windows/test-desktop-pipe.ps1` exercises the supervisor without installing a service. GitHub Windows tests install the package on a disposable runner and run a CPU SVT-AV1 encode against a local test server. The local Dagger E2E can instead select the desktop supervisor using `WINDOWS_DESKTOP_SERVICE` and `WINDOWS_DESKTOP_WORKER`, with `WINDOWS_BUNDLE` pointing at a portable bundle; it does not use production settings.


## Isolated VM reboot drill

Before broad unattended rollout, use a disposable Windows VM, a snapshot and an
isolated test server/token. Never perform this drill on a production worker.

1. Install the updater-enabled version as a regular installing administrator.
   Set distinct CPU/GPU limits, a schedule and working directory. Record the
   installing SID and verify the tray and service report that version.
2. Publish a signed newer test release, submit a CPU fixture and wait for the
   mandatory banner. Confirm both profiles stop requesting work, configuration
   is locked, the existing output uploads, and the recovery task runs as SYSTEM.
3. Force a VM power loss after `updates/transaction.json` reaches `installing`
   or `verifying`. Reboot with automatic logon disabled. From VM administration,
   confirm the recovery task restores/verifies the installation without a logged
   in user and the service keeps admission blocked until successful verification.
4. Log in as the original owner. Confirm pipe access, tray relaunch/autorun,
   retained credentials, limits, pause/schedule and workspace. Retry with a locked
   tray, a failed prerequisite and a restart-required installer. Collect the
   transaction, recovery and installer logs before restoring the VM snapshot.

Hosted CI invokes the actual SYSTEM recovery task and simulates interrupted and
restart-required journal states. It does not reboot its hosted runner or prove
the boot trigger/no-logon scenario; the VM drill remains the rollout gate for
that environment-dependent behavior.
