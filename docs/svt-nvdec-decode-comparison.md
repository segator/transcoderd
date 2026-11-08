# SVT-AV1 with software versus NVIDIA decoding

On 2026-11-08, the Ryzen 9 9900X3D and RTX 5080 ran FFmpeg 9.0.2 with SVT-AV1 4.3.0, preset 4, CRF26 and 10-bit output. Each test encoded 900 frames at the original resolution from the same compressed source. Three timed repetitions alternated order. The hardware-decoding runs retained CUDA-decoded video in system memory for SVT; logs identified NVDEC. Source CPU and NVDEC frame hashes matched for all 900 frames of each excerpt. Isolated timings had no NVENC activity in sampled telemetry.

| Source | Workload | Software decode | NVDEC | NVDEC time change |
| --- | --- | ---: | ---: | ---: |
| H.264, 8-bit | SVT alone | 7.364 s | 7.613 s | +3.4% |
| HEVC, 10-bit | SVT alone | 8.421 s | 8.446 s | +0.3% |
| H.264, 8-bit | SVT + one NVENC job | 7.706 s | 7.904 s | +2.6% |
| HEVC, 10-bit | SVT + one NVENC job | 8.556 s | 8.747 s | +2.2% |

Concurrent rows are elapsed time to finish an identical batch of one 900-frame SVT job and one 900-frame NVENC job. The GPU job uses NVDEC and the production AV1 NVENC p7/UHQ CQ38 profile. Every process completed and reported the requested frame count. Individual runs and separate CPU/GPU completion times are retained in [the anonymized CSV](benchmarks/svt-nvdec-decode.csv); private command vectors, raw samples and logs remain in the local lab directory.

These two short excerpts show no worthwhile SVT improvement from NVIDIA decode; differences are small relative to run-to-run variation. Transferring decoded frames from GPU memory to the CPU can offset decoder savings. The evidence is not a whole-library result, a Windows-native timing, or a test of every codec, resolution and GPU load. It does not justify enabling NVDEC for CPU encoders by default.
