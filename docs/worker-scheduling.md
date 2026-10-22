# Schedule new job requests

Each CPU or GPU worker can restrict **new job requests** to operator-selected
hours. Use the same CLI schedule arguments on both workers when they should
follow the same hours. Keep their distinct names, encoders, and concurrency
limits. Restart each worker after changing its arguments.

## Daily 22:00-10:00 on the CLI

Supply these same arguments on each worker's Docker command, after the image:

```bash
-e TZ=Europe/Madrid \
--schedule=22:00-10:00
```

The worker accepts new jobs every day from 22:00 inclusive until 10:00
exclusive, including weekends. It stays online and sends heartbeats when
admission is closed. Requests resume on the normal five-second poll when the
next window opens and capacity is available. No daily cron task or restart
is needed. Restart the worker when changing its CLI arguments.

Use `--schedule=00:00-18:00` for a daily 18:00 cutoff. Repeat `--schedule`
to allow multiple periods, for example `--schedule=22:00-10:00
--schedule=14:00-16:00`. `--schedule=` clears configured windows. The shortcut
replaces the schedule windows from YAML if both are supplied. Existing
`worker.schedule.windows` configuration remains supported, but the Docker
examples use CLI arguments and need no config mount. Advanced weekly selectors
are available through `--worker.schedule.windows=Mon-Fri@22:00-10:00`.

The shortcut uses the worker process's local timezone. In Docker, set `TZ` to
an IANA timezone such as `Europe/Madrid`; it is not hardcoded in the worker.
The windows are operator-supplied hours, not automatic electricity-tariff
selection. There is no price lookup or public-holiday calendar.

## Window rules

- Syntax: daily `HH:MM-HH:MM` or `Days@HH:MM-HH:MM`, with 24-hour clock times and minute precision.
- Days: `*`, `Mon`, `Tue`, `Wed`, `Thu`, `Fri`, `Sat`, `Sun`, or an inclusive
  range such as `Mon-Fri`. Names are case insensitive. A range can wrap through
  Sunday, so `Fri-Mon` covers Friday, Saturday, Sunday, and Monday.
- Multiple entries are combined with OR. Overlapping entries are allowed.
- The start is inclusive and the end exclusive. A 10:00 end closes admission
  at exactly 10:00; the normal request poll runs approximately every five seconds.
- `24:00` is accepted only as an end. Use `00:00-24:00` for a whole day.
  Equal endpoints, such as `00:00-00:00`, are rejected.
- Overnight entries belong to their starting day. `Fri@22:00-08:00` permits
  Friday night and Saturday morning until 08:00.
- Use an IANA zone such as `Europe/Madrid`, `Atlantic/Canary`, or `UTC`.
  The default is `Local`, which follows the worker process timezone and is
  often UTC in containers. Explicit zones use bundled timezone data without
  a host timezone mount. Admission follows the civil clock across DST:
  repeated autumn hours have the same policy both times; missing spring
  hours never occur.
- No windows means no extra time restriction. Pause and `maxActiveJobs` still
  apply inside permitted hours.
- Legacy `worker.startAfter` / `worker.stopAfter` remain available with their
  existing duration semantics, such as `8h` and `22h`. Do not combine active
  legacy fields with weekly windows; startup rejects the conflict. Invalid
  zones and window syntax also fail before contacting the server.

## Accepted work finishes

Closing a window does not cancel or suspend an encode. All previously accepted
work continues, including downloaded or locally queued jobs and work recovered
from a previous worker run. A request already in flight at the boundary may
also receive its assignment afterward. The schedule controls when a worker
initiates its next request.

For example, if four GPU jobs are accepted at 09:59 and the window closes at
10:00, those jobs finish normally after 10:00. This can use electricity outside
the preferred periods. To reduce that overrun, close admission earlier or lower
`maxActiveJobs`. This feature is not a guarantee of zero encoding outside the
windows and does not predict each movie's completion time.

See [worker deployment](worker-deployment.md) for separate four-GPU/two-CPU
configuration, and [the CPU/GPU measurements](av1-cpu-preset-comparison.md) for
encode-time evidence. There is no mixed-load or wall-plug energy measurement yet.
