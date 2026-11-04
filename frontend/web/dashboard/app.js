"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const number = new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 });
  const integer = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
  const esc = (value) =>
    String(value ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  const basename = (path) => String(path).split(/[\\/]/).pop() || path;
  const stateNames = {
    active: "Active",
    queued: "Queued",
    completed: "Completed",
    failed: "Failed",
    canceled: "Canceled",
    assigned: "Assigned",
    started: "Started",
  };
  const phases = {
    Job: "Preparation",
    Download: "Download",
    MKVExtract: "Subtitles",
    PGS: "OCR",
    FFMPEG: "Encoding",
    JobVerify: "Verification",
    Upload: "Upload",
  };
  const titles = {
    library: [
      "Library",
      "Your library, visually",
      "Browse, play and choose which videos to queue.",
    ],
    overview: [
      "Overview",
      "Your fleet in motion",
      "Every job, every worker and every byte you save.",
    ],
    workers: [
      "Workers",
      "Your processing power, your control",
      "Adjust CPU and GPU limits independently for each worker, without restarting.",
    ],
    jobs: [
      "Jobs",
      "Every file has a story",
      "Track the phases, progress and outcome of every job.",
    ],
  };
  const resources = window.TranscoderResources;
  const resourceHistory = new resources.History();
  let selectedWorker = null;
  let data = null,
    hours = 168,
    filter = "all",
    fetching = false,
    refreshQueued = false,
    view = "overview",
    timer = null,
    toastTimer;
  const drafts = new Map(),
    saving = new Set();

  function bytes(value) {
    if (!Number.isFinite(value)) return "—";
    const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
    const index =
      value === 0
        ? 0
        : Math.min(
            units.length - 1,
            Math.max(0, Math.floor(Math.log(Math.abs(value)) / Math.log(1024))),
          );
    return `${number.format(value / 1024 ** index)} ${units[index]}`;
  }
  function duration(seconds) {
    if (!(seconds > 0)) return "—";
    if (seconds < 60) return `${Math.ceil(seconds)} s`;
    if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
    return `${number.format(seconds / 3600)} h`;
  }
  function age(time) {
    if (!time) return "no heartbeat";
    const seconds = Math.max(0, (Date.now() - new Date(time)) / 1000);
    if (seconds < 60) return `${Math.floor(seconds)} s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)} min ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)} h ago`;
    return `${Math.floor(seconds / 86400)} d ago`;
  }
  function empty(title, detail = "") {
    return `<div class="empty-state"><span class="empty-icon" aria-hidden="true">◌</span><strong>${esc(title)}</strong><p>${esc(detail)}</p></div>`;
  }
  function badge(state, label) {
    return `<span class="badge ${esc(state)}">${esc(label ?? stateNames[state] ?? state)}</span>`;
  }
  function barWidth(element, percent) {
    element.style.width = `${Math.max(0, Math.min(100, percent || 0))}%`;
  }
  function applyWidths(root) {
    root
      .querySelectorAll("[data-width]")
      .forEach((el) => barWidth(el, Number(el.dataset.width)));
  }
  function showToast(message, failed = false) {
    $("toast").textContent = message;
    $("toast").classList.toggle("error", failed);
    $("toast").hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      $("toast").hidden = true;
    }, 5500);
  }
  async function api(path, options = {}) {
    const response = await fetch(`/dashboard/api/${path}`, {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.timeout(15000),
      ...options,
      headers: {
        "Content-Type": "application/json",
        "X-Dashboard-Request": "1",
        ...options.headers,
      },
    });
    if (response.status === 401) {
      window.location.replace("/dashboard/login");
      throw new Error("Your session has expired.");
    }
    if (!response.ok)
      throw new Error(
        response.status === 400
          ? "Check the values you entered."
          : "The server could not complete the operation.",
      );
    return response.status === 204 ? null : response.json();
  }
  function schedule() {
    clearTimeout(timer);
    if (!document.hidden) timer = setTimeout(refresh, 5000);
  }
  async function refresh() {
    if (fetching) {
      refreshQueued = true;
      return;
    }
    fetching = true;
    $("refresh").disabled = true;
    const requestedHours = hours;
    try {
      const next = await api(`stats?hours=${requestedHours}`);
      if (requestedHours !== hours) return;
      data = next;
      $("error-banner").hidden = true;
      $("loading").hidden = true;
      $("dashboard-content").hidden = false;
      $("connection").innerHTML = '<span class="status-dot"></span>Live';
      $("last-sync").textContent = new Date(
        data.generatedAt,
      ).toLocaleTimeString("en-US", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
      render();
    } catch (e) {
      $("loading").hidden = true;
      $("error-banner").textContent =
        `${e.message} ${data ? "Showing the last received data; it may be outdated." : "Retrying automatically in a few seconds."}`;
      $("error-banner").hidden = false;
      $("connection").innerHTML =
        '<span class="status-dot error"></span>Offline';
    } finally {
      fetching = false;
      $("refresh").disabled = false;
      if (requestedHours !== hours || refreshQueued) {
        refreshQueued = false;
        refresh();
      } else schedule();
    }
  }
  function render() {
    const s = data.summary,
      workers = data.workers;
    resourceHistory.observe(workers, Date.parse(data.generatedAt));
    const online = workers.filter((w) => w.online).length;
    $("nav-worker-count").textContent = workers.length;
    $("nav-job-count").textContent = integer.format(s.active + s.queued);
    $("fleet-note").textContent =
      `${online} of ${workers.length} workers online`;
    $("fleet-detail").textContent = workers.length
      ? `${integer.format(s.active)} active · ${integer.format(s.queued)} queued`
      : "Waiting for the first worker";
    $("fleet-lights").innerHTML = workers
      .slice(0, 24)
      .map((w) => `<i class="${w.online ? "" : "offline"}"></i>`)
      .join("");
    renderMetrics();
    renderThroughput();
    renderImpact();
    renderQueue();
    renderSavings();
    renderCodecs();
    renderWorkerChart();
    renderActivity();
    const currentJobs = data.jobs.filter((j) => j.state === "active");
    $("overview-jobs").innerHTML = jobsTable(
      (currentJobs.length ? currentJobs : data.jobs).slice(0, 6),
    );
    renderJobs();
    // Polling must not replace a form while an operator is typing.
    if (!$("worker-cards").contains(document.activeElement)) renderWorkers();
    renderWorkerDetails();
    applyWidths($("overview-view"));
    applyWidths($("jobs-view"));
  }
  function renderMetrics() {
    const s = data.summary;
    const live = data.workers.filter((w) => w.online && w.runtime);
    const encodes = live.reduce((sum, w) => sum + w.runtime.activeEncodes, 0);
    const target = live.reduce((sum, w) => sum + w.runtime.maxEncodes, 0);
    const split = live.filter((w) => w.runtime.capacityVersion >= 2);
    const cpuActive = split.reduce(
      (sum, w) => sum + w.runtime.activeCPUEncodes,
      0,
    );
    const gpuActive = split.reduce(
      (sum, w) => sum + w.runtime.activeGPUEncodes,
      0,
    );
    const outcomes = s.completedInRange + s.failedInRange;
    const success = outcomes
      ? number.format((100 * s.completedInRange) / outcomes)
      : "—";
    const saved = bytes(s.savedBytes).split(" ");
    const cards = [
      [
        "Space saved",
        "↙",
        `${saved[0]}<small>${saved[1]}</small>`,
        `<span class="positive">${s.sourceBytes ? `${number.format(s.compressionPercent)} % less space` : "Waiting for the first result"}</span> · all time`,
      ],
      [
        "Completed jobs",
        "✓",
        integer.format(s.completedInRange),
        `${integer.format(s.completed)} total · <span class="positive">${integer.format(s.queued)} queued</span>`,
      ],
      [
        "Active encodes",
        "ϟ",
        live.length ? `${encodes}<small>/ ${target} slots</small>` : "—",
        split.length
          ? `<span class="positive">CPU ${cpuActive}</span> · <span class="purple-text">GPU ${gpuActive}</span> · ${integer.format(s.active)} active jobs`
          : `${integer.format(s.active)} active jobs · CPU/GPU breakdown unavailable`,
      ],
      [
        "Success rate",
        "↗",
        `${success}${outcomes ? "<small>%</small>" : ""}`,
        `${integer.format(s.failedInRange)} failed · average ${duration(s.averageSeconds)} / job`,
      ],
    ];
    $("metrics").innerHTML = cards
      .map(
        (c) =>
          `<article class="metric-card"><div class="metric-label"><span>${c[0]}</span><span class="metric-symbol" aria-hidden="true">${c[1]}</span></div><div class="metric-value">${c[2]}</div><p class="metric-caption">${c[3]}</p></article>`,
      )
      .join("");
  }
  const chartDate = (time) =>
    new Date(time).toLocaleString(
      "en-US",
      hours === 24
        ? { hour: "2-digit", minute: "2-digit" }
        : { day: "numeric", month: "short" },
    );
  function renderThroughput() {
    const s = data.summary,
      buckets = data.timeline;
    $("throughput-total").textContent =
      `${integer.format(s.completedInRange)} completed`;
    if (!s.completedInRange && !s.failedInRange) {
      $("throughput-chart").innerHTML = empty(
        "Your next result starts here",
        "Completions and failures will appear in this chart as they arrive.",
      );
      return;
    }
    const width = 660,
      height = 210,
      left = 33,
      right = 10,
      top = 12,
      bottom = 30;
    const max = Math.max(
      1,
      ...buckets.map((b) => Math.max(b.completed, b.failed)),
    );
    const ceiling = max <= 4 ? 4 : Math.ceil(max / 4) * 4;
    const x = (i) =>
      left + ((width - left - right) * i) / Math.max(1, buckets.length - 1);
    const y = (v) => height - bottom - ((height - top - bottom) * v) / ceiling;
    const points = (key) =>
      buckets
        .map(
          (b, i) =>
            `${i ? "L" : "M"}${x(i).toFixed(1)},${y(b[key]).toFixed(1)}`,
        )
        .join(" ");
    const grid = Array.from({ length: 5 }, (_, i) => {
      const value = (ceiling * i) / 4;
      return `<line class="chart-gridline" x1="${left}" y1="${y(value)}" x2="${width - right}" y2="${y(value)}"/><text class="chart-label" x="${left - 11}" y="${y(value) + 3}" text-anchor="end">${integer.format(value)}</text>`;
    }).join("");
    const labelIndices = [
      ...new Set([
        0,
        Math.floor((buckets.length - 1) / 4),
        Math.floor((buckets.length - 1) / 2),
        Math.floor((3 * (buckets.length - 1)) / 4),
        buckets.length - 1,
      ]),
    ];
    $("throughput-chart").innerHTML =
      `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${s.completedInRange} completed jobs and ${s.failedInRange} failed in the selected period"><defs><linearGradient id="throughput-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="#b7ef79" stop-opacity=".16"/><stop offset="100%" stop-color="#b7ef79" stop-opacity="0"/></linearGradient></defs>${grid}<path class="chart-area" d="${points("completed")} L${x(buckets.length - 1)},${y(0)} L${x(0)},${y(0)} Z"/><path class="chart-line" d="${points("completed")}"/><path class="chart-failed" d="${points("failed")}"/>${buckets.map((b, i) => `<circle class="chart-point" cx="${x(i)}" cy="${y(b.completed)}" r="${b.completed > 0 ? 3 : 0}"/><rect class="chart-hover" x="${Math.max(left, x(i) - 8)}" y="${top}" width="16" height="${height - bottom - top}"><title>${esc(chartDate(b.time))}: ${b.completed} completed · ${b.failed} failed</title></rect>`).join("")}${labelIndices.map((i, n) => `<text class="chart-label" x="${x(i)}" y="${height - 7}" text-anchor="${n === 0 ? "start" : n === labelIndices.length - 1 ? "end" : "middle"}">${esc(chartDate(buckets[i].time))}</text>`).join("")}</svg>`;
  }
  function renderImpact() {
    const s = data.summary;
    if (!s.sourceBytes) {
      $("storage-impact").innerHTML = empty(
        "Savings are measured on completion",
        "Verified final sizes are compared with their source files.",
      );
      return;
    }
    $("storage-impact").innerHTML =
      `<div class="impact-number">${number.format(s.compressionPercent)}<span>%</span></div><p class="impact-subtitle">space saved · ${bytes(s.savedBytes)}</p><div class="storage-row"><span>Source</span><div class="storage-bar"><span data-width="100"></span></div><strong>${bytes(s.sourceBytes)}</strong></div><div class="storage-row output"><span>Output</span><div class="storage-bar"><span data-width="${(s.targetBytes / s.sourceBytes) * 100}"></span></div><strong>${bytes(s.targetBytes)}</strong></div>`;
  }
  function renderQueue() {
    const s = data.summary;
    if (!s.total) {
      $("queue-chart").innerHTML = empty(
        "Ready to get started",
        "Add files through the library or TranscoderD API to see your queue.",
      );
      return;
    }
    const segments = [
      ["queued", s.queued, "#7eb8ff", "blue"],
      ["active", s.active, "#ae9bff", "purple"],
      ["completed", s.completed, "#b7ef79", "green"],
      ["failed", s.failed, "#ff8992", "red"],
      ["canceled", s.canceled, "#4d5c72", "gray"],
    ];
    let offset = 0;
    const circle = 2 * Math.PI * 49;
    const rings = segments
      .map(([key, value, color]) => {
        const length = (value / s.total) * circle;
        const svg = `<circle class="ring-segment" cx="63" cy="63" r="49" stroke="${color}" stroke-dasharray="${length} ${circle - length}" stroke-dashoffset="${-offset}"><title>${stateNames[key]}: ${value}</title></circle>`;
        offset += length;
        return svg;
      })
      .join("");
    $("queue-chart").innerHTML =
      `<div class="queue-layout"><div class="queue-ring"><svg viewBox="0 0 126 126" role="img" aria-label="Status of ${s.total} jobs"><circle class="ring-base" cx="63" cy="63" r="49"/>${rings}</svg><div class="ring-total"><strong>${integer.format(s.total)}</strong><small>JOBS</small></div></div><div class="queue-legend">${segments.map(([key, value, , color]) => `<div><i class="legend-dot ${color}"></i>${stateNames[key]}<strong>${integer.format(value)}</strong></div>`).join("")}</div></div>`;
  }
  function renderSavings() {
    const buckets = data.timeline;
    const total = buckets.reduce(
      (n, b) => n + b.sourceBytes - b.targetBytes,
      0,
    );
    if (!buckets.some((b) => b.sourceBytes)) {
      $("savings-chart").innerHTML = empty(
        "See your savings over time",
        "See how much space you save in each interval.",
      );
      return;
    }
    const width = 360,
      height = 160,
      left = 35,
      bottom = 28,
      top = 12;
    const values = buckets.map((b) => b.sourceBytes - b.targetBytes);
    const maximum = Math.max(1, ...values),
      minimum = Math.min(0, ...values),
      range = maximum - minimum;
    const x = (i) => left + (i * (width - left - 8)) / buckets.length;
    const y = (value) =>
      top + ((maximum - value) / range) * (height - bottom - top);
    const barSize = Math.max(3, ((width - left - 8) / buckets.length) * 0.55);
    $("savings-chart").innerHTML =
      `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${esc(bytes(total))} saved in the selected period">${[
        0, 0.5, 1,
      ]
        .map((f) => {
          const value = minimum + range * f;
          return `<line class="chart-gridline" x1="${left}" y1="${y(value)}" x2="${width - 8}" y2="${y(value)}"/><text class="chart-label" x="${left - 6}" y="${y(value) + 3}" text-anchor="end">${esc(bytes(value))}</text>`;
        })
        .join(
          "",
        )}${buckets.map((b, i) => `<rect x="${x(i) + 3}" y="${Math.min(y(values[i]), y(0))}" width="${barSize}" height="${Math.max(0, Math.abs(y(values[i]) - y(0)))}" rx="2" fill="${values[i] < 0 ? "#ff8992" : "#ae9bff"}"><title>${esc(chartDate(b.time))}: ${esc(bytes(values[i]))}</title></rect>`).join("")}<text class="chart-label" x="${left}" y="${height - 5}">${esc(chartDate(buckets[0].time))}</text><text class="chart-label" x="${width - 8}" y="${height - 5}" text-anchor="end">${esc(chartDate(buckets.at(-1).time))}</text></svg><div class="graph-footer"><span>Saved in this period</span><strong>${bytes(total)}</strong></div>`;
  }
  function renderCodecs() {
    if (!data.codecs.length) {
      $("codec-chart").innerHTML = empty(
        "No codecs to compare yet",
        "Based on finalized file sizes, grouped by output codec.",
      );
      return;
    }
    $("codec-chart").innerHTML = `<div class="horizontal-chart">${data.codecs
      .slice(0, 5)
      .map((c) => {
        const saved = 100 * (1 - c.targetBytes / c.sourceBytes);
        return `<div><div class="bar-label"><strong>${esc(c.name === "unknown" ? "Output metadata unavailable" : c.name.toUpperCase())}</strong><span>${number.format(saved)} % smaller</span></div><div class="bar-track codec"><span data-width="${Math.max(0, saved)}"></span></div><div class="bar-detail"><span>${integer.format(c.jobs)} files</span><span>${bytes(c.sourceBytes)} → ${bytes(c.targetBytes)}</span></div></div>`;
      })
      .join("")}</div>`;
  }
  function renderWorkerChart() {
    const workers = [...data.workers]
      .sort((a, b) => b.completed - a.completed || a.name.localeCompare(b.name))
      .slice(0, 6);
    if (!workers.length) {
      $("worker-chart").innerHTML = empty(
        "Your fleet is on its way",
        "Workers appear after their first heartbeat.",
      );
      return;
    }
    const max = Math.max(1, ...workers.map((w) => w.completed + w.failed));
    $("worker-chart").innerHTML =
      `<div class="horizontal-chart">${workers.map((w) => `<a class="worker-summary-link" href="${esc(resources.link(w.name))}"><div class="bar-label"><strong><span class="status-dot ${w.online ? "" : "offline"}"></span> ${esc(w.name)}</strong><span>${integer.format(w.completed)} ✓ · ${integer.format(w.failed)} failures</span></div>${resources.summary(w, Date.parse(data.generatedAt))}<div class="bar-track"><span data-width="${(w.completed / max) * 100}"></span><span class="failed" data-width="${(w.failed / max) * 100}"></span></div><span class="worker-details-link">Resource details ↗</span></a>`).join("")}</div><p class="worker-chart-footer">Job outcomes in the selected period · measured host/device usage. Multi-GPU summaries show the highest reported value for each metric.</p>`;
  }
  function renderActivity() {
    if (!data.activity.length) {
      $("activity-feed").innerHTML = empty(
        "No events in this period",
        "Assignments, completions and errors will appear here.",
      );
      return;
    }
    $("activity-feed").innerHTML = data.activity
      .slice(0, 5)
      .map(
        (a) =>
          `<article class="activity-item"><span class="activity-icon ${esc(a.status)}" aria-hidden="true">${a.status === "completed" ? "✓" : a.status === "failed" ? "!" : "↗"}</span><div class="activity-content"><strong title="${esc(a.sourcePath)}">${esc(basename(a.sourcePath))}</strong><p>${esc(stateNames[a.status] ?? a.status)} · ${esc(a.worker || "Server")}${a.phase !== "Job" ? " · " + esc(phases[a.phase] ?? a.phase) : ""}</p>${a.message ? `<p class="activity-error" title="${esc(a.message)}">${esc(a.message)}</p>` : ""}</div><time datetime="${esc(a.time)}" title="${esc(new Date(a.time).toLocaleString("en-US"))}">${age(a.time)}</time></article>`,
      )
      .join("");
  }
  function jobProgress(j) {
    if (j.state === "completed") {
      const savings =
        j.sourceBytes > 0 && j.targetBytes > 0
          ? `${number.format((1 - j.targetBytes / j.sourceBytes) * 100)} % smaller · ${bytes(j.targetBytes)}`
          : "Final size unavailable";
      return `<span class="accent">✓ Finished</span><div class="progress-caption">${esc(savings)}</div>`;
    }
    if (j.state === "failed")
      return `<p class="job-message" title="${esc(j.message)}">${esc(j.message || "The worker reported an error.")}</p>`;
    if (j.state === "queued")
      return '<span class="muted">Waiting for assignment</span>';
    if (j.state === "canceled") return '<span class="muted">Canceled</span>';
    if (!j.progress.length)
      return `<span>${esc((phases[j.phase] ?? j.phase) + (j.message ? " · " + j.message : ""))}</span><div class="progress-caption">Waiting for progress</div>`;
    const percent =
      j.progress.reduce((n, p) => n + p.percent, 0) / j.progress.length;
    const eta = Math.max(...j.progress.map((p) => p.etaSeconds));
    const stale = j.progress.some((p) => p.stale);
    return `<div class="load-labels"><span>${esc((phases[j.phase] ?? j.phase) + (j.message ? " · " + j.message : ""))}</span><strong>${number.format(percent)} %</strong></div><div class="progress-track"><span data-width="${percent}"></span></div><div class="progress-caption ${stale ? "stale" : ""}"><span>${stale ? "Outdated progress" : "Phase ETA: " + duration(eta)}</span><span>${j.progress.length > 1 ? `${j.progress.length} subtasks` : ""}</span></div>`;
  }
  function observedJob(job) {
    if (job.state !== "active") return null;
    const worker = data.workers.find((w) => w.name === job.worker);
    const local = worker?.runtime?.jobs?.find((j) => j.id === job.id);
    if (!local) return null;
    return {
      ...local,
      queueHeld:
        local.state === "queued" &&
        local.phase === "FFMPEG" &&
        worker.runtime.paused,
    };
  }
  function jobBadge(job) {
    const local = observedJob(job);
    return local?.state === "paused"
      ? badge(
          "queued",
          local.individuallyPaused ? "Individually paused" : "RAM pause",
        )
      : local?.queueHeld
        ? badge("queued", "Queue stopped")
        : badge(job.state);
  }
  function observedProgress(job) {
    const local = observedJob(job);
    if (local?.state === "paused")
      return `<span class="muted">${job.progress?.length ? number.format(job.progress[0].percent) + "% · " : ""}Paused in RAM · progress retained</span>`;
    if (local?.queueHeld)
      return '<span class="muted">Waiting for the queue to resume</span>';
    return jobProgress(job);
  }
  function jobsTable(jobs) {
    if (!jobs.length)
      return empty(
        "No jobs to display",
        "Files appear here when they are queued or you change the filter.",
      );
    return `<table><thead><tr><th>File</th><th>Status</th><th>Worker</th><th>Progress / result</th><th>Updated</th><th>Video</th></tr></thead><tbody>${jobs.map((j) => `<tr><td class="file-cell"><div class="file-name"><span aria-hidden="true">▹</span><div><strong title="${esc(j.sourcePath)}">${esc(basename(j.sourcePath))}</strong><small title="${esc(j.id)}">${bytes(j.sourceBytes)} · ${esc(j.id.slice(0, 8))}</small></div></div></td><td>${jobBadge(j)}</td><td>${esc(j.worker || "—")}</td><td><div class="job-progress">${observedProgress(j)}</div></td><td title="${esc(new Date(j.updatedAt).toLocaleString("en-US"))}">${age(j.updatedAt)}</td><td><button type="button" class="button media-job-play" data-job-media="${esc(j.id)}" aria-label="Play ${esc(basename(j.sourcePath))}">▷ View</button></td></tr>`).join("")}</tbody></table>`;
  }
  function renderJobs() {
    if (!data) return;
    const search = $("job-search").value.toLocaleLowerCase("en");
    const jobs = data.jobs.filter(
      (j) =>
        (filter === "all" || j.state === filter) &&
        (!search ||
          `${j.sourcePath} ${j.worker} ${j.id}`
            .toLocaleLowerCase("en")
            .includes(search)),
    );
    $("job-list").innerHTML = jobsTable(jobs);
    applyWidths($("job-list"));
  }
  function workerLimits(w) {
    if (w.desiredCPUJobs != null && w.desiredGPUJobs != null)
      return { cpu: String(w.desiredCPUJobs), gpu: String(w.desiredGPUJobs) };
    if (w.desiredMax != null && w.runtime?.encoder) {
      if (w.runtime.encoder === "av1_nvenc")
        return { cpu: "0", gpu: String(w.desiredMax) };
      if (["libsvtav1", "libx265"].includes(w.runtime.encoder))
        return { cpu: String(w.desiredMax), gpu: "0" };
    }
    if (w.runtime?.capacityVersion >= 2)
      return { cpu: String(w.runtime.cpuJobs), gpu: String(w.runtime.gpuJobs) };
    if (w.runtime?.kind === "cpu")
      return { cpu: String(w.runtime.maxActiveJobs), gpu: "0" };
    if (w.runtime?.kind === "gpu")
      return { cpu: "0", gpu: String(w.runtime.maxActiveJobs) };
    return { cpu: "", gpu: "" };
  }
  function renderWorkers() {
    if (!data) return;
    if (!data.workers.length) {
      $("worker-cards").innerHTML = empty(
        "Connect your first worker",
        "Its name, encoders and status will appear automatically.",
      );
      return;
    }
    $("worker-cards").innerHTML = data.workers
      .filter(w => !selectedWorker || w.name === selectedWorker)
      .map((w, i) => {
        const rt = w.runtime;
        const modern = rt?.capacityVersion >= 2;
        const desired = workerLimits(w);
        const draft = drafts.get(w.name) ?? desired;
        const paired = w.desiredCPUJobs != null && w.desiredGPUJobs != null;
        const pending =
          (paired || w.desiredMax != null) &&
          (!modern ||
            Number(desired.cpu) !== rt.cpuJobs ||
            Number(desired.gpu) !== rt.gpuJobs);
        const paused =
          modern && (rt.paused || (rt.cpuJobs === 0 && rt.gpuJobs === 0));
        const status = !w.online
          ? ["offline", "Offline"]
          : !modern
            ? ["queued", "Update worker"]
            : rt.draining
              ? ["queued", "Maintenance"]
              : rt.finishing
                ? ["queued", "Finishing jobs"]
                : rt.ramPaused
                  ? ["queued", "RAM pause"]
                  : paused
                    ? ["queued", "Queue paused"]
                    : !rt.withinSchedule
                      ? ["queued", "Outside schedule"]
                      : ["completed", "Online"];
        const busy = saving.has(w.name);
        const fixedCPU = rt?.managedProfile && rt.encoder !== "av1_nvenc",
          fixedGPU = rt?.managedProfile && rt.encoder === "av1_nvenc";
        const lane = (kind, encoder, active, limit) =>
          `<div class="encoder-lane ${kind}"><div class="load-labels"><span><b>${kind.toUpperCase()}</b> · ${esc(encoder || "No telemetry")}</span><strong>${modern ? `${active} / ${limit}` : "—"}</strong></div><div class="progress-track"><span data-width="${modern && limit > 0 ? (active / limit) * 100 : 0}"></span></div><p class="lane-note">${!modern ? "Update this worker to monitor and control both encoders." : active > limit ? `${active} occupied slots · limit ${limit}` : limit === 0 ? "Disabled for new encodes" : `${limit} encoding slots${w.online ? "" : " · last reported"}`}</p></div>`;
        const note = pending
          ? `◷ Desired: CPU ${desired.cpu || "?"} · GPU ${desired.gpu || "?"} — ${!modern ? "worker update required" : w.online ? "pending application" : "will apply on reconnection"}`
          : !modern
            ? "Both limits are saved together; this older worker is paused until updated."
            : rt.draining
              ? "Maintenance: finishing accepted work without admitting new jobs."
              : rt.finishing
                ? "Finishing accepted work; synchronization preserves this hold."
                : rt.ramPaused
                  ? `RAM pause · ${rt.suspendedEncodes || 0} encodes suspended. Configured limits are retained.`
                  : paused
                    ? "Queue stopped. Active encodes finish; queued encodes wait for Resume."
                    : `✓ Limits applied · up to ${rt.cpuJobs + rt.gpuJobs + 1} jobs in the pipeline`;
        return `<article class="panel worker-card" data-worker="${esc(w.name)}">
        <div class="worker-heading"><span class="worker-icon" aria-hidden="true">▦</span><div><h2>${esc(w.name)}</h2><p>${modern ? (rt.managedProfile ? "Windows profile · " + (fixedCPU ? "CPU" : "GPU") : "CPU + GPU · independent execution") : esc(rt?.encoder || "Waiting for compatible telemetry")}</p></div>${badge(...status)}</div>
        <div class="worker-stats"><div class="worker-stat"><small>CPU encodes${w.online ? "" : " · last reported"}</small><strong>${modern ? rt.activeCPUEncodes : "—"}<span>${modern ? "/ " + rt.cpuJobs : ""}</span></strong></div><div class="worker-stat"><small>GPU encodes${w.online ? "" : " · last reported"}</small><strong>${modern ? rt.activeGPUEncodes : "—"}<span>${modern ? "/ " + rt.gpuJobs : ""}</span></strong></div><div class="worker-stat"><small>Accepted jobs</small><strong>${rt ? rt.activeJobs : w.activeJobs}</strong></div></div>
        ${resources.summary(w, Date.parse(data.generatedAt))}<a class="text-link worker-resource-link" href="${esc(resources.link(w.name))}">View resource details ↗</a>
        <div class="encoder-lanes">${lane("cpu", rt?.cpuEncoder, rt?.activeCPUEncodes, rt?.cpuJobs)}${lane("gpu", rt?.gpuEncoder, rt?.activeGPUEncodes, rt?.gpuJobs)}</div>
        <form class="worker-capacity" data-worker="${esc(w.name)}"><div class="encoder-inputs"><div><label for="cpu-${i}">CPU limit <span>ENCODES</span></label><input id="cpu-${i}" name="cpuJobs" type="number" min="0" step="1" required inputmode="numeric" value="${fixedGPU ? 0 : esc(draft.cpu)}" ${rt?.managedProfile ? 'max="64"' : ""} ${busy || fixedGPU ? "disabled" : ""} aria-label="CPU limit for ${esc(w.name)}"></div><div><label for="gpu-${i}">GPU limit <span>ENCODES</span></label><input id="gpu-${i}" name="gpuJobs" type="number" min="0" step="1" required inputmode="numeric" value="${fixedCPU ? 0 : esc(draft.gpu)}" ${rt?.managedProfile ? 'max="64"' : ""} ${busy || fixedCPU ? "disabled" : ""} aria-label="GPU limit for ${esc(w.name)}"></div></div><div class="capacity-controls"><button class="button primary" type="submit" ${busy ? "disabled" : ""}>${busy ? "Saving…" : "Save limits"}</button><button class="button subtle" type="button" data-pause="${esc(w.name)}" ${busy ? "disabled" : ""}>Pause both</button></div><p class="pending-note ${pending ? "pending" : ""}">${esc(note)}</p></form>
        <div class="worker-footer"><span>${esc(w.ip || "No heartbeat recorded")} · ${age(w.lastSeen)}</span><span>${integer.format(w.completed)} completed · ${integer.format(w.failed)} failures · ${bytes(w.sourceBytes - w.targetBytes)}</span></div>
      </article>`;
      })
      .join("");
    applyWidths($("worker-cards"));
  }
  function renderWorkerDetails() {
    const show = view === "workers" && selectedWorker !== null;
    $("worker-detail-navigation").hidden = !show;
    $("worker-resource-detail").hidden = !show;
    if (!show || !data) return;
    const worker = data.workers.find(w => w.name === selectedWorker);
    $("worker-resource-detail").innerHTML = worker
      ? resources.details(worker, resourceHistory, Date.parse(data.generatedAt))
      : empty("Worker not found", "Return to all workers to choose an available worker.");
    applyWidths($("worker-resource-detail"));
  }
  async function saveWorker(name, values) {
    const w = data.workers.find((worker) => worker.name === name);
    if (!w || saving.has(name)) return;
    const cpu = Number(values.cpu),
      gpu = Number(values.gpu);
    if (
      values.cpu === "" ||
      values.gpu === "" ||
      !Number.isSafeInteger(cpu) ||
      !Number.isSafeInteger(gpu) ||
      cpu < 0 ||
      gpu < 0 ||
      !Number.isSafeInteger(cpu + gpu)
    ) {
      showToast("Enter two whole numbers greater than or equal to 0.", true);
      return;
    }
    saving.add(name);
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    renderWorkers();
    try {
      await api(`workers/${encodeURIComponent(name)}/settings`, {
        method: "PUT",
        body: JSON.stringify({ cpuJobs: cpu, gpuJobs: gpu }),
      });
      const current = data.workers.find((worker) => worker.name === name);
      if (current) {
        current.desiredCPUJobs = cpu;
        current.desiredGPUJobs = gpu;
        current.desiredMax = cpu + gpu;
      }
      drafts.delete(name);
      showToast(
        `${name}: CPU ${cpu} · GPU ${gpu} saved. They will apply at the next synchronization.`,
      );
      await refresh();
    } catch (e) {
      showToast(e.message, true);
    } finally {
      saving.delete(name);
      renderWorkers();
    }
  }
  function setView() {
    window.scrollTo(0, 0);
    const [route, query = ""] = window.location.hash.slice(1).split("?", 2);
    view = route;
    selectedWorker = view === "workers" ? new URLSearchParams(query).get("worker") : null;
    if (!titles[view]) view = "overview";
    const [label, title, description] = titles[view];
    $("breadcrumb-view").textContent = label;
    $("page-title").innerHTML = `${esc(title)}<span class="accent">.</span>`;
    $("page-description").textContent = description;
    document.title = `${label} · TranscoderD`;
    for (const key of Object.keys(titles))
      $(`${key}-view`).hidden = key !== view;
    document.querySelectorAll("[data-view]").forEach((a) => {
      const active = a.dataset.view === view;
      a.classList.toggle("active", active);
      if (active) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    });
    if (view === "workers") renderWorkers();
    renderWorkerDetails();
  }
  $("period").addEventListener("change", () => {
    hours = Number($("period").value);
    refresh();
  });
  $("refresh").addEventListener("click", refresh);
  document.querySelectorAll("[data-logout]").forEach((button) =>
    button.addEventListener("click", async () => {
      try {
        await api("logout", { method: "POST" });
        window.location.replace("/dashboard/login");
      } catch (e) {
        showToast(e.message, true);
      }
    }),
  );
  $("job-search").addEventListener("input", renderJobs);
  document.querySelectorAll("[data-filter]").forEach((button) =>
    button.addEventListener("click", () => {
      filter = button.dataset.filter;
      document.querySelectorAll("[data-filter]").forEach((b) => {
        b.classList.toggle("active", b === button);
        b.setAttribute("aria-pressed", String(b === button));
      });
      renderJobs();
    }),
  );
  $("worker-cards").addEventListener("input", (event) => {
    if (event.target.name === "cpuJobs" || event.target.name === "gpuJobs") {
      const form = event.target.closest("form");
      drafts.set(form.dataset.worker, {
        cpu: form.elements.cpuJobs.value,
        gpu: form.elements.gpuJobs.value,
      });
    }
  });
  $("worker-cards").addEventListener("submit", (event) => {
    event.preventDefault();
    saveWorker(event.target.dataset.worker, {
      cpu: event.target.elements.cpuJobs.value,
      gpu: event.target.elements.gpuJobs.value,
    });
  });
  $("worker-cards").addEventListener("click", (event) => {
    const button = event.target.closest("[data-pause]");
    if (button) saveWorker(button.dataset.pause, { cpu: "0", gpu: "0" });
  });
  window.addEventListener("hashchange", setView);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) clearTimeout(timer);
    else refresh();
  });
  setView();
  refresh();
})();
