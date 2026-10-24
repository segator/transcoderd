# Complete-film AV1 comparison method

This historical benchmark compares the then-approved GPU CQ34 profile with
SVT CPU presets 6, 4 and 3 on the same complete source films. The current
defaults were later adjusted to GPU CQ38 and CPU preset 4 / CRF25 based on
representative multiclip comparisons; see [the current encoding profiles](encoding-profiles.md).
Source files remain unchanged. The measured report is generated only after
jobs and every-frame scoring finish.

## Workload and settings

- One accepted job and one encode at a time for the primary timing runs.
- GPU: the production p7/UHQ, CQ34, QP0–100, fullres multipass, four middle
  B references, disabled split, P010 and CPU width-filter profile.
- CPU: SVT-AV1 presets 6, 4 and 3, initially CRF23, 10-bit output and the same
  width filter. SVT manages its own thread pool; no CPU quota is imposed.
- The real application selects audio/subtitles, converts image subtitles,
  encodes audio with `libfdk_aac -vbr 5`, verifies size/duration, uploads the
  file and publishes completion. The E2E test then probes and fully decodes
  the output. Its input is a disposable hard link to the local source.
- Record actual complete MKV bytes and summed video packet bytes separately.
  Source audio conversion and stream selection can contribute to total savings.
- Compare fixed profiles first. A size difference at different VMAF is not a
  same-quality codec-efficiency result. Add completed quality-matched runs when
  needed before choosing a compression winner. No title-dependent calibration
  is added to normal worker operation.

## Time and resources

`FFMPEG` start/completion events define the application encode phase, including
its normal probing/hash bookkeeping. `Job` start/completion defines the whole
worker job. Transfer, extraction, OCR and upload have their own event intervals.
The E2E wall clock also includes container setup, polling, copying the retained
output and strict playback verification. Offline VMAF time is reported separately
and is not part of production encoding cost.

The runner samples the worker container's cumulative CPU counters and memory
approximately once per second, keeping raw samples. Encode-phase CPU time is
calculated from counter differences at its event boundaries, linearly
interpolating the adjacent samples. CPU percent uses 100% per logical CPU;
1,800% on a 24-logical-CPU host means about 18 logical CPUs of scheduled work,
or 75% of capacity. Report the encode-phase average, observed sample peak,
number of samples, and largest sample gap. These are container measurements,
including the worker and its FFmpeg child, not whole-host power measurements.

Memory reports peak container working set (usage minus inactive file cache)
and anonymous memory separately. They are not an instantaneous encoder RSS
trace. More accepted jobs need additional memory; configured job limits do not
prove that a particular combination fits the host. GPU utilization and wall-plug
energy are not inferred from CPU usage. Background work and any overlapping
runs must be disclosed when interpreting timings.

## Complete-video quality

Decode and count all source/output video frames; require equal counts. Score
all frames with Netflix's standalone `vmaf` CLI and model `vmaf_v0.6.1`.
FFmpeg decodes each input to an 8-bit `yuv420p` raw-video pipe; VMAF pairs the
streams by frame order, so container and audio timestamps cannot shift the
comparison. A 240-frame sample matched FFmpeg's libvmaf filter exactly when
both methods used the same 8-bit format. Require the VMAF log to contain exactly
the decoded frame count with contiguous frame numbers and finite scores. The
standalone CLI is an offline lab dependency; the worker image and production
encoding do not run it.

Report whole-film mean, p5, p1 and minimum; also compute the worst **rolling**
60-second p5, advancing the window by one frame. A separate sequence of fixed
one-minute p5 blocks supports a readable timeline chart. These are different
statistics: a strong whole-film p5 can hide a difficult short scene, and fixed
blocks can hide a dip crossing a block boundary. VMAF remains an objective
comparison with the existing source, not proof of invisible loss or restoration
of a previously compressed original. These test sources are SDR; no HDR
recommendation follows from them.

## Retained artifacts and resume

`scripts/av1_full_films.py` accepts a JSON array with `title`, `slug` and absolute
`source` fields. `approved_gpu` can identify the previous watched film for a
separate payload comparison; a fresh GPU E2E run supplies current telemetry.
Each run retains its launch settings/image IDs, completed movie, event history,
worker log, container samples, decoded probes, exact FFmpeg argv when the local
Docker process is accessible, VMAF command/log/frame scores, and quality summary.

```bash
python3 scripts/av1_full_films.py \
  --manifest /lab/films.json --output /lab/full-film-results \
  --go /path/to/go --ffmpeg /path/to/project/ffmpeg \
  --vmaf-cli /path/to/vmaf --phase all

python3 scripts/av1_full_film_report.py \
  --manifest /lab/films.json --root /lab/full-film-results
```

`--phase encode` and `--phase quality` separate timing from offline scoring.
`--profiles cpu-p6,cpu-p4,cpu-p3,gpu` is the default order. `--cpu-crf` and a
fresh output root support an additional quality-matched trial without replacing
baseline outputs. Completed matching jobs are reused. Changed source/settings
or missing completed artifacts fail instead of silently reusing a stale result.
Review incomplete jobs before using `--retry-incomplete`, which archives them
before restarting that run. An interrupted video encode restarts; completed
jobs remain reusable. Source identity uses size/mtime and copy hashes where
available. Keep the selected Docker tags unchanged during a timing batch.

The E2E harness supports `E2E_CPU_PRESET`, `E2E_CPU_CRF`, `E2E_JOB_TIMEOUT`,
`E2E_REPORT_DIR`, and the existing movie/output/image variables. Complete CPU
films default to CRF23; the tiny functional CI fixture keeps CRF35. Resource
sampling requires a local Docker daemon at `/var/run/docker.sock`; raw samples
stay in the ignored local lab directory. A production deployment does not run
these benchmark collectors or VMAF passes.
