"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
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
  const num = new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 });
  const bytes = (n) => {
    if (!(n >= 0)) return "—";
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    const i = n ? Math.min(4, Math.floor(Math.log(n) / Math.log(1024))) : 0;
    return `${num.format(n / 1024 ** i)} ${units[i]}`;
  };
  const clock = (value) => {
    const s = Math.max(0, Math.floor(Number.isFinite(value) ? value : 0));
    const h = Math.floor(s / 3600);
    return `${h ? h + ":" : ""}${String(Math.floor(s / 60) % 60).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
  };
  const names = {
    queued: "Queued",
    active: "Active",
    completed: "Completed",
    failed: "Failed",
    canceled: "Canceled",
  };
  let roots = [],
    presets = {},
    root = "source",
    folder = ".",
    browseOffset = 0,
    initialized = false,
    initializing = false;
  let scan = null,
    operation = null,
    pollTimer = null,
    dirty = false,
    selected = new Set(),
    busy = false;
  let playerFiles = [],
    currentFile = null,
    offset = 0,
    absoluteTime = 0,
    totalTime = 0,
    pauseTimer = null,
    internalStop = false;
  let infoGeneration = 0,
    seekTimer = null;

  async function api(path, options = {}) {
    const response = await fetch(`/dashboard/api/media/${path}`, {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.timeout(20000),
      ...options,
      headers: {
        "Content-Type": "application/json",
        "X-Dashboard-Request": "1",
        ...options.headers,
      },
    });
    if (response.status === 401) {
      closePlayer();
      location.replace("/dashboard/login");
      throw new Error("Your session has expired.");
    }
    if (!response.ok) {
      const message = (await response.text()).trim();
      const error = new Error(message || "Could not complete the operation.");
      error.status = response.status;
      throw error;
    }
    return response.status === 204 ? null : response.json();
  }
  function fail(message) {
    $("media-error").textContent = message;
    $("media-error").hidden = false;
  }
  function clearError() {
    $("media-error").hidden = true;
  }
  function csv(id) {
    return $(id)
      .value.split(",")
      .map((v) => v.trim())
      .filter(Boolean);
  }
  function filters() {
    return {
      root,
      path: $("media-folder").value.trim() || ".",
      recursive: $("media-recursive").checked,
      contains: $("media-contains").value,
      exclude: $("media-exclude").value,
      minBytes: Math.round(Number($("media-min-size").value) * 1024 ** 3),
      maxBytes: Math.round(Number($("media-max-size").value) * 1024 ** 3),
      minWidth: Number($("media-width").value),
      minBitrateMbps: Number($("media-bitrate").value),
      includeCodecs: csv("media-include-codecs"),
      excludeCodecs: csv("media-exclude-codecs"),
      candidateLimit: Number($("media-candidate-limit").value),
      onlyEligible: $("media-eligible-only").checked,
      retryFailed: $("media-retry-failed").checked,
      retryCanceled: $("media-retry-canceled").checked,
    };
  }
  function markDirty() {
    if (
      scan ||
      (operation?.kind === "scan" && operation?.state === "running")
    ) {
      dirty = true;
      selected.clear();
      renderSelection();
    }
  }
  function rootState() {
    const source = root === "source";
    $("media-eligible-only").disabled = !source;
    if (!source) $("media-eligible-only").checked = false;
    $("media-retry-failed").disabled = !source;
    $("media-retry-canceled").disabled = !source;
  }
  async function init() {
    if (initialized || initializing) return;
    initializing = true;
    try {
      const response = await api("roots");
      roots = response.roots;
      presets = response.presets;
      $("media-root").innerHTML = roots
        .map(
          (r) =>
            `<option value="${esc(r.id)}" ${r.available ? "" : "disabled"}>${esc(r.name)}${r.queueable ? "" : " · playback"}${r.available ? "" : " · unavailable"}</option>`,
        )
        .join("");
      root = $("media-root").value || "source";
      initialized = true;
      rootState();
      await browse(".");
    } catch (e) {
      fail(e.message);
    } finally {
      initializing = false;
    }
  }
  function stateLabel(f) {
    if (f.directory) return '<span class="muted">Folder</span>';
    if (f.job)
      return `<span class="badge ${esc(f.job.state)}">${esc(f.job.role === "output" ? "Encoded" : names[f.job.state] || f.job.state)}</span>`;
    return `<span class="badge ${f.eligible ? "completed" : "offline"}">${f.root === "source" ? "Available" : "Output"}</span>`;
  }
  async function browse(next = folder, page = 0) {
    clearError();
    folder = next || ".";
    browseOffset = page;
    $("media-folder").value = folder;
    $("media-files").innerHTML =
      '<div class="empty-state">Reading folder…</div>';
    try {
      const result = await api(
        `browse?root=${encodeURIComponent(root)}&path=${encodeURIComponent(folder)}&offset=${page}`,
      );
      folder = result.path;
      const segments = folder === "." ? [] : folder.split("/");
      let path = "";
      $("media-breadcrumb").innerHTML =
        '<button type="button" data-media-folder=".">Home</button>' +
        segments
          .map((part) => {
            path += (path ? "/" : "") + part;
            return `<span>/</span><button type="button" data-media-folder="${esc(path)}">${esc(part)}</button>`;
          })
          .join("");
      $("media-file-count").textContent = `${result.total} items`;
      $("media-files").innerHTML = result.files.length
        ? `<table><thead><tr><th>Name</th><th>Size</th><th>Status</th><th></th></tr></thead><tbody>${result.files.map((f) => `<tr><td class="media-name-cell">${f.directory ? `<button class="media-folder-link" data-media-folder="${esc(f.path)}" type="button"><span aria-hidden="true">▱</span>${esc(f.name)}</button>` : `<span title="${esc(f.path)}">${esc(f.name)}</span>`}</td><td>${f.directory ? "—" : bytes(f.size)}</td><td>${stateLabel(f)}</td><td>${f.directory ? "" : `<button class="button" data-media-play="${esc(f.id)}" type="button">▷ View</button>`}</td></tr>`).join("")}</tbody></table>`
        : '<div class="empty-state"><strong>No videos in this folder</strong><p>Open a subfolder or choose another library.</p></div>';
      $("media-pagination").innerHTML =
        `<span>${result.total ? result.offset + 1 : 0}–${Math.min(result.offset + 100, result.total)} of ${result.total}</span><div><button class="button" data-media-page="${Math.max(0, result.offset - 100)}" ${result.offset === 0 ? "disabled" : ""} type="button">Previous</button><button class="button" data-media-page="${result.offset + 100}" ${result.offset + 100 >= result.total ? "disabled" : ""} type="button">Next</button></div>`;
    } catch (e) {
      $("media-files").innerHTML =
        '<div class="empty-state">Could not read the folder.</div>';
      fail(e.message);
    }
  }
  function applyPreset() {
    const p = presets[$("media-preset").value];
    if (!p) return;
    $("media-contains").value = p.contains || "";
    $("media-exclude").value = p.exclude || "";
    $("media-width").value = p.minWidth || 0;
    $("media-bitrate").value = p.minBitrateMbps || 0;
    $("media-min-size").value = 0;
    $("media-max-size").value = 0;
    $("media-include-codecs").value = (p.includeCodecs || []).join(", ");
    $("media-exclude-codecs").value = (p.excludeCodecs || []).join(", ");
    $("media-recursive").checked = p.recursive;
    $("media-eligible-only").checked = root === "source";
    $("media-candidate-limit").value = p.candidateLimit || 1000;
    markDirty();
  }
  function setBusy(value) {
    busy = value;
    $("media-scan").disabled = value;
    $("media-queue").disabled =
      value || dirty || !selected.size || root !== "source";
  }
  async function startScan(event) {
    event.preventDefault();
    clearError();
    try {
      if (operation?.state === "running") {
        await api(`operations/${operation.id}`, { method: "DELETE" });
      }
      clearTimeout(pollTimer);
      scan = null;
      selected.clear();
      dirty = false;
      $("media-review-panel").hidden = true;
      setBusy(true);
      operation = await api("scans", {
        method: "POST",
        body: JSON.stringify(filters()),
      });
      renderOperation();
      poll();
    } catch (e) {
      setBusy(false);
      fail(e.message);
    }
  }
  function renderOperation() {
    const op = operation;
    if (!op) return;
    $("media-operation-panel").hidden = false;
    $("media-operation-kind").textContent =
      op.kind === "scan" ? "CANDIDATE SCAN" : "JOB INTAKE";
    const running = op.state === "running";
    $("media-cancel").hidden = !running;
    $("media-operation-title").textContent =
      op.kind === "scan"
        ? running
          ? "Finding your next jobs"
          : op.state === "completed"
            ? `${op.matched} matching videos`
            : op.state === "canceled"
              ? "Scan stopped"
              : "Could not complete the scan"
        : running
          ? `${op.scheduled} jobs queued`
          : `Intake ${op.state === "canceled" ? "stopped" : op.state === "failed" ? "interrupted" : "completed"} · ${op.scheduled} jobs`;
    $("media-operation-detail").textContent =
      op.kind === "scan"
        ? op.stage === "discovering"
          ? `Exploring folders · ${op.discovered} videos discovered`
          : `${op.examined} of ${op.candidates} scanned · ${op.matched} matches · ${op.failed} not scanned`
        : `${op.examined} candidates reviewed · target ${op.target} successes · ${op.skipped} skipped · ${op.failed} failed`;
    const percent =
      op.kind === "scan"
        ? op.candidates
          ? (op.examined / op.candidates) * 100
          : 0
        : op.target
          ? (op.scheduled / op.target) * 100
          : 0;
    $("media-progress").style.width = `${Math.max(0, Math.min(100, percent))}%`;
    $("media-truncated").hidden = !op.truncated;
    $("media-truncated").textContent =
      "Preview shows the largest candidates within the scan limit. If the 100,000 discovered-video cap is reached, narrow the folder for a complete selection.";
    const rows = op.results || [];
    $("media-operation-results").innerHTML = rows.length
      ? `<details class="media-result-details" ${op.kind === "queue" ? "open" : ""}><summary>Results by file (${rows.length})</summary><div class="media-result-list">${rows
          .slice(-200)
          .map(
            (r) =>
              `<div><span class="badge ${r.state === "scheduled" ? "completed" : r.state === "failed" ? "failed" : "offline"}">${r.state === "scheduled" ? "Queued" : r.state === "failed" ? "Failed" : "Skipped"}</span><div><strong title="${esc(r.path)}">${esc(r.path)}</strong>${r.message ? `<p>${esc(r.message)}</p>` : ""}</div></div>`,
          )
          .join(
            "",
          )}</div>${rows.length > 200 ? '<p class="media-help">Showing the 200 most recent results.</p>' : ""}</details>`
      : "";
    if (op.error) fail(op.error);
  }
  async function poll() {
    clearTimeout(pollTimer);
    if (!operation) return;
    try {
      operation = await api(`operations/${operation.id}`);
      renderOperation();
      if (operation.state === "running") {
        pollTimer = setTimeout(poll, 1500);
      } else {
        setBusy(false);
        if (operation.kind === "scan" && operation.state === "completed") {
          scan = operation;
          renderSelection();
        }
        if (operation.kind === "queue") {
          selected.clear();
          dirty = true;
          renderSelection();
          if (location.hash === "#library") await browse(folder, browseOffset);
        }
      }
    } catch (e) {
      if (e.status === 404) operation = null;
      setBusy(operation?.state === "running");
      fail(e.message);
      if (operation?.state === "running") pollTimer = setTimeout(poll, 5000);
    }
  }
  function renderSelection() {
    if (!scan) return;
    $("media-review-panel").hidden = false;
    $("media-dirty").hidden = !dirty;
    $("media-review-count").textContent =
      `${scan.files.length} matches · largest first · ${scan.files.filter((f) => f.eligible).length} eligible for queuing`;
    $("media-selection").innerHTML = scan.files.length
      ? `<table><thead><tr><th>Select</th><th>Video</th><th>Size</th><th>Dimensions / codec</th><th>Total bitrate</th><th></th></tr></thead><tbody>${scan.files.map((f) => `<tr><td><input type="checkbox" data-media-select="${esc(f.id)}" ${selected.has(f.id) ? "checked" : ""} ${!f.eligible || dirty || busy ? "disabled" : ""} aria-label="Select ${esc(f.name)}" /></td><td class="media-name-cell"><strong title="${esc(f.path)}">${esc(f.name)}</strong><small>${esc(f.path)}</small>${f.job ? stateLabel(f) : ""}${!f.eligible ? `<p class="media-help">${esc(f.reason)}</p>` : ""}</td><td>${bytes(f.size)}</td><td>${f.metadata ? `${f.metadata.width} × ${f.metadata.height}<br>${esc(f.metadata.codec.toUpperCase())}` : "—"}</td><td>${f.metadata ? `${num.format(f.metadata.bitrateMbps)} Mb/s<br>${clock(f.metadata.durationSeconds)}` : "—"}</td><td><button class="button" data-media-play="${esc(f.id)}" type="button">▷ View</button></td></tr>`).join("")}</tbody></table>`
      : '<div class="empty-state"><strong>No matching videos</strong><p>Try another folder, lower the thresholds or check excluded codecs.</p></div>';
    const chosen = scan.files.filter((f) => selected.has(f.id));
    $("media-selected-count").textContent = `${chosen.length} selected`;
    $("media-selected-size").textContent =
      `${bytes(chosen.reduce((n, f) => n + f.size, 0))} source size`;
    $("media-select-all").disabled =
      dirty || busy || !scan.files.some((f) => f.eligible);
    $("media-queue").disabled =
      busy || dirty || !chosen.length || scan.filters.root !== "source";
  }
  async function queueSelection() {
    if (!scan || dirty || busy || !selected.size) return;
    const target = Number($("media-target").value);
    if (!Number.isInteger(target) || target < 1 || target > 5000) {
      fail("The target must be between 1 and 5000 jobs.");
      return;
    }
    clearError();
    setBusy(true);
    renderSelection();
    try {
      operation = await api("batches", {
        method: "POST",
        body: JSON.stringify({ scanId: scan.id, ids: [...selected], target }),
      });
      renderOperation();
      poll();
    } catch (e) {
      setBusy(false);
      renderSelection();
      fail(e.message);
    }
  }
  function playerError(message) {
    $("media-player-error").textContent = message;
    $("media-player-error").hidden = false;
  }
  function currentTime() {
    const v = $("media-video");
    return $("media-player-mode").value === "original"
      ? Number.isFinite(v.currentTime)
        ? v.currentTime
        : absoluteTime
      : offset + (Number.isFinite(v.currentTime) ? v.currentTime : 0);
  }
  function stopVideo() {
    internalStop = true;
    clearTimeout(pauseTimer);
    clearTimeout(seekTimer);
    const v = $("media-video");
    v.pause();
    v.removeAttribute("src");
    v.load();
    $("media-player-toggle").textContent = "Play";
    internalStop = false;
  }
  function closePlayer() {
    infoGeneration++;
    stopVideo();
    if ($("media-player").open) $("media-player").close();
    currentFile = null;
    playerFiles = [];
  }
  function videoURL() {
    return $("media-player-mode").value === "original"
      ? `/dashboard/api/media/file/${currentFile.id}`
      : `/dashboard/api/media/preview/${currentFile.id}?start=${offset.toFixed(3)}`;
  }
  function assignSource(at = 0) {
    stopVideo();
    absoluteTime = Math.max(0, at);
    offset = $("media-player-mode").value === "original" ? 0 : absoluteTime;
    const v = $("media-video");
    v.src = videoURL();
    if ($("media-player-mode").value === "original" && at > 0)
      v.addEventListener(
        "loadedmetadata",
        () => {
          v.currentTime = at;
        },
        { once: true },
      );
    $("media-player-error").hidden = true;
    updateTime();
  }
  function updateTime() {
    if (!currentFile) return;
    const v = $("media-video");
    if (v.getAttribute("src")) absoluteTime = currentTime();
    $("media-player-current").textContent = clock(absoluteTime);
    if (document.activeElement !== $("media-player-seek"))
      $("media-player-seek").value = Math.min(totalTime, absoluteTime);
  }
  async function chooseAsset(id) {
    const generation = ++infoGeneration;
    stopVideo();
    currentFile = null;
    $("media-player-toggle").disabled = true;
    $("media-player-error").hidden = true;
    $("media-player-info").textContent = "Reading video information…";
    try {
      const f = await api(`info/${id}`);
      if (generation !== infoGeneration) return;
      currentFile = f;
      $("media-player-toggle").disabled = false;
      totalTime = f.metadata?.durationSeconds || 0;
      $("media-player-title").textContent = f.name;
      $("media-player-duration").textContent = totalTime
        ? clock(totalTime)
        : "—";
      $("media-player-seek").max = Math.max(1, totalTime);
      $("media-player-seek").disabled = !totalTime;
      $("media-player-info").textContent =
        `${bytes(f.size)} · ${f.metadata?.codec?.toUpperCase() || "Video"} · ${f.metadata?.width || "?"} × ${f.metadata?.height || "?"} · ${f.metadata ? num.format(f.metadata.bitrateMbps) + " Mb/s total" : ""}${playerFiles.some((a) => a.job?.role === "output") && !playerFiles.some((a) => a.job?.role === "source") ? " · Source unavailable" : ""}`;
      assignSource(0);
    } catch (e) {
      if (generation === infoGeneration) playerError(e.message);
    }
  }
  function showPlayer(files) {
    closePlayer();
    playerFiles = files;
    $("media-player-title").textContent = "Video player";
    $("media-player-duration").textContent = "—";
    $("media-player-current").textContent = "00:00";
    $("media-player-info").textContent = "";
    $("media-player-seek").disabled = true;
    $("media-player-toggle").disabled = true;
    $("media-player-asset").innerHTML = "";
    $("media-player-error").hidden = true;
    if (!files.length) {
      $("media-player").showModal();
      playerError(
        "No source or output file is available for this job.",
      );
      return;
    }
    $("media-player-asset").innerHTML = files
      .map(
        (f) =>
          `<option value="${esc(f.id)}">${f.job?.role === "output" ? "Encoded" : f.job?.role === "source" ? "Source" : f.root === "source" ? "Source file" : "Available file"}</option>`,
      )
      .join("");
    const preferred = files.find((f) => f.job?.role === "output") || files[0];
    $("media-player-asset").value = preferred.id;
    $("media-player-mode").value = "compatible";
    $("media-player").showModal();
    chooseAsset(preferred.id);
  }
  async function openFile(id) {
    showPlayer([{ id, root, job: null }]);
  }
  async function openJob(id) {
    try {
      const response = await api(`jobs/${id}`);
      showPlayer(response.files);
    } catch (e) {
      $("media-player").showModal();
      playerError(e.message);
    }
  }
  async function togglePlay() {
    if (!currentFile) return;
    const v = $("media-video");
    if (!v.paused) {
      v.pause();
      return;
    }
    if (!v.getAttribute("src")) assignSource(absoluteTime);
    try {
      await v.play();
    } catch (e) {
      if (e.name !== "AbortError")
        playerError(
          "Could not start playback. Try again or switch to original mode.",
        );
    }
  }
  $("media-video").addEventListener("timeupdate", updateTime);
  $("media-video").addEventListener("play", () => {
    clearTimeout(pauseTimer);
    $("media-player-toggle").textContent = "Pause";
  });
  $("media-video").addEventListener("pause", () => {
    if (internalStop || !currentFile) return;
    absoluteTime = currentTime();
    $("media-player-toggle").textContent = "Resume";
    if ($("media-player-mode").value === "compatible")
      pauseTimer = setTimeout(() => {
        if ($("media-video").paused) {
          const at = absoluteTime;
          stopVideo();
          absoluteTime = at;
          offset = at;
          updateTime();
          $("media-player-toggle").textContent = "Resume";
        }
      }, 2000);
  });
  $("media-video").addEventListener("error", () => {
    if (!internalStop && $("media-video").getAttribute("src"))
      playerError(
        $("media-player-mode").value === "original"
          ? "Your browser does not support this file. Switch to compatible mode."
          : "Could not start compatible playback. Other videos may be using the available slots; retry or use original mode.",
      );
  });
  $("media-player-toggle").addEventListener("click", togglePlay);
  $("media-player-close").addEventListener("click", closePlayer);
  $("media-player").addEventListener("cancel", () => {
    stopVideo();
    currentFile = null;
    infoGeneration++;
  });
  $("media-player").addEventListener("close", () => {
    if (!$("media-player").open) {
      stopVideo();
      currentFile = null;
      infoGeneration++;
    }
  });
  $("media-player-asset").addEventListener("change", () =>
    chooseAsset($("media-player-asset").value),
  );
  $("media-player-mode").addEventListener("change", () => {
    if (!currentFile) return;
    const at = absoluteTime;
    assignSource(at);
  });
  $("media-player-seek").addEventListener("input", () => {
    $("media-player-current").textContent = clock(
      Number($("media-player-seek").value),
    );
  });
  $("media-player-seek").addEventListener("change", () => {
    if (!currentFile) return;
    const at = Number($("media-player-seek").value);
    if ($("media-player-mode").value === "original") {
      $("media-video").currentTime = at;
      absoluteTime = at;
    } else {
      clearTimeout(seekTimer);
      seekTimer = setTimeout(() => {
        if (!currentFile) return;
        assignSource(at);
        togglePlay();
      }, 150);
    }
  });
  $("media-root").addEventListener("change", () => {
    root = $("media-root").value;
    markDirty();
    rootState();
    browse(".");
  });
  $("media-open-folder").addEventListener("click", () => {
    markDirty();
    browse($("media-folder").value.trim() || ".");
  });
  $("media-preset").addEventListener("change", applyPreset);
  $("media-filters").addEventListener("input", markDirty);
  $("media-filters").addEventListener("submit", startScan);
  $("media-select-all").addEventListener("click", () => {
    if (!scan || dirty || busy) return;
    const eligible = scan.files.filter((f) => f.eligible);
    const all = eligible.every((f) => selected.has(f.id));
    selected = all ? new Set() : new Set(eligible.map((f) => f.id));
    renderSelection();
  });
  $("media-queue").addEventListener("click", queueSelection);
  $("media-cancel").addEventListener("click", async () => {
    if (!operation) return;
    try {
      await api(`operations/${operation.id}`, { method: "DELETE" });
      poll();
    } catch (e) {
      fail(e.message);
    }
  });
  document.addEventListener("click", (event) => {
    const play = event.target.closest("[data-media-play]");
    if (play) {
      openFile(play.dataset.mediaPlay);
      return;
    }
    const job = event.target.closest("[data-job-media]");
    if (job) {
      openJob(job.dataset.jobMedia);
      return;
    }
    const nav = event.target.closest("[data-media-folder]");
    if (nav) {
      markDirty();
      browse(nav.dataset.mediaFolder);
      return;
    }
    const page = event.target.closest("[data-media-page]");
    if (page) browse(folder, Number(page.dataset.mediaPage));
  });
  $("media-selection").addEventListener("change", (event) => {
    const id = event.target.dataset.mediaSelect;
    if (!id || dirty || busy) return;
    if (event.target.checked) selected.add(id);
    else selected.delete(id);
    renderSelection();
  });
  window.addEventListener("hashchange", () => {
    if (location.hash === "#library") init();
  });
  window.addEventListener("pagehide", () => {
    clearTimeout(pollTimer);
    stopVideo();
  });
  document.addEventListener("visibilitychange", () => {
    if (document.hidden && $("media-player").open) {
      $("media-video").pause();
    }
  });
  if (location.hash === "#library") init();
})();
