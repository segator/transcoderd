"use strict";
(() => {
  const format = new Intl.NumberFormat("en-US", {maximumFractionDigits: 1});
  const esc = (value) => String(value ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[c]);
  const finite = value => typeof value === "number" && Number.isFinite(value);
  const percent = value => finite(value) && value >= 0 && value <= 100 ? value : null;
  const pct = value => percent(value) === null ? "—" : `${format.format(value)}%`;
  const quantity = (value, suffix) => finite(value) && value >= 0 ? `${format.format(value)} ${suffix}` : "—";
  const bytes = value => {
    if (!finite(value) || value < 0) return "—";
    const units = ["B","KiB","MiB","GiB","TiB"];
    const i = value ? Math.min(4, Math.floor(Math.log(value) / Math.log(1024))) : 0;
    return `${format.format(value / 1024 ** i)} ${units[i]}`;
  };
  const memoryPercent = memory => memory && finite(memory.totalBytes) && memory.totalBytes > 0 && finite(memory.usedBytes) && memory.usedBytes >= 0 && memory.usedBytes <= memory.totalBytes ? 100 * memory.usedBytes / memory.totalBytes : null;
  const gpuID = (gpu, index) => gpu.uuid || gpu.busId || `device-${index}`;
  const link = name => `#workers?worker=${encodeURIComponent(name)}`;
  function state(worker, now = Date.now()) {
    const resource = worker.runtime?.resources;
    if (!resource) return {resource:null, live:false, label:"No hardware telemetry", detail:"Update this worker to report hardware metrics."};
    const at = Date.parse(resource.sampledAt);
    if (!Number.isFinite(at)) return {resource, live:false, label:"Invalid sample time", detail:"Waiting for a valid resource sample."};
    if (at > now + 30000) return {resource, at, live:false, label:"Sample clock mismatch", detail:"The worker clock is ahead of the server; readings are last reported."};
    const live = worker.online && now - at <= 20000;
    return {resource, at, live, label:live ? "Live resource sample" : "Last reported · stale", detail:`Sampled ${new Date(at).toLocaleString("en-US")}`};
  }
  function busiest(gpus, key) {
    const values = (gpus || []).map(g => percent(g[key])).filter(v => v !== null);
    return values.length ? Math.max(...values) : null;
  }
  function profile(worker) {
    if (!worker.runtime?.managedProfile) return {cpu:true,gpu:true};
    const gpu = worker.runtime.encoder === "av1_nvenc" || worker.runtime.kind === "gpu";
    return {cpu:!gpu,gpu};
  }
  function summary(worker, now) {
    const s = state(worker, now), r = s.resource, p = profile(worker);
    const cpu = r?.cpu?.scope === "processor-group" ? "CPU group" : "CPU";
    const gpu = (r?.gpus?.length || 0) > 1 ? "GPU max" : "GPU";
    return `<div class="resource-summary ${s.live ? "" : "resource-stale"}" title="${esc(s.label + '. ' + s.detail)}">${p.cpu ? `<span>${cpu} <b>${s.live ? pct(r?.cpu?.percent) : "—"}</b></span>` : ""}${p.gpu ? `<span title="Highest reported device utilization">${gpu} <b>${s.live ? pct(busiest(r?.gpus,"percent")) : "—"}</b></span><span title="Highest reported NVIDIA video encoder utilization">NVENC <b>${s.live ? pct(busiest(r?.gpus,"encoderPercent")) : "—"}</b></span>` : ""}<span>RAM <b>${s.live ? pct(memoryPercent(r?.memory)) : "—"}</b></span>${!s.live ? `<small>${esc(s.label)}</small>` : ""}</div>`;
  }
  class History {
    constructor() { this.workers = new Map(); }
    observe(workers, now = Date.now()) {
      const present = new Set(workers.map(w => w.name));
      for (const name of this.workers.keys()) if (!present.has(name)) this.workers.delete(name);
      for (const worker of workers) {
        const s = state(worker, now), r = s.resource;
        let points = (this.workers.get(worker.name) || []).filter(p => p.at >= now - 600000);
        if (r && finite(s.at) && s.at <= now + 30000 && s.at >= now - 600000) {
          const last = points.at(-1);
          if (last && s.at < last.at) points = [];
          if (!points.length || points.at(-1).at !== s.at) {
            points.push({at:s.at, cpu:percent(r.cpu?.percent), memory:memoryPercent(r.memory), gpus:(r.gpus || []).map((g,i) => ({id:gpuID(g,i), gpu:percent(g.percent), encoder:percent(g.encoderPercent)}))});
          }
        }
        this.workers.set(worker.name, points.slice(-120));
      }
    }
    get(name) { return (this.workers.get(name) || []).slice(); }
  }
  function chart(points, series, title) {
    if (!points.some(p => series.some(s => percent(s.value(p)) !== null))) return '<div class="resource-empty">Waiting for resource samples</div>';
    const width=600, height=180, left=30, right=12, top=12, bottom=28;
    const start=points[0].at, span=Math.max(1,points.at(-1).at-start);
    const x=p=>points.length === 1 ? (width+left-right)/2 : left+(p.at-start)*(width-left-right)/span;
    const y=v=>top+(100-v)*(height-top-bottom)/100;
    const paths=series.map((s,i)=>{
      let pen=false, d="";
      const dots=[];
      for (const point of points) {
        const value=percent(s.value(point));
        if (value===null) { pen=false; continue; }
        d+=`${pen ? "L" : "M"}${x(point).toFixed(1)},${y(value).toFixed(1)} `; pen=true;
        dots.push(`<circle cx="${x(point).toFixed(1)}" cy="${y(value).toFixed(1)}" r="2" class="resource-line-${i}"><title>${esc(new Date(point.at).toLocaleTimeString("en-US"))}: ${esc(s.name)} ${pct(value)}</title></circle>`);
      }
      return `<path d="${d}" class="resource-path resource-line-${i}"/>${dots.join("")}`;
    }).join("");
    return `<div class="resource-legend">${series.map((s,i)=>`<span><i class="resource-line-${i}"></i>${esc(s.name)}</span>`).join("")}</div><svg class="resource-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${esc(title)}">${[0,50,100].map(v=>`<line class="chart-gridline" x1="${left}" x2="${width-right}" y1="${y(v)}" y2="${y(v)}"/><text class="chart-label" x="${left-5}" y="${y(v)+4}" text-anchor="end">${v}</text>`).join("")}${paths}<text class="chart-label" x="${left}" y="${height-6}">${esc(new Date(start).toLocaleTimeString("en-US"))}</text><text class="chart-label" x="${width-right}" y="${height-6}" text-anchor="end">${esc(new Date(points.at(-1).at).toLocaleTimeString("en-US"))}</text></svg>`;
  }
  const stat = (label, value, note = "") => `<div class="resource-stat"><small>${esc(label)}</small><strong>${esc(value)}</strong>${note ? `<span>${esc(note)}</span>` : ""}</div>`;
  function details(worker, history, now) {
    const s=state(worker,now), r=s.resource, points=history.get(worker.name), p=profile(worker);
    if (!r) return `<section class="panel resource-panel"><h2>Hardware telemetry unavailable</h2><p class="muted">${esc(s.detail)} Existing capacity controls remain available below.</p></section>`;
    const cpuLabel=r.cpu?.scope === "processor-group" ? "Processor-group CPU" : "Host CPU";
    const cpuNote=!r.cpu ? "CPU metrics unavailable" : r.cpu.percent == null ? "Waiting for a second CPU sample" : `${r.cpu?.cores || "—"} logical processors`;
    const memory=memoryPercent(r.memory);
    const gpuCards=(p.gpu ? r.gpus || [] : []).map((g,i)=>{
      const id=gpuID(g,i), gpuMemory=memoryPercent({usedBytes:g.memoryUsedBytes,totalBytes:g.memoryTotalBytes});
      return `<section class="panel resource-panel gpu-resource ${s.live ? "" : "resource-last-reported"}"><div class="resource-heading"><div><span class="eyebrow muted">NVIDIA DEVICE ${i+1}</span><h3>${esc(g.name || "NVIDIA GPU")}</h3><p class="resource-identity">${esc(g.uuid || g.busId || "Identity unavailable")}</p></div><span class="badge ${s.live ? "completed" : "offline"}">${s.live ? "Live" : "Last reported"}</span></div><div class="resource-stats">${stat("GPU utilization",pct(g.percent))}${stat("Video encode · NVENC",pct(g.encoderPercent))}${stat("Video decode",pct(g.decoderPercent))}${stat("Temperature",quantity(g.temperatureC,"°C"))}${stat("Board power",quantity(g.powerWatts,"W"),`Limit ${quantity(g.powerLimitWatts,"W")}`)}${stat("VRAM",`${bytes(g.memoryUsedBytes)} / ${bytes(g.memoryTotalBytes)}`,pct(gpuMemory))}</div>${gpuMemory === null ? '<p class="media-help">VRAM utilization unavailable</p>' : `<div class="progress-track resource-memory"><span data-width="${gpuMemory}"></span></div>`}${chart(points,[{name:"GPU",value:p=>p.gpus.find(g=>g.id===id)?.gpu},{name:"NVENC",value:p=>p.gpus.find(g=>g.id===id)?.encoder}],`${g.name || "GPU"} and video encoder utilization`)}<p class="media-help">Device-level readings include other processes. Driver ${esc(r.driverVersion || "unavailable")} · PCI ${esc(g.busId || "unavailable")}</p></section>`;
    }).join("");
    return `<section class="panel resource-panel ${s.live ? "" : "resource-last-reported"}"><div class="resource-heading"><div><span class="eyebrow muted">WORKER RESOURCES</span><h2>${esc(worker.name)}</h2><p>${esc(r.hostname || "Unknown host")} · ${esc(r.os)} / ${esc(r.architecture)}</p></div><span class="badge ${s.live ? "completed" : "offline"}">${esc(s.label)}</span></div><p class="media-help">${esc(s.detail)}. OS-visible CPU/memory and visible NVIDIA devices are shared with other processes and worker profiles; they are not per-job utilization.</p><div class="resource-stats">${p.cpu ? stat(cpuLabel,pct(r.cpu?.percent),cpuNote) : ""}${stat("Host RAM",r.memory ? `${bytes(r.memory.usedBytes)} / ${bytes(r.memory.totalBytes)}` : "—",pct(memory))}${stat("Recent history",`${points.length} samples`,"In this tab · up to 10 minutes")}</div>${chart(points,[...(p.cpu ? [{name:cpuLabel,value:p=>p.cpu}] : []),{name:"RAM",value:p=>p.memory}],p.cpu ? "CPU and memory utilization" : "Memory utilization")}<p class="media-help">Measured hardware usage is separate from configured encode slots. Missing sensors are shown as unavailable.</p></section>${gpuCards || (p.gpu ? '<section class="panel resource-panel"><h3>No NVIDIA telemetry</h3><p class="muted">No devices were reported, or the driver/nvidia-smi is unavailable to this worker.</p></section>' : "")}`;
  }
  const api={profile,percent,memoryPercent,state,busiest,link,summary,History,chart,details};
  if (typeof module !== "undefined" && module.exports) module.exports=api;
  else window.TranscoderResources=api;
})();
