"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const number = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 1 });
  const integer = new Intl.NumberFormat("es-ES", { maximumFractionDigits: 0 });
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
    active: "Activo",
    queued: "En cola",
    completed: "Completado",
    failed: "Fallido",
    canceled: "Cancelado",
    assigned: "Asignado",
    started: "Iniciado",
  };
  const phases = {
    Job: "Preparación",
    Download: "Descarga",
    MKVExtract: "Subtítulos",
    PGS: "OCR",
    FFMPEG: "Codificación",
    JobVerify: "Verificación",
    Upload: "Subida",
  };
  const titles = {
    library: [
      "Biblioteca",
      "Tu biblioteca, sin terminal",
      "Explora, reproduce y elige qué vídeos cargar en la cola.",
    ],
    overview: [
      "Resumen",
      "Tu flota, en movimiento",
      "Cada trabajo, cada worker y cada byte que recuperas.",
    ],
    workers: [
      "Workers",
      "La potencia, bajo tu control",
      "Ajusta CPU y GPU por separado en cada worker, sin reiniciar.",
    ],
    jobs: [
      "Trabajos",
      "Cada archivo tiene su historia",
      "Sigue las fases, el progreso y el resultado de cada trabajo.",
    ],
  };
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
    if (!time) return "sin heartbeat";
    const seconds = Math.max(0, (Date.now() - new Date(time)) / 1000);
    if (seconds < 60) return `hace ${Math.floor(seconds)} s`;
    if (seconds < 3600) return `hace ${Math.floor(seconds / 60)} min`;
    if (seconds < 86400) return `hace ${Math.floor(seconds / 3600)} h`;
    return `hace ${Math.floor(seconds / 86400)} d`;
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
      throw new Error("La sesión ha caducado.");
    }
    if (!response.ok)
      throw new Error(
        response.status === 400
          ? "Revisa los datos introducidos."
          : "El servidor no pudo completar la operación.",
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
      $("connection").innerHTML = '<span class="status-dot"></span>En directo';
      $("last-sync").textContent = new Date(
        data.generatedAt,
      ).toLocaleTimeString("es-ES", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
      render();
    } catch (e) {
      $("loading").hidden = true;
      $("error-banner").textContent =
        `${e.message} ${data ? "Mostrando los últimos datos recibidos; pueden estar desactualizados." : "Reintentaremos automáticamente en unos segundos."}`;
      $("error-banner").hidden = false;
      $("connection").innerHTML =
        '<span class="status-dot error"></span>Sin conexión';
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
    const online = workers.filter((w) => w.online).length;
    $("nav-worker-count").textContent = workers.length;
    $("nav-job-count").textContent = integer.format(s.active + s.queued);
    $("fleet-note").textContent =
      `${online} de ${workers.length} workers online`;
    $("fleet-detail").textContent = workers.length
      ? `${integer.format(s.active)} activos · ${integer.format(s.queued)} en cola`
      : "Esperando el primer worker";
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
        "Espacio recuperado",
        "↙",
        `${saved[0]}<small>${saved[1]}</small>`,
        `<span class="positive">${s.sourceBytes ? `${number.format(s.compressionPercent)} % menos espacio` : "Esperando el primer resultado"}</span> · acumulado`,
      ],
      [
        "Trabajos completados",
        "✓",
        integer.format(s.completedInRange),
        `${integer.format(s.completed)} en total · <span class="positive">${integer.format(s.queued)} en cola</span>`,
      ],
      [
        "Encodes en marcha",
        "ϟ",
        live.length ? `${encodes}<small>/ ${target} slots</small>` : "—",
        split.length
          ? `<span class="positive">CPU ${cpuActive}</span> · <span class="purple-text">GPU ${gpuActive}</span> · ${integer.format(s.active)} jobs activos`
          : `${integer.format(s.active)} trabajos activos · sin desglose CPU/GPU`,
      ],
      [
        "Tasa de éxito",
        "↗",
        `${success}${outcomes ? "<small>%</small>" : ""}`,
        `${integer.format(s.failedInRange)} fallidos · media ${duration(s.averageSeconds)} / trabajo`,
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
      "es-ES",
      hours === 24
        ? { hour: "2-digit", minute: "2-digit" }
        : { day: "numeric", month: "short" },
    );
  function renderThroughput() {
    const s = data.summary,
      buckets = data.timeline;
    $("throughput-total").textContent =
      `${integer.format(s.completedInRange)} completados`;
    if (!s.completedInRange && !s.failedInRange) {
      $("throughput-chart").innerHTML = empty(
        "Tu próximo resultado empieza aquí",
        "Las finalizaciones y los fallos aparecerán en este gráfico cuando lleguen.",
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
      `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${s.completedInRange} trabajos completados y ${s.failedInRange} fallidos en el periodo"><defs><linearGradient id="throughput-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="#b7ef79" stop-opacity=".16"/><stop offset="100%" stop-color="#b7ef79" stop-opacity="0"/></linearGradient></defs>${grid}<path class="chart-area" d="${points("completed")} L${x(buckets.length - 1)},${y(0)} L${x(0)},${y(0)} Z"/><path class="chart-line" d="${points("completed")}"/><path class="chart-failed" d="${points("failed")}"/>${buckets.map((b, i) => `<circle class="chart-point" cx="${x(i)}" cy="${y(b.completed)}" r="${b.completed > 0 ? 3 : 0}"/><rect class="chart-hover" x="${Math.max(left, x(i) - 8)}" y="${top}" width="16" height="${height - bottom - top}"><title>${esc(chartDate(b.time))}: ${b.completed} completados · ${b.failed} fallidos</title></rect>`).join("")}${labelIndices.map((i, n) => `<text class="chart-label" x="${x(i)}" y="${height - 7}" text-anchor="${n === 0 ? "start" : n === labelIndices.length - 1 ? "end" : "middle"}">${esc(chartDate(buckets[i].time))}</text>`).join("")}</svg>`;
  }
  function renderImpact() {
    const s = data.summary;
    if (!s.sourceBytes) {
      $("storage-impact").innerHTML = empty(
        "El ahorro se mide al terminar",
        "Los tamaños finales verificados se comparan con sus archivos de origen.",
      );
      return;
    }
    $("storage-impact").innerHTML =
      `<div class="impact-number">${number.format(s.compressionPercent)}<span>%</span></div><p class="impact-subtitle">de espacio recuperado · ${bytes(s.savedBytes)}</p><div class="storage-row"><span>Origen</span><div class="storage-bar"><span data-width="100"></span></div><strong>${bytes(s.sourceBytes)}</strong></div><div class="storage-row output"><span>Destino</span><div class="storage-bar"><span data-width="${(s.targetBytes / s.sourceBytes) * 100}"></span></div><strong>${bytes(s.targetBytes)}</strong></div>`;
  }
  function renderQueue() {
    const s = data.summary;
    if (!s.total) {
      $("queue-chart").innerHTML = empty(
        "Todo listo para empezar",
        "Programa archivos desde la API de TranscoderD para ver tu cola.",
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
      `<div class="queue-layout"><div class="queue-ring"><svg viewBox="0 0 126 126" role="img" aria-label="Estado de ${s.total} trabajos"><circle class="ring-base" cx="63" cy="63" r="49"/>${rings}</svg><div class="ring-total"><strong>${integer.format(s.total)}</strong><small>TRABAJOS</small></div></div><div class="queue-legend">${segments.map(([key, value, , color]) => `<div><i class="legend-dot ${color}"></i>${stateNames[key]}<strong>${integer.format(value)}</strong></div>`).join("")}</div></div>`;
  }
  function renderSavings() {
    const buckets = data.timeline;
    const total = buckets.reduce(
      (n, b) => n + b.sourceBytes - b.targetBytes,
      0,
    );
    if (!buckets.some((b) => b.sourceBytes)) {
      $("savings-chart").innerHTML = empty(
        "El espacio también cuenta una historia",
        "Aquí verás cuánto recuperas en cada intervalo.",
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
      `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${esc(bytes(total))} recuperados en el periodo">${[
        0, 0.5, 1,
      ]
        .map((f) => {
          const value = minimum + range * f;
          return `<line class="chart-gridline" x1="${left}" y1="${y(value)}" x2="${width - 8}" y2="${y(value)}"/><text class="chart-label" x="${left - 6}" y="${y(value) + 3}" text-anchor="end">${esc(bytes(value))}</text>`;
        })
        .join(
          "",
        )}${buckets.map((b, i) => `<rect x="${x(i) + 3}" y="${Math.min(y(values[i]), y(0))}" width="${barSize}" height="${Math.max(0, Math.abs(y(values[i]) - y(0)))}" rx="2" fill="${values[i] < 0 ? "#ff8992" : "#ae9bff"}"><title>${esc(chartDate(b.time))}: ${esc(bytes(values[i]))}</title></rect>`).join("")}<text class="chart-label" x="${left}" y="${height - 5}">${esc(chartDate(buckets[0].time))}</text><text class="chart-label" x="${width - 8}" y="${height - 5}" text-anchor="end">${esc(chartDate(buckets.at(-1).time))}</text></svg><div class="graph-footer"><span>Recuperado en este periodo</span><strong>${bytes(total)}</strong></div>`;
  }
  function renderCodecs() {
    if (!data.codecs.length) {
      $("codec-chart").innerHTML = empty(
        "Aún no hay códecs que comparar",
        "Se usan los tamaños de archivos finalizados, agrupados por códec de salida.",
      );
      return;
    }
    $("codec-chart").innerHTML = `<div class="horizontal-chart">${data.codecs
      .slice(0, 5)
      .map((c) => {
        const saved = 100 * (1 - c.targetBytes / c.sourceBytes);
        return `<div><div class="bar-label"><strong>${esc(c.name === "unknown" ? "Sin probe de salida" : c.name.toUpperCase())}</strong><span>${number.format(saved)} % menos</span></div><div class="bar-track codec"><span data-width="${Math.max(0, saved)}"></span></div><div class="bar-detail"><span>${integer.format(c.jobs)} archivos</span><span>${bytes(c.sourceBytes)} → ${bytes(c.targetBytes)}</span></div></div>`;
      })
      .join("")}</div>`;
  }
  function renderWorkerChart() {
    const workers = [...data.workers]
      .sort((a, b) => b.completed - a.completed || a.name.localeCompare(b.name))
      .slice(0, 6);
    if (!workers.length) {
      $("worker-chart").innerHTML = empty(
        "Tu flota aún está por llegar",
        "Los workers aparecen cuando envían su primer heartbeat.",
      );
      return;
    }
    const max = Math.max(1, ...workers.map((w) => w.completed + w.failed));
    $("worker-chart").innerHTML =
      `<div class="horizontal-chart">${workers.map((w) => `<div><div class="bar-label"><strong><span class="status-dot ${w.online ? "" : "offline"}"></span> ${esc(w.name)}</strong><span>${integer.format(w.completed)} ✓ · ${integer.format(w.failed)} fallos</span></div><div class="bar-track"><span data-width="${(w.completed / max) * 100}"></span><span class="failed" data-width="${(w.failed / max) * 100}"></span></div></div>`).join("")}</div><p class="worker-chart-footer">Resultados por worker en el periodo. La capacidad no equivale a utilización de CPU/GPU.</p>`;
  }
  function renderActivity() {
    if (!data.activity.length) {
      $("activity-feed").innerHTML = empty(
        "Sin eventos en este periodo",
        "Las asignaciones, finalizaciones y errores aparecerán aquí.",
      );
      return;
    }
    $("activity-feed").innerHTML = data.activity
      .slice(0, 5)
      .map(
        (a) =>
          `<article class="activity-item"><span class="activity-icon ${esc(a.status)}" aria-hidden="true">${a.status === "completed" ? "✓" : a.status === "failed" ? "!" : "↗"}</span><div class="activity-content"><strong title="${esc(a.sourcePath)}">${esc(basename(a.sourcePath))}</strong><p>${esc(stateNames[a.status] ?? a.status)} · ${esc(a.worker || "Servidor")}${a.phase !== "Job" ? " · " + esc(phases[a.phase] ?? a.phase) : ""}</p>${a.message ? `<p class="activity-error" title="${esc(a.message)}">${esc(a.message)}</p>` : ""}</div><time datetime="${esc(a.time)}" title="${esc(new Date(a.time).toLocaleString("es-ES"))}">${age(a.time)}</time></article>`,
      )
      .join("");
  }
  function jobProgress(j) {
    if (j.state === "completed") {
      const savings =
        j.sourceBytes > 0 && j.targetBytes > 0
          ? `${number.format((1 - j.targetBytes / j.sourceBytes) * 100)} % menos · ${bytes(j.targetBytes)}`
          : "Tamaño final no disponible";
      return `<span class="accent">✓ Finalizado</span><div class="progress-caption">${esc(savings)}</div>`;
    }
    if (j.state === "failed")
      return `<p class="job-message" title="${esc(j.message)}">${esc(j.message || "El worker ha informado un error.")}</p>`;
    if (j.state === "queued")
      return '<span class="muted">Esperando asignación</span>';
    if (j.state === "canceled") return '<span class="muted">Cancelado</span>';
    if (!j.progress.length)
      return `<span>${esc((phases[j.phase] ?? j.phase) + (j.message ? " · " + j.message : ""))}</span><div class="progress-caption">Esperando progreso</div>`;
    const percent =
      j.progress.reduce((n, p) => n + p.percent, 0) / j.progress.length;
    const eta = Math.max(...j.progress.map((p) => p.etaSeconds));
    const stale = j.progress.some((p) => p.stale);
    return `<div class="load-labels"><span>${esc((phases[j.phase] ?? j.phase) + (j.message ? " · " + j.message : ""))}</span><strong>${number.format(percent)} %</strong></div><div class="progress-track"><span data-width="${percent}"></span></div><div class="progress-caption ${stale ? "stale" : ""}"><span>${stale ? "Progreso desactualizado" : "ETA de fase: " + duration(eta)}</span><span>${j.progress.length > 1 ? `${j.progress.length} subtareas` : ""}</span></div>`;
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
          local.individuallyPaused ? "Pausa individual" : "Pausa RAM",
        )
      : local?.queueHeld
        ? badge("queued", "Cola detenida")
        : badge(job.state);
  }
  function observedProgress(job) {
    const local = observedJob(job);
    if (local?.state === "paused")
      return `<span class="muted">${job.progress?.length ? number.format(job.progress[0].percent) + "% · " : ""}Suspendido en RAM · progreso conservado</span>`;
    if (local?.queueHeld)
      return '<span class="muted">En espera de reanudar la cola</span>';
    return jobProgress(job);
  }
  function jobsTable(jobs) {
    if (!jobs.length)
      return empty(
        "No hay trabajos que mostrar",
        "Los archivos aparecerán aquí cuando se incorporen a la cola o cambies el filtro.",
      );
    return `<table><thead><tr><th>Archivo</th><th>Estado</th><th>Worker</th><th>Progreso / resultado</th><th>Actualización</th><th>Vídeo</th></tr></thead><tbody>${jobs.map((j) => `<tr><td class="file-cell"><div class="file-name"><span aria-hidden="true">▹</span><div><strong title="${esc(j.sourcePath)}">${esc(basename(j.sourcePath))}</strong><small title="${esc(j.id)}">${bytes(j.sourceBytes)} · ${esc(j.id.slice(0, 8))}</small></div></div></td><td>${jobBadge(j)}</td><td>${esc(j.worker || "—")}</td><td><div class="job-progress">${observedProgress(j)}</div></td><td title="${esc(new Date(j.updatedAt).toLocaleString("es-ES"))}">${age(j.updatedAt)}</td><td><button type="button" class="button media-job-play" data-job-media="${esc(j.id)}" aria-label="Reproducir ${esc(basename(j.sourcePath))}">▷ Ver</button></td></tr>`).join("")}</tbody></table>`;
  }
  function renderJobs() {
    if (!data) return;
    const search = $("job-search").value.toLocaleLowerCase("es");
    const jobs = data.jobs.filter(
      (j) =>
        (filter === "all" || j.state === filter) &&
        (!search ||
          `${j.sourcePath} ${j.worker} ${j.id}`
            .toLocaleLowerCase("es")
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
        "Conecta tu primer worker",
        "Aparecerá automáticamente con su nombre, encoders y estado.",
      );
      return;
    }
    $("worker-cards").innerHTML = data.workers
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
            ? ["queued", "Actualizar worker"]
            : rt.draining
              ? ["queued", "Mantenimiento"]
              : rt.finishing
                ? ["queued", "Terminando jobs"]
                : rt.ramPaused
                  ? ["queued", "Pausa RAM"]
                  : paused
                    ? ["queued", "Cola pausada"]
                    : !rt.withinSchedule
                      ? ["queued", "Fuera de horario"]
                      : ["completed", "Online"];
        const busy = saving.has(w.name);
        const fixedCPU = rt?.managedProfile && rt.encoder !== "av1_nvenc",
          fixedGPU = rt?.managedProfile && rt.encoder === "av1_nvenc";
        const lane = (kind, encoder, active, limit) =>
          `<div class="encoder-lane ${kind}"><div class="load-labels"><span><b>${kind.toUpperCase()}</b> · ${esc(encoder || "Sin telemetría")}</span><strong>${modern ? `${active} / ${limit}` : "—"}</strong></div><div class="progress-track"><span data-width="${modern && limit > 0 ? (active / limit) * 100 : 0}"></span></div><p class="lane-note">${!modern ? "Actualiza este worker para observar y controlar ambos encoders." : active > limit ? `${active} slots ocupados · límite ${limit}` : limit === 0 ? "Desactivado para nuevos encodes" : `${limit} slots de codificación${w.online ? "" : " · último dato"}`}</p></div>`;
        const note = pending
          ? `◷ Deseado: CPU ${desired.cpu || "?"} · GPU ${desired.gpu || "?"} — ${!modern ? "requiere actualizar el worker" : w.online ? "pendiente de aplicar" : "se aplicará al reconectar"}`
          : !modern
            ? "Los límites se guardan juntos; el worker antiguo se pausa hasta actualizarlo."
            : rt.draining
              ? "Mantenimiento: termina el trabajo aceptado sin admitir más."
              : rt.finishing
                ? "Terminando trabajo aceptado; la sincronización mantiene este bloqueo."
                : rt.ramPaused
                  ? `Pausa RAM · ${rt.suspendedEncodes || 0} encodes suspendidos. Los límites se conservan.`
                  : paused
                    ? "Cola detenida. Los encodes activos terminan; los pendientes esperan reanudación."
                    : `✓ Límites aplicados · hasta ${rt.cpuJobs + rt.gpuJobs + 1} jobs en el pipeline`;
        return `<article class="panel worker-card" data-worker="${esc(w.name)}">
        <div class="worker-heading"><span class="worker-icon" aria-hidden="true">▦</span><div><h2>${esc(w.name)}</h2><p>${modern ? (rt.managedProfile ? "Perfil Windows · " + (fixedCPU ? "CPU" : "GPU") : "CPU + GPU · ejecución independiente") : esc(rt?.encoder || "Esperando telemetría compatible")}</p></div>${badge(...status)}</div>
        <div class="worker-stats"><div class="worker-stat"><small>Encodes CPU${w.online ? "" : " · último dato"}</small><strong>${modern ? rt.activeCPUEncodes : "—"}<span>${modern ? "/ " + rt.cpuJobs : ""}</span></strong></div><div class="worker-stat"><small>Encodes GPU${w.online ? "" : " · último dato"}</small><strong>${modern ? rt.activeGPUEncodes : "—"}<span>${modern ? "/ " + rt.gpuJobs : ""}</span></strong></div><div class="worker-stat"><small>Jobs aceptados</small><strong>${rt ? rt.activeJobs : w.activeJobs}</strong></div></div>
        <div class="encoder-lanes">${lane("cpu", rt?.cpuEncoder, rt?.activeCPUEncodes, rt?.cpuJobs)}${lane("gpu", rt?.gpuEncoder, rt?.activeGPUEncodes, rt?.gpuJobs)}</div>
        <form class="worker-capacity" data-worker="${esc(w.name)}"><div class="encoder-inputs"><div><label for="cpu-${i}">Límite CPU <span>ENCODES</span></label><input id="cpu-${i}" name="cpuJobs" type="number" min="0" step="1" required inputmode="numeric" value="${fixedGPU ? 0 : esc(draft.cpu)}" ${rt?.managedProfile ? 'max="64"' : ""} ${busy || fixedGPU ? "disabled" : ""} aria-label="Límite CPU de ${esc(w.name)}"></div><div><label for="gpu-${i}">Límite GPU <span>ENCODES</span></label><input id="gpu-${i}" name="gpuJobs" type="number" min="0" step="1" required inputmode="numeric" value="${fixedCPU ? 0 : esc(draft.gpu)}" ${rt?.managedProfile ? 'max="64"' : ""} ${busy || fixedCPU ? "disabled" : ""} aria-label="Límite GPU de ${esc(w.name)}"></div></div><div class="capacity-controls"><button class="button primary" type="submit" ${busy ? "disabled" : ""}>${busy ? "Guardando…" : "Guardar límites"}</button><button class="button subtle" type="button" data-pause="${esc(w.name)}" ${busy ? "disabled" : ""}>Pausar ambos</button></div><p class="pending-note ${pending ? "pending" : ""}">${esc(note)}</p></form>
        <div class="worker-footer"><span>${esc(w.ip || "Sin presencia registrada")} · ${age(w.lastSeen)}</span><span>${integer.format(w.completed)} completados · ${integer.format(w.failed)} fallos · ${bytes(w.sourceBytes - w.targetBytes)}</span></div>
      </article>`;
      })
      .join("");
    applyWidths($("worker-cards"));
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
      showToast("Introduce dos números enteros mayores o iguales a 0.", true);
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
        `${name}: CPU ${cpu} · GPU ${gpu} guardados. Se aplicarán en la próxima sincronización.`,
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
    view = window.location.hash.slice(1);
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
  }
  $("period").addEventListener("change", () => {
    hours = Number($("period").value);
    refresh();
  });
  $("refresh").addEventListener("click", refresh);
  $("logout").addEventListener("click", async () => {
    try {
      await api("logout", { method: "POST" });
      window.location.replace("/dashboard/login");
    } catch (e) {
      showToast(e.message, true);
    }
  });
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
