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
    overview: "Overview",
    workers: "Workers",
    jobs: "Jobs",
    library: "Library",
  };
  const resources = window.TranscoderResources;
  const resourceHistory = new resources.History();
  let selectedWorker = null,
    workerPage = 0,
    jobsOffset = 0,
    jobsGeneration = 0,
    jobSearchTimer;
  let data = null,
    hours = 168,
    customRange = null,
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
    return `<div class="empty-state"><strong>${esc(title)}</strong>${detail ? `<p>${esc(detail)}</p>` : ""}</div>`;
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
    if (!response.ok) {
      const reason =
        response.status === 409
          ? (await response.text()).trim().slice(0, 512)
          : "";
      throw new Error(
        reason ||
          (response.status === 400
            ? "Check the values you entered."
            : "The server could not complete the operation."),
      );
    }
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
    const requestedPeriod = periodQuery();
    try {
      const next = await api(`stats?${requestedPeriod}`);
      if (requestedPeriod !== periodQuery()) return;
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
      if (requestedPeriod !== periodQuery() || refreshQueued) {
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
    $("nav-worker-count").textContent = visibleWorkers().length;
    $("nav-job-count").textContent = integer.format(s.active + s.queued);
    $("fleet-note").textContent =
      `${online} of ${workers.length} recent workers online`;
    $("fleet-detail").textContent = workers.length
      ? `${integer.format(s.active)} active · ${integer.format(s.queued)} queued`
      : "Waiting for the first worker";
    $("fleet-lights").innerHTML = workers
      .slice(0, 24)
      .map((w) => `<i class="${w.online ? "" : "offline"}"></i>`)
      .join("");
    renderMetrics();
    $("period-scope").textContent = rangeLabel();
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
    const editingWorker =
      document.activeElement?.closest?.("form[data-worker]")?.dataset.worker;
    if (
      !editingWorker ||
      !data.workers.some((w) => w.name === editingWorker && w.online)
    )
      renderWorkers();
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
    const p = periodSummary();
    const saved = bytes(p.savedBytes).split(" ");
    const cards = [
      [
        "Space saved",
        "↙",
        `${saved[0]}<small>${saved[1]}</small>`,
        p.sourceBytes
          ? `${number.format(p.compressionPercent)}% reduction`
          : "No measured sizes",
      ],
      [
        "Completed",
        "✓",
        integer.format(s.completedInRange),
        `${integer.format(s.failedInRange)} failed`,
      ],
      [
        "Success rate",
        "↗",
        `${success}${outcomes ? "<small>%</small>" : ""}`,
        "Excludes canceled jobs",
      ],
      ["Average time", "◷", duration(s.averageSeconds), "Per completed job"],
    ];
    const global = [
      [
        "Space saved · All time",
        "↙",
        bytes(s.savedBytes),
        `${number.format(s.compressionPercent)}% reduction`,
      ],
      [
        "Completed · All time",
        "✓",
        integer.format(s.completed),
        `${integer.format(s.failed)} failed`,
      ],
      [
        "Active encodes · Live",
        "ϟ",
        live.length ? `${encodes}<small>/ ${target}</small>` : "—",
        split.length
          ? `CPU ${cpuActive} · GPU ${gpuActive}`
          : `${integer.format(s.active)} active jobs`,
      ],
      [
        "Queue · Live",
        "▥",
        integer.format(s.queued),
        `${integer.format(s.active)} active jobs`,
      ],
    ];
    const card = (c) =>
      `<article class="metric-card"><div class="metric-label"><span>${c[0]}</span><span class="metric-symbol" aria-hidden="true">${c[1]}</span></div><div class="metric-value">${c[2]}</div><p class="metric-caption">${c[3]}</p></article>`;
    $("metrics").innerHTML = cards.map(card).join("");
    $("global-metrics").innerHTML = global.map(card).join("");
  }
  function periodSummary() {
    // Range sizes can also be calculated from the same filtered timeline when
    // a frontend is deployed before its API upgrade.
    const p =
      data.period ||
      data.timeline.reduce(
        (p, b) => ({
          sourceBytes: p.sourceBytes + b.sourceBytes,
          targetBytes: p.targetBytes + b.targetBytes,
        }),
        { sourceBytes: 0, targetBytes: 0 },
      );
    const savedBytes = p.sourceBytes - p.targetBytes;
    return {
      ...p,
      savedBytes,
      compressionPercent: p.sourceBytes
        ? (100 * savedBytes) / p.sourceBytes
        : 0,
    };
  }
  function periodQuery() {
    return customRange
      ? new URLSearchParams(customRange).toString()
      : `hours=${hours}`;
  }
  function rangeLabel() {
    const start = customRange?.start || data?.range?.start;
    const end = customRange?.end || data?.range?.end;
    if (!start || !end) return $("period").selectedOptions[0].textContent;
    const format = (date) =>
      new Date(date).toLocaleString("en-US", {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    if (customRange) {
      const day = (date) =>
        new Date(date).toLocaleDateString("en-US", {
          month: "short",
          day: "numeric",
          year: "numeric",
        });
      return `${day(start)} – ${day(Date.parse(end) - 1)} · local dates`;
    }
    return `${format(start)} – ${format(end)}`;
  }
  const chartDate = (time) =>
    new Date(time).toLocaleString(
      "en-US",
      (
        data.range
          ? Date.parse(data.range.end) - Date.parse(data.range.start) <=
            86400000
          : hours === 24
      )
        ? { hour: "2-digit", minute: "2-digit" }
        : { day: "numeric", month: "short" },
    );
  function renderThroughput() {
    const s = data.summary,
      buckets = data.timeline;
    $("throughput-total").textContent =
      `${integer.format(s.completedInRange)} completed`;
    if (!s.completedInRange && !s.failedInRange) {
      $("throughput-chart").innerHTML = empty("No outcomes in this period", "");
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
    const s = periodSummary();
    if (!s.sourceBytes) {
      $("storage-impact").innerHTML = empty("No measured savings", "");
      return;
    }
    $("storage-impact").innerHTML =
      `<div class="impact-number">${number.format(s.compressionPercent)}<span>%</span></div><p class="impact-subtitle">space saved · ${bytes(s.savedBytes)}</p><div class="storage-row"><span>Source</span><div class="storage-bar"><span data-width="100"></span></div><strong>${bytes(s.sourceBytes)}</strong></div><div class="storage-row output"><span>Output</span><div class="storage-bar"><span data-width="${(s.targetBytes / s.sourceBytes) * 100}"></span></div><strong>${bytes(s.targetBytes)}</strong></div>`;
  }
  function renderQueue() {
    const s = data.summary;
    if (!s.total) {
      $("queue-chart").innerHTML = empty("No jobs", "");
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
      $("savings-chart").innerHTML = empty("No savings in this period", "");
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
      $("codec-chart").innerHTML = empty("No completed outputs", "");
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
      .sort(
        (a, b) =>
          Number(b.online) - Number(a.online) ||
          b.activeJobs - a.activeJobs ||
          b.completed - a.completed ||
          a.name.localeCompare(b.name),
      )
      .slice(0, 6);
    if (!workers.length) {
      $("worker-chart").innerHTML = empty("No recent workers", "");
      return;
    }
    const max = Math.max(1, ...workers.map((w) => w.completed + w.failed));
    $("worker-chart").innerHTML =
      `<div class="horizontal-chart">${workers.map((w) => `<a class="worker-summary-link" href="${esc(resources.link(w.name))}"><div class="bar-label"><strong><span class="status-dot ${w.online ? "" : "offline"}"></span> ${esc(w.name)}</strong><span>${integer.format(w.completed)} ✓ · ${integer.format(w.failed)} failures</span></div>${w.online ? resources.summary(w, Date.parse(data.generatedAt)) : badge("offline", "Offline")}<div class="bar-track"><span data-width="${(w.completed / max) * 100}"></span><span class="failed" data-width="${(w.failed / max) * 100}"></span></div><span class="worker-details-link">Resource details ↗</span></a>`).join("")}</div>`;
  }
  function renderActivity() {
    if (!data.activity.length) {
      $("activity-feed").innerHTML = empty("No events in this period", "");
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
    return `<div class="load-labels"><span>${esc((phases[j.phase] ?? j.phase) + (j.message ? " · " + j.message : ""))}</span><strong>${number.format(percent)} %</strong></div><div class="progress-track"><span data-width="${percent}"></span></div><div class="progress-caption ${stale ? "stale" : ""}"><span>${stale ? "Outdated progress" : j.held ? "Progress retained" : "Phase ETA: " + duration(eta)}</span><span>${j.progress.length > 1 ? `${j.progress.length} subtasks` : ""}</span></div>`;
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
      return `<span class="muted">${job.progress?.length ? number.format(job.progress[0].percent) + "% · " : ""}Paused in RAM</span>`;
    if (local?.queueHeld) return '<span class="muted">Queue held</span>';
    return jobProgress(job);
  }
  function jobsTable(jobs) {
    if (!jobs.length) return empty("No jobs", "");
    return `<table><thead><tr><th>File</th><th>Status</th><th>Worker</th><th>Progress / result</th><th>Updated</th><th>Video</th></tr></thead><tbody>${jobs.map((j) => `<tr><td class="file-cell"><div class="file-name"><span aria-hidden="true">▹</span><div><strong title="${esc(j.sourcePath)}">${esc(basename(j.sourcePath))}</strong><small title="${esc(j.id)}">${bytes(j.sourceBytes)} · ${esc(j.id.slice(0, 8))}</small></div></div></td><td>${jobBadge(j)}</td><td>${esc(j.worker || "—")}</td><td><div class="job-progress">${observedProgress(j)}</div></td><td title="${esc(new Date(j.updatedAt).toLocaleString("en-US"))}">${age(j.updatedAt)}</td><td>${["queued", "completed"].includes(j.state) ? `<button type="button" class="button media-job-play" data-job-media="${esc(j.id)}" aria-label="Play ${j.state === "completed" ? "completed output" : "queued source"} ${esc(basename(j.sourcePath))}">▷ View ${j.state === "completed" ? "output" : "source"}</button>` : `<span class="muted">—</span>`}</td></tr>`).join("")}</tbody></table>`;
  }
  function pagination(total, offset, limit, kind) {
    if (!total) return "";
    return `<span>${integer.format(offset + 1)}–${integer.format(Math.min(offset + limit, total))} of ${integer.format(total)}</span><button class="button" type="button" data-${kind}-page="${offset - limit}" ${offset <= 0 ? "disabled" : ""}>Previous</button><button class="button" type="button" data-${kind}-page="${offset + limit}" ${offset + limit >= total ? "disabled" : ""}>Next</button>`;
  }
  async function renderJobs() {
    if (!data) return;
    const generation = ++jobsGeneration;
    const query = new URLSearchParams({
      state: filter,
      search: $("job-search").value.trim(),
      offset: String(jobsOffset),
      limit: "25",
    });
    try {
      const page = await api(`jobs?${query}`);
      if (generation !== jobsGeneration) return;
      if (page.total > 0 && page.offset >= page.total) {
        jobsOffset = Math.floor((page.total - 1) / page.limit) * page.limit;
        return renderJobs();
      }
      $("job-list").innerHTML = jobsTable(page.jobs);
      $("job-pagination").innerHTML = pagination(
        page.total,
        page.offset,
        page.limit,
        "job",
      );
      applyWidths($("job-list"));
    } catch (error) {
      if (generation !== jobsGeneration) return;
      $("job-list").innerHTML = empty("Could not load jobs", error.message);
      $("job-pagination").innerHTML = "";
    }
  }
  function reloadJobs() {
    jobsOffset = 0;
    jobsGeneration++;
    $("job-list").innerHTML = empty("Loading jobs…");
    $("job-pagination").innerHTML = "";
    renderJobs();
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
  function visibleWorkers() {
    if (!data) return [];
    const since = customRange
      ? Date.parse(customRange.start)
      : Date.parse(data.generatedAt) - hours * 3600000;
    const until = customRange ? Date.parse(customRange.end) : Infinity;
    return data.workers.filter(
      (w) =>
        w.lastSeen &&
        Date.parse(w.lastSeen) >= since &&
        Date.parse(w.lastSeen) < until,
    );
  }
  function renderWorkers() {
    if (!data) return;
    const periodWorkers = visibleWorkers();
    $("worker-scope-note").textContent =
      `${periodWorkers.length} workers · last seen in selected period (up to 30 days) · live resources`;
    if (!periodWorkers.length) {
      $("worker-pagination").innerHTML = "";
      $("worker-cards").innerHTML = empty("No workers in this period", "");
      return;
    }
    const all = periodWorkers.filter(
      (worker) => !selectedWorker || worker.name === selectedWorker,
    );
    workerPage = Math.max(
      0,
      Math.min(workerPage, Math.ceil(all.length / 12) - 1),
    );
    const visible = selectedWorker
      ? all
      : all.slice(workerPage * 12, workerPage * 12 + 12);
    $("worker-pagination").innerHTML = selectedWorker
      ? ""
      : pagination(all.length, workerPage * 12, 12, "worker");
    $("worker-cards").innerHTML = visible
      .map((w, i) => {
        if (!w.online)
          return `<article class="panel worker-card worker-offline"><div class="worker-heading"><div><h2>${esc(w.name)}</h2><p>Last seen ${age(w.lastSeen)}</p></div>${badge("offline", "Offline")}</div><p class="media-help">${esc(w.ip || "Address unavailable")} · Offline</p></article>`;
        const rt = w.runtime,
          modern = rt?.capacityVersion >= 2;
        const profile = resources.profile(w);
        const desired = workerLimits(w),
          draft = drafts.get(w.name) ?? desired;
        const pending =
          (w.desiredCPUJobs != null || w.desiredMax != null) &&
          (!modern ||
            Number(desired.cpu) !== rt.cpuJobs ||
            Number(desired.gpu) !== rt.gpuJobs);
        const paused =
          modern && (rt.paused || (rt.cpuJobs === 0 && rt.gpuJobs === 0));
        const status = !modern
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
        const command = w.control;
        const pendingControl = command?.state === "pending";
        const busy = saving.has(w.name),
          disabled = busy || !modern || pendingControl;
        const controlDisabled =
          disabled || !(rt?.controlVersion >= 1) || rt?.draining;
        const controlNote = pendingControl
          ? `Waiting for worker confirmation: ${command.action}. `
          : command?.state === "failed"
            ? command.error || "The worker could not apply the control."
            : !(rt?.controlVersion >= 1)
              ? "Update this worker to enable remote RAM and queue controls."
              : "";
        const controls = `<div class="worker-controls"><button class="button subtle" type="button" data-worker-control="${rt?.ramPaused ? "resume-encodes" : "pause-encodes"}" data-worker-name="${esc(w.name)}" ${controlDisabled || rt?.finishing ? "disabled" : ""}>${rt?.ramPaused ? "Resume encodes" : "Pause in RAM"}</button><button class="button subtle" type="button" data-worker-control="${rt?.maxActiveJobs === 0 ? "resume-queue" : "stop-after-current"}" data-worker-name="${esc(w.name)}" ${controlDisabled ? "disabled" : ""}>${rt?.maxActiveJobs === 0 ? "Resume queue" : "Stop after current"}</button></div><p class="pending-note ${pendingControl ? "pending" : command?.state === "failed" ? "control-error" : ""}">${esc(controlNote)}</p>`;
        const lane = (kind) => {
          const active =
              rt?.[kind === "cpu" ? "activeCPUEncodes" : "activeGPUEncodes"],
            limit = rt?.[`${kind}Jobs`];
          return `<div class="encoder-lane ${kind}"><div class="load-labels"><span><b>${kind.toUpperCase()}</b> · ${esc(rt?.[`${kind}Encoder`] || rt?.encoder || "No telemetry")}</span><strong>${modern ? `${active} / ${limit}` : "—"}</strong></div><div class="progress-track"><span data-width="${modern && limit > 0 ? (active / limit) * 100 : 0}"></span></div><p class="lane-note">${!modern ? "Update the worker to enable capacity controls." : limit === 0 ? "New encodes stopped" : `${limit} encoding slots · ${active} occupied`}</p></div>`;
        };
        const input = (kind, enabled) =>
          enabled
            ? `<div><label for="${kind}-${i}">${kind.toUpperCase()} limit <span>ENCODES</span></label><input id="${kind}-${i}" name="${kind}Jobs" type="number" min="0" step="1" required inputmode="numeric" value="${esc(draft[kind])}" ${rt?.managedProfile ? 'max="64"' : ""} ${disabled ? "disabled" : ""} aria-label="${kind.toUpperCase()} limit for ${esc(w.name)}"></div>`
            : `<input type="hidden" name="${kind}Jobs" value="0">`;
        const profileLabel = rt?.managedProfile
          ? `Windows ${profile.cpu ? "CPU" : "GPU"} profile · shared host${rt.resources?.hostname ? " " + rt.resources.hostname : ""}`
          : "CPU + GPU worker";
        const note = pending
          ? `Limits pending${rt.managedProfile ? " for this profile" : ""}.`
          : !modern
            ? "Update this worker to enable controls."
            : rt.draining
              ? "Maintenance: finishing accepted work."
              : rt.finishing
                ? "Finishing accepted work; admission remains held."
                : rt.ramPaused
                  ? `RAM pause · ${rt.suspendedEncodes || 0} encodes suspended; limits retained.`
                  : paused
                    ? "Queue stopped. Active encodes finish; queued encodes wait."
                    : "";
        return `<article class="panel worker-card" data-worker="${esc(w.name)}"><div class="worker-heading"><span class="worker-icon" aria-hidden="true">▦</span><div><h2>${esc(w.name)}</h2><p>${esc(profileLabel)}</p></div>${badge(...status)}</div><p class="media-help">${rt ? rt.activeJobs : w.activeJobs} accepted jobs${rt?.managedProfile ? " · shared host resources" : ""}</p>${resources.summary(w, Date.parse(data.generatedAt))}${resources.compact(w, resourceHistory, Date.parse(data.generatedAt))}<a class="text-link worker-resource-link" href="${esc(resources.link(w.name))}">View resource details ↗</a><div class="encoder-lanes">${profile.cpu ? lane("cpu") : ""}${profile.gpu ? lane("gpu") : ""}</div><form class="worker-capacity" data-worker="${esc(w.name)}"><div class="encoder-inputs">${input("cpu", profile.cpu)}${input("gpu", profile.gpu)}</div><div class="capacity-controls"><button class="button primary" type="submit" ${disabled ? "disabled" : ""}>${busy ? "Saving…" : "Save limits"}</button></div><p class="pending-note ${pending ? "pending" : ""}">${esc(note)}</p></form>${controls}${workerTasks(w)}<div class="worker-footer"><span>${esc(w.ip)} · ${age(w.lastSeen)}</span><span>Period: ${integer.format(w.completed)} completed · ${integer.format(w.failed)} failed · ${bytes(w.sourceBytes - w.targetBytes)} saved</span></div></article>`;
      })
      .join("");
    applyWidths($("worker-cards"));
  }
  function workerTasks(worker) {
    const rt = worker.runtime || {};
    const known = data.jobs.filter(
      (j) => j.worker === worker.name && j.state === "active",
    );
    const tasks = rt?.jobs?.length
      ? rt.jobs
      : known.map((j) => ({ id: j.id, phase: j.phase, state: "running" }));
    if (!tasks.length)
      return '<section class="worker-tasks"><h3>Tasks</h3><p class="muted">Idle</p></section>';
    return `<section class="worker-tasks"><h3>Tasks <span class="muted">${tasks.length}</span></h3>${tasks
      .map((task) => {
        const job = known.find((j) => j.id === task.id);
        const progress = task.progress?.length
          ? task.progress
          : job?.progress || [];
        const paused = task.state === "paused";
        const held =
          task.state === "queued" && (rt.paused || rt.maxActiveJobs === 0);
        const control =
          rt.controlVersion >= 2 &&
          task.canPause &&
          task.phase === "FFMPEG" &&
          ["running", "paused"].includes(task.state);
        const unavailable =
          saving.has(worker.name) ||
          worker.control?.state === "pending" ||
          rt.draining ||
          rt.finishing;
        const name =
          task.name || (job ? basename(job.sourcePath) : task.id.slice(0, 8));
        const state = task.individuallyPaused
          ? "Paused individually"
          : task.pausedByWorker
            ? "Worker RAM hold"
            : held
              ? "Queue held"
              : task.state === "queued"
                ? "Waiting"
                : "";
        return `<article class="worker-task" data-task-id="${esc(task.id)}"><div class="task-heading"><strong title="${esc(name)}">${esc(name)}</strong>${control ? `<button type="button" class="button subtle" data-worker-control="${task.individuallyPaused ? "resume-job" : "pause-job"}" data-worker-name="${esc(worker.name)}" data-job-id="${esc(task.id)}" ${unavailable ? "disabled" : ""} aria-label="${task.individuallyPaused ? "Resume" : "Pause"} ${esc(name)}">${task.individuallyPaused ? "Resume" : "Pause"}</button>` : ""}</div>${state ? `<p class="task-phase">${esc(state)}${task.individuallyPaused && task.pausedByWorker ? " · Worker RAM hold" : ""}</p>` : ""}<div class="job-progress">${jobProgress({ state: "active", held: paused || held, phase: task.phase, progress: progress.map((p) => ({ ...p, etaSeconds: paused || held ? 0 : p.etaSeconds })) })}</div></article>`;
      })
      .join(
        "",
      )}${rt.controlVersion < 2 ? '<p class="media-help">Update worker for individual pause controls.</p>' : ""}</section>`;
  }
  function renderWorkerDetails() {
    const show = view === "workers" && selectedWorker !== null;
    $("worker-detail-navigation").hidden = !show;
    $("worker-resource-detail").hidden = !show;
    if (!show || !data) return;
    const worker = visibleWorkers().find((w) => w.name === selectedWorker);
    $("worker-resource-detail").innerHTML = worker
      ? worker.online
        ? resources.details(
            worker,
            resourceHistory,
            Date.parse(data.generatedAt),
          )
        : empty(
            "Worker offline",
            "Last seen " +
              age(worker.lastSeen) +
              ". Hardware details and controls are available after reconnection.",
          )
      : empty(
          "Worker not seen in this period",
          "Choose a longer reporting period or return to all workers.",
        );
    applyWidths($("worker-resource-detail"));
  }
  async function saveWorker(name, values) {
    const w = data.workers.find((worker) => worker.name === name);
    if (
      !w ||
      !w.online ||
      !(w.runtime?.capacityVersion >= 2) ||
      saving.has(name)
    )
      return;
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
        w.runtime?.managedProfile
          ? `${name}: ${resources.profile(w).cpu ? "CPU " + cpu : "GPU " + gpu} saved. Applies at the next synchronization.`
          : `${name}: CPU ${cpu} · GPU ${gpu} saved. Applies at the next synchronization.`,
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
    selectedWorker =
      view === "workers" ? new URLSearchParams(query).get("worker") : null;
    if (!titles[view]) view = "overview";
    const label = titles[view];
    $("breadcrumb-view").textContent = label;
    $("page-title").textContent = label;
    $("reporting-control").hidden = !["overview", "workers"].includes(view);
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
    const custom = $("period").value === "custom";
    $("custom-period").hidden = !custom;
    if (custom) {
      if (!$("period-start").value) {
        const date = new Date();
        const localDate = (d) =>
          `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
        $("period-end").value = localDate(date);
        date.setDate(date.getDate() - 6);
        $("period-start").value = localDate(date);
      }
      return;
    }
    customRange = null;
    hours = Number($("period").value);
    workerPage = 0;
    refresh();
  });
  $("custom-period").addEventListener("submit", (event) => {
    event.preventDefault();
    const start = new Date(`${$("period-start").value}T00:00:00`);
    const end = new Date(`${$("period-end").value}T00:00:00`);
    end.setDate(end.getDate() + 1);
    if (
      !Number.isFinite(start.getTime()) ||
      !Number.isFinite(end.getTime()) ||
      end <= start ||
      end - start > 3660 * 86400000
    ) {
      showToast("Choose dates in order, spanning at most 3660 days.", true);
      return;
    }
    customRange = { start: start.toISOString(), end: end.toISOString() };
    workerPage = 0;
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
  $("job-search").addEventListener("input", () => {
    jobsGeneration++;
    clearTimeout(jobSearchTimer);
    jobSearchTimer = setTimeout(reloadJobs, 250);
  });
  $("job-pagination").addEventListener("click", (event) => {
    const button = event.target.closest("[data-job-page]");
    if (!button || button.disabled) return;
    jobsOffset = Number(button.dataset.jobPage);
    renderJobs();
  });
  $("worker-pagination").addEventListener("click", (event) => {
    const button = event.target.closest("[data-worker-page]");
    if (!button || button.disabled) return;
    workerPage = Number(button.dataset.workerPage) / 12;
    renderWorkers();
  });
  document.querySelectorAll("[data-filter]").forEach((button) =>
    button.addEventListener("click", () => {
      filter = button.dataset.filter;
      document.querySelectorAll("[data-filter]").forEach((b) => {
        b.classList.toggle("active", b === button);
        b.setAttribute("aria-pressed", String(b === button));
      });
      reloadJobs();
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
  async function controlWorker(name, action, jobId) {
    const worker = data?.workers.find((w) => w.name === name);
    if (
      !worker?.online ||
      !(worker.runtime?.controlVersion >= 1) ||
      saving.has(name) ||
      worker.control?.state === "pending"
    )
      return;
    saving.add(name);
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    renderWorkers();
    try {
      const command = await api(`workers/${encodeURIComponent(name)}/control`, {
        method: "POST",
        body: JSON.stringify({ action, ...(jobId ? { jobId } : {}) }),
      });
      worker.control = command;
      showToast(`${name}: command queued; waiting for worker confirmation.`);
      await refresh();
    } catch (error) {
      showToast(error.message, true);
    } finally {
      saving.delete(name);
      renderWorkers();
    }
  }
  $("worker-cards").addEventListener("click", (event) => {
    const button = event.target.closest("[data-worker-control]");
    if (button && !button.disabled)
      controlWorker(
        button.dataset.workerName,
        button.dataset.workerControl,
        button.dataset.jobId,
      );
  });
  window.addEventListener("hashchange", setView);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) clearTimeout(timer);
    else refresh();
  });
  async function loadIdentity() {
    try {
      const identity = await api("identity");
      $("session-name").textContent =
        identity.name ||
        identity.username ||
        identity.email ||
        (identity.mode === "token" ? "Token session" : "Signed-in user");
      $("session-account").textContent =
        identity.email ||
        identity.username ||
        (identity.mode === "token"
          ? "Dashboard token"
          : "Sign in again to show your account");
      $("session-mobile").textContent =
        identity.email ||
        identity.username ||
        identity.name ||
        (identity.mode === "token" ? "Token session" : "Signed in");
      $("session-mobile").title = $("session-mobile").textContent;
      $("session-name").title = $("session-name").textContent;
      $("session-account").title = $("session-account").textContent;
    } catch (error) {
      $("session-account").textContent = "Account unavailable";
    }
  }
  setView();
  refresh();
  loadIdentity();
})();
