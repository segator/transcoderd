# Schedule new job requests

Each CPU or GPU worker can restrict **new job requests** to weekly time windows.
Apply the same schedule to both worker configuration files if both should follow
the same hours. Keep their distinct names, encoders, and concurrency limits.
Restart each worker after changing its YAML.

## Daily cutoff

To accept new jobs from midnight until 18:00 every day:

```yaml
worker:
  schedule:
    timeZone: Europe/Madrid
    windows:
      - "*@00:00-18:00"
```

At 18:00 the worker stops initiating requests for more jobs. It remains online,
sends heartbeats, and resumes normal requests at midnight if it has capacity.
There is no need for a cron task or container restart at each boundary.

## Several weekday windows and all-day weekends

This example permits requests on weekdays during 00:00–10:00, 14:00–18:00,
and 22:00–24:00, plus all day on Saturday and Sunday:

```yaml
worker:
  schedule:
    timeZone: Europe/Madrid
    windows:
      - "Mon-Fri@00:00-10:00"
      - "Mon-Fri@14:00-18:00"
      - "Mon-Fri@22:00-24:00"
      - "Sat-Sun@00:00-24:00"
```

These are example hours to match against your own electricity contract, not an
automatic tariff selection. The worker does not retrieve prices or classify
public holidays. A weekday public holiday follows its normal weekday rule.
You can change the windows manually; calendar-specific exceptions are not
implemented. Workers on other tariffs or in other timezones should use their
own hours and zone.

The same settings can be supplied on the command line:

```bash
transcoderd-worker --worker.schedule.timeZone=Europe/Madrid \
  --worker.schedule.windows='Mon-Fri@00:00-10:00,Mon-Fri@14:00-18:00,Mon-Fri@22:00-24:00,Sat-Sun@00:00-24:00'
```

A supplied CLI value replaces the corresponding YAML value. Use
`--worker.schedule.windows=` or YAML `windows: []` to remove the weekly
restriction. Nested environment variable aliases are not a verified interface;
use the YAML or CLI settings shown here.

## Window rules

- Syntax: `Days@HH:MM-HH:MM`, with 24-hour clock times and minute precision.
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
