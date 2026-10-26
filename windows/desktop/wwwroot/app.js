'use strict';
const $ = id => document.getElementById(id);
const pending = new Map();
let sequence = 0, current = null, dirty = false, shownSetup = false, lastTarget = 2, polling = false;
function request(command, data = {}) {
  return new Promise((resolve, reject) => {
    if (!window.chrome?.webview) return reject(new Error('La aplicación de escritorio no está disponible.'));
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('El servicio no responde.')); }, 14000);
    pending.set(id, {resolve, reject, timer});
    window.chrome.webview.postMessage({id, command, ...data});
  });
}
if (window.chrome?.webview) window.chrome.webview.addEventListener('message', event => {
  const {id, ok, payload} = event.data, call = pending.get(id);
  if (!call) return;
  pending.delete(id); clearTimeout(call.timer);
  if (!ok) call.reject(new Error(typeof payload === 'string' ? payload : 'No se pudo conectar con el servicio.'));
  else if (payload.error) call.reject(new Error(payload.error));
  else call.resolve(payload.status || payload);
});
function page(name) {
  document.querySelectorAll('.page').forEach(el => el.classList.toggle('active-page', el.id === name));
  document.querySelectorAll('.nav').forEach(el => el.classList.toggle('active', el.dataset.page === name));
  $('crumb').textContent = name === 'settings' ? 'CONFIGURACIÓN' : 'PANEL';
}
document.querySelectorAll('[data-page]').forEach(el => el.addEventListener('click', () => page(el.dataset.page)));
document.querySelectorAll('[data-goto]').forEach(el => el.addEventListener('click', () => page(el.dataset.goto)));
function toast(message) { const el = $('toast'); el.textContent = message; el.classList.add('show'); clearTimeout(toast.timer); toast.timer = setTimeout(() => el.classList.remove('show'), 4500); }
function escapeHTML(value) { return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function empty(title, detail, small = false) { return `<div class="empty ${small ? 'small' : ''}"><div class="empty-icon">◌</div><strong>${escapeHTML(title)}</strong><span>${escapeHTML(detail)}</span></div>`; }
function duration(seconds) { if (!seconds || seconds < 0) return ''; const m = Math.ceil(seconds / 60); return m >= 60 ? `${Math.floor(m / 60)} h ${m % 60} min restantes` : `${m} min restantes`; }
function jobLabel(job) {
  if (job.state === 'queued') return job.downloaded ? (job.phase === 'FFMPEG' ? ['Listo para encode','ready'] : ['Descargado · en cola','ready']) : ['En cola',''];
  return ({FFMPEG:['Codificando',''], Download:['Descargando','download'], PGS:['Subtítulos',''], MKVExtract:['Preparando',''], Upload:['Subiendo','download']})[job.phase] || ['Procesando',''];
}
function render(status) {
  current = status;
  $('connection').className = 'connection online'; $('connection').innerHTML = '<i></i> Servicio conectado';
  const worker = status.worker, jobs = worker?.jobs || [], progress = worker?.progress || [];
  if (worker?.maxActiveJobs > 0) lastTarget = worker.maxActiveJobs;
  $('stat-encoding').textContent = worker ? worker.activeEncodes : '—';
  $('stat-encoding-caption').textContent = worker ? `de ${worker.maxEncodes} encodes simultáneos` : 'Encodes activos';
  $('stat-ready').textContent = worker ? jobs.filter(j => j.downloaded && j.state === 'queued' && j.phase === 'FFMPEG').length : '—';
  $('stat-downloading').textContent = worker ? jobs.filter(j => j.phase === 'Download' && j.state === 'running').length : '—';
  $('local-count').textContent = jobs.length;
  $('job-list').innerHTML = jobs.length ? jobs.map(j => {
    const p = progress.find(x => x.jobId === j.jobId && x.phase === j.phase);
    const [label, badge] = jobLabel(j); const name = j.name || p?.name || `Trabajo ${j.jobId.slice(0,8)}`;
    const pct = p ? Math.min(100, Math.max(0, Number(p.percent) || 0)) : 0;
    return `<article class="job"><div class="job-head"><div class="file-icon">▶</div><div class="job-info"><div class="job-title" title="${escapeHTML(name)}">${escapeHTML(name)}</div><div class="job-meta">${escapeHTML(duration(p?.etaSeconds) || (j.downloaded ? 'Archivo descargado' : 'Preparando archivo'))}</div></div><span class="badge ${badge}">${label}</span></div>${p ? `<div class="job-progress"><div class="track"><div class="fill" style="width:${pct}%"></div></div><strong>${pct.toFixed(0)}%</strong></div>` : ''}</article>`;
  }).join('') : empty(status.configured ? 'Todo tranquilo por aquí' : 'Prepara tu worker', status.configured ? 'Los nuevos trabajos aparecerán aquí automáticamente.' : 'Configura la conexión para empezar.');
  const queue = status.queue;
  $('stat-queue').textContent = queue ? queue.total : '—'; $('queue-count').textContent = queue ? queue.total : '—';
  $('stat-queue-caption').textContent = queue ? 'Pendientes de asignar' : 'No disponible';
  $('server-queue').innerHTML = queue ? (queue.jobs?.length ? queue.jobs.map((j, i) => `<div class="queue-item"><span class="queue-num">${String(i+1).padStart(2,'0')}</span><div class="queue-info"><div class="queue-name" title="${escapeHTML(j.name)}">${escapeHTML(j.name)}</div><div class="queue-meta">Pendiente de asignación</div></div><span class="badge ready">En cola</span></div>`).join('') : empty('Cola vacía','No hay trabajos pendientes de asignar.',true)) : empty('Cola no disponible',status.queueError === 'unsupported' ? 'Actualiza el servidor para consultar la cola global.' : 'La cola aparecerá cuando el servidor esté disponible.',true);
  const notice = !status.configured ? 'Configura la conexión para poner en marcha tu worker.' : status.pending ? 'Los cambios están pendientes: se aplicarán cuando terminen los trabajos actuales.' : status.message || '';
  $('notice').classList.toggle('hidden', !notice); $('notice-text').textContent = notice; $('notice-start').classList.add('hidden'); $('notice-settings').classList.remove('hidden');
  $('toggle-pause').disabled = !worker || status.pending;
  $('toggle-pause').textContent = worker?.maxActiveJobs === 0 ? '▶  Reanudar' : 'Ⅱ  Pausar';
  if (!dirty && status.settings) { const s = status.settings; $('domain').value = s.domain; $('name').value = s.name; $('codec').value = s.codec; $('jobs').value = s.maxJobs; $('schedule').value = (s.schedule || []).join('\n'); $('token-note').textContent = s.tokenSet ? 'Token guardado. Déjalo vacío para conservarlo.' : 'El token se guarda de forma protegida en este equipo.'; }
  if (!status.configured && !shownSetup) { shownSetup = true; page('settings'); request('show-window').catch(() => {}); }
}
async function refresh() {
  if (polling) return; polling = true;
  try { render(await request('status')); }
  catch (_) { $('connection').className = 'connection error'; $('connection').innerHTML = '<i></i> Servicio desconectado'; $('notice').classList.remove('hidden'); $('notice-text').textContent = 'El servicio de Windows no está disponible.'; $('notice-start').classList.remove('hidden'); $('notice-settings').classList.add('hidden'); $('toggle-pause').disabled = true; }
  finally { polling = false; }
}
$('refresh').addEventListener('click', refresh);
$('notice-start').addEventListener('click', async () => { try { await request('start-service'); toast('Iniciando servicio…'); setTimeout(refresh, 2000); } catch (err) { toast(err.message); } });
$('settings-form').addEventListener('input', () => { dirty = true; });
$('settings-form').addEventListener('submit', async e => {
  e.preventDefault(); $('save').disabled = true;
  const settings = {domain:$('domain').value.trim(), token:$('token').value, name:$('name').value.trim(), codec:$('codec').value, maxJobs:Number($('jobs').value), schedule:$('schedule').value.split(/\r?\n/).map(x => x.trim()).filter(Boolean)};
  try { const status = await request('configure', {settings}); $('token').value = ''; dirty = false; render(status); toast(status.pending ? 'Cambios guardados; se aplicarán al terminar los trabajos.' : 'Configuración guardada.'); page('overview'); }
  catch (err) { toast(err.message); } finally { $('save').disabled = false; }
});
$('toggle-pause').addEventListener('click', async () => { try { const next = current?.worker?.maxActiveJobs === 0 ? lastTarget : 0; render(await request('set-max-jobs', {maxJobs:next})); toast(next ? 'Worker reanudado.' : 'Nuevas asignaciones pausadas.'); } catch (err) { toast(err.message); } });
$('show-token').addEventListener('click', () => { $('token').type = $('token').type === 'password' ? 'text' : 'password'; });
for (const [id, delta] of [['less',-1],['more',1]]) $(id).addEventListener('click', () => { $('jobs').value = Math.min(64,Math.max(0,(Number($('jobs').value)||0)+delta)); dirty = true; });
refresh(); setInterval(refresh, 2000);
