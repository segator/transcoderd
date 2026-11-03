# Design

## Decisions
The server persists desired limits by worker name. Both local controls and the fleet UI write the same settings, with last committed writes winning; worker polling converges every five seconds. Local controls serialize with polling and apply only after successful persistence. Heartbeats report observed state without writing desired settings. Managed profiles read persisted settings on startup and use their service defaults locally only when no override exists.

Retain current main's queue stop: zero stops assignment and new FFmpeg starts while running/downstream work finishes. RAM and individual holds remain independent and survive capacity changes. Maintenance/finish holds cannot be reopened by the next poll. Dual encoders retain per-class targets, with one extra pipeline job. Windows continues to supervise separate CPU/GPU-named profiles; live profile limits drive its displayed settings.

Add authenticated local pair requests and local-only CLI controls. Publish bounded job pause details plus queue/RAM/finishing/draining flags in existing heartbeat JSON. Display distinct pause badges and omit live ETA for held encodes. Offline data stays explicitly last-observed. No server-to-worker inbound networking or plaintext credentials in URLs are introduced.

## Verification
Exercise actual local HTTP controls -> worker client -> real server routes/repository -> heartbeat -> dashboard snapshot, and server writes -> synchronization -> local status. Cover failures, canceled/maintenance holds, pair preservation through RAM controls, managed restart precedence, queued encode holds and Windows compilation. Repeat relevant browser tests and all Go/race/lint/spec checks before creating a non-draft PR.
