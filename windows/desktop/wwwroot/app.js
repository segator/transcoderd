'use strict';
const $ = id => document.getElementById(id);
const pending = new Map();
let sequence = 0, current = null, dirty = false, shownSetup = false, lastJobs = {cpu:0,gpu:2}, polling = false;
function request(command, data = {}) {
  return new Promise((resolve, reject) => {
    if (!window.chrome?.webview) return reject(new Error('The desktop application is unavailable.'));
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('The service is not responding.')); }, 14000);
    pending.set(id, {resolve, reject, timer});
    window.chrome.webview.postMessage({id, command, ...data});
  });
}
if (window.chrome?.webview) window.chrome.webview.addEventListener('message', event => {
  const {id, ok, payload} = event.data, call = pending.get(id);
  if (!call) return;
  pending.delete(id); clearTimeout(call.timer);
  if (!ok) call.reject(new Error(typeof payload === 'string' ? payload : 'Could not connect to the service.'));
  else if (payload.error) call.reject(new Error(payload.error));
  else call.resolve(payload.status || payload);
});
function page(name) {
  document.querySelectorAll('.page').forEach(el => el.classList.toggle('active-page', el.id === name));
  document.querySelectorAll('.nav').forEach(el => el.classList.toggle('active', el.dataset.page === name));
  $('crumb').textContent = name === 'settings' ? 'SETTINGS' : 'DASHBOARD';
}
document.querySelectorAll('[data-page]').forEach(el => el.addEventListener('click', () => page(el.dataset.page)));
document.querySelectorAll('[data-goto]').forEach(el => el.addEventListener('click', () => page(el.dataset.goto)));
function toast(message) { const el = $('toast'); el.textContent = message; el.classList.add('show'); clearTimeout(toast.timer); toast.timer = setTimeout(() => el.classList.remove('show'), 4500); }
function escapeHTML(value) { return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function empty(title, detail, small = false) { return `<div class="empty ${small ? 'small' : ''}"><div class="empty-icon">◌</div><strong>${escapeHTML(title)}</strong><span>${escapeHTML(detail)}</span></div>`; }
function fileSize(bytes) { if (!Number.isFinite(bytes) || bytes <= 0) return ''; const units = ['B','KB','MB','GB','TB']; const power = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024))); return `${(bytes / 1024 ** power).toFixed(power ? 1 : 0)} ${units[power]}`; }
function duration(seconds) { if (!seconds || seconds < 0) return ''; const m = Math.ceil(seconds / 60); return m >= 60 ? `${Math.floor(m / 60)} h ${m % 60} min remaining` : `${m} min remaining`; }
function jobLabel(job) {
  if (job.state === 'queued') return job.downloaded ? (job.phase === 'FFMPEG' ? ['Ready to encode','ready'] : ['Downloaded · queued','ready']) : ['Queued',''];
  return ({FFMPEG:['Encoding',''], Download:['Downloading','download'], PGS:['Subtitles',''], MKVExtract:['Preparing',''], Upload:['Uploading','download']})[job.phase] || ['Processing',''];
}
function render(status) {
  current = status;
  const worker = status.worker, jobs = worker?.jobs || [], progress = worker?.progress || [];
  const admission = !status.configured ? 'Service connected' : !worker ? 'Worker starting' : worker.maxActiveJobs === 0 ? 'Paused' : worker.withinSchedule === false ? 'Outside schedule' : worker.accepting ? (worker.maxAcceptedJobs > 0 && (worker.activeJobs + worker.reservedJobs) >= worker.maxAcceptedJobs ? 'At capacity' : 'Accepting jobs') : 'Waiting for settings';
  $('connection').className = 'connection online'; $('connection').innerHTML = `<i></i> ${admission}`;
  const isPaused = worker?.maxActiveJobs === 0;
  if (status.settings) lastJobs = {cpu:status.settings.cpuJobs, gpu:status.settings.gpuJobs};
  $('stat-encoding').textContent = worker ? worker.activeEncodes : '—';
  $('stat-encoding-caption').textContent = worker ? `of ${worker.maxEncodes} concurrent encodes` : 'Active encodes';
  $('stat-ready').textContent = worker ? jobs.filter(j => j.downloaded && j.state === 'queued' && j.phase === 'FFMPEG').length : '—';
  $('stat-downloading').textContent = worker ? jobs.filter(j => j.phase === 'Download' && j.state === 'running').length : '—';
  $('local-count').textContent = jobs.length;
  $('job-list').innerHTML = jobs.length ? jobs.map(j => {
    const p = progress.find(x => x.jobId === j.jobId && x.phase === j.phase);
    const [label, badge] = jobLabel(j); const name = j.name || p?.name || `Job ${j.jobId.slice(0,8)}`;
    const pct = p ? Math.min(100, Math.max(0, Number(p.percent) || 0)) : 0;
    const profile = p?.profile || j.profile;
    return `<article class="job"><div class="job-head"><div class="file-icon">▶</div><div class="job-info"><div class="job-title" title="${escapeHTML(name)}">${escapeHTML(name)}</div><div class="job-meta">${escapeHTML(duration(p?.etaSeconds) || (j.downloaded ? 'File downloaded' : 'Preparing file'))}${profile ? ` · ${escapeHTML(profile.toUpperCase())}` : ''}</div></div><span class="badge ${badge}">${label}</span></div>${p ? `<div class="job-progress"><div class="track"><div class="fill" style="width:${pct}%"></div></div><strong>${pct.toFixed(0)}%</strong></div>` : ''}</article>`;
  }).join('') : empty(status.configured ? 'All quiet here' : 'Set up your worker', status.configured ? 'New jobs will appear here automatically.' : 'Configure the connection to get started.');
  const queue = status.queue;
  $('stat-queue').textContent = queue ? queue.total : '—'; $('queue-count').textContent = queue ? queue.total : '—';
  $('stat-queue-caption').textContent = queue ? 'Awaiting assignment' : 'Unavailable';
  $('server-queue').innerHTML = queue ? (queue.jobs?.length ? queue.jobs.map((j, i) => `<div class="queue-item"><span class="queue-num">${String(i+1).padStart(2,'0')}</span><div class="queue-info"><div class="queue-name" title="${escapeHTML(j.name)}">${escapeHTML(j.name)}</div><div class="queue-meta">${escapeHTML(fileSize(j.size) ? `Awaiting assignment · ${fileSize(j.size)}` : 'Awaiting assignment')}</div></div><span class="badge ready">Queued</span></div>`).join('') : empty('Queue empty','There are no jobs waiting to be assigned.',true)) : empty('Queue unavailable',status.queueError === 'unsupported' ? 'Update the server to view its queue.' : 'The queue will appear when the server is available.',true);
  const notice = !status.configured ? 'Configure the connection to start your worker.' : status.pending ? 'Changes are pending and will be applied when current jobs finish.' : status.message || (worker?.maxActiveJobs === 0 ? 'New assignments are paused. Current jobs will continue.' : worker?.withinSchedule === false ? 'Outside the work schedule. New assignments will resume during the next window.' : '');
  $('notice').classList.toggle('hidden', !notice); $('notice-text').textContent = notice; $('notice-start').classList.add('hidden'); $('notice-settings').classList.remove('hidden');
  $('toggle-pause').disabled = !worker || status.pending;
  $('toggle-pause').textContent = isPaused ? '▶  Resume' : 'Ⅱ  Pause new jobs';
  if (!dirty && status.settings) { const s = status.settings; $('domain').value = s.domain; $('name').value = s.name; $('cpu-codec').value = s.cpuCodec; $('cpu-jobs').value = s.cpuJobs; $('gpu-codec').value = s.gpuCodec; $('gpu-jobs').value = s.gpuJobs; $('schedule').value = (s.schedule || []).join('\n'); $('token-note').textContent = s.tokenSet ? 'Token saved. Leave blank to keep it.' : 'Your token is stored securely on this machine.'; }
  if (!status.configured && !shownSetup) { shownSetup = true; page('settings'); request('show-window').catch(() => {}); }
}
async function refresh() {
  if (polling) return; polling = true;
  try { render(await request('status')); }
  catch (error) { window.__lastServiceError = String(error?.message || error); $('connection').className = 'connection error'; $('connection').innerHTML = '<i></i> Service disconnected'; $('notice').classList.remove('hidden'); $('notice-text').textContent = 'The Windows service is unavailable.'; $('notice-start').classList.remove('hidden'); $('notice-settings').classList.add('hidden'); $('toggle-pause').disabled = true; }
  finally { polling = false; }
}
$('refresh').addEventListener('click', refresh);
$('notice-start').addEventListener('click', async () => { try { await request('start-service'); toast('Starting service…'); setTimeout(refresh, 2000); } catch (err) { toast(err.message); } });
$('settings-form').addEventListener('input', () => { dirty = true; });
$('settings-form').addEventListener('submit', async e => {
  e.preventDefault(); $('save').disabled = true;
  const settings = {domain:$('domain').value.trim(), token:$('token').value, name:$('name').value.trim(), cpuCodec:$('cpu-codec').value, cpuJobs:Number($('cpu-jobs').value), gpuCodec:$('gpu-codec').value, gpuJobs:Number($('gpu-jobs').value), schedule:$('schedule').value.split(/\r?\n/).map(x => x.trim()).filter(Boolean)};
  try { const status = await request('configure', {settings}); $('token').value = ''; dirty = false; render(status); toast(status.pending ? 'Changes saved; they will apply when current jobs finish.' : 'Settings saved.'); page('overview'); }
  catch (err) { toast(err.message); } finally { $('save').disabled = false; }
});
$('toggle-pause').addEventListener('click', async () => { try { const paused = current?.worker?.maxActiveJobs === 0; const cpuJobs = paused ? lastJobs.cpu : 0, gpuJobs = paused ? lastJobs.gpu : 0; render(await request('set-max-jobs', {cpuJobs,gpuJobs})); toast(paused ? 'Worker profiles resumed.' : 'New assignments paused for both profiles.'); } catch (err) { toast(err.message); } });
$('show-token').addEventListener('click', () => { $('token').type = $('token').type === 'password' ? 'text' : 'password'; });
refresh(); setInterval(refresh, 2000);
