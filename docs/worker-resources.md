# Worker resource monitoring

The independent fleet frontend shows CPU, GPU and NVENC usage beside each overview worker. Open a row or **View resource details** to select that worker in **Workers**. **All workers** restores the complete list. Capacity and pause controls remain available; slot occupancy is separate from measured utilization.

## What is measured

- **CPU:** busy counter deltas, normalized to 0–100% across the reported logical processors. The first reading needs a second sample. Linux reads `/proc/stat`, excluding idle/I/O wait and avoiding double-counted guest time. Windows reads `GetSystemTimes`; on systems with multiple processor groups the pinned sampler reports its group explicitly, including that group's processor count.
- **Host RAM:** total minus available physical memory. Linux uses `MemAvailable`; Windows uses `GlobalMemoryStatusEx`. These are OS-visible host readings, not worker-process RSS or container memory quotas. A VM/container can expose a different scope from the physical machine.
- **GPU utilization:** the NVIDIA device's reported utilization, not configured GPU slots. Multiple GPUs have separate identity and detail cards; compact summaries use the highest available value for each metric.
- **Video encode / NVENC and video decode:** device-level engine utilization. NVENC load can be high while general GPU utilization is low.
- **VRAM:** reported used and total framebuffer bytes, separate from memory-controller activity.
- **Temperature and board power:** reported Celsius and watts, with the reported power limit. Unsupported sensors remain unavailable.

Readings include other processes and other worker profiles sharing the host/device. Do not add duplicated host measurements across CPU/GPU profiles or interpret them as per-job utilization. GPU UUID/PCI identity and driver version help distinguish devices.

## Collection and availability

Workers collect read-only snapshots in the background every five seconds. CPU/RAM use native OS counters; NVIDIA metrics use `nvidia-smi -q -x`, with a two-second command cancellation deadline, bounded pipe cleanup and a two-MiB output limit. Only selected metrics are published; process lists and raw command output are not included. A failed query clears current GPU readings while CPU monitoring, heartbeats and encoding continue. Stopping the worker cancels collection.

Install the NVIDIA driver/utility on the worker and make `nvidia-smi` available on PATH. GPU containers need the existing `NVIDIA_DRIVER_CAPABILITIES=compute,utility,video` and GPU device exposure described in [worker deployment](worker-deployment.md). CPU-only hosts need no NVIDIA tools. The API and frontend hosts do not run these queries for remote workers, and the frontend still needs only API access.

Upgrade the API/frontend, then the workers to receive the optional `runtime.resources` heartbeat field. Existing workers remain compatible and show **No hardware telemetry**. No database migration is needed: current resources are stored in the existing runtime JSON.

The UI uses the source sample time independently of worker presence. Offline workers and samples older than twenty seconds are last reported, not live; missing readings display an em dash, whereas genuine idle zero remains 0%. An ahead-of-server sample clock is identified explicitly. Detail plots retain at most 120 distinct samples/ten minutes in page memory, preserve gaps and reset on reload. Resource history is never stored in browser persistent storage or the PWA cache.

## References

Sensor definitions and platform scope follow [NVIDIA System Management Interface](https://docs.nvidia.com/deploy/nvidia-smi/) and Microsoft's [GetSystemTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getsystemtimes) and [GlobalMemoryStatusEx](https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-globalmemorystatusex) documentation.
