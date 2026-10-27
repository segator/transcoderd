'use strict';
const $ = id => document.getElementById(id);
document.body.classList.toggle('browser-host', !window.chrome?.webview);
const pending = new Map();
let sequence = 0, current = null, dirty = false, shownSetup = false, lastJobs = {cpu:0,gpu:2}, polling = false, selectedFilters = new Set(), lastBrowserLimit = 1, dashboardPromptAllowed = true;
let configurationLocked = true, pendingUpdate = null;
try { pendingUpdate = JSON.parse(localStorage.getItem('transcoderd.pending-update') || 'null'); } catch {}
function lockConfiguration(locked) {
  configurationLocked = locked;
  $('settings-controls').disabled = locked;
  $('save').disabled = locked;
  $('show-token').disabled = locked;
  $('settings-lock').hidden = !locked;
  $('settings-lock').textContent = pendingUpdate?.pending ? 'Configuration is locked until the mandatory update completes.' : 'Configuration is locked until the service status is confirmed.';
  if (locked) $('toggle-pause').disabled = true;
}
function renderUpdate(update) {
  if (update !== undefined) {
    pendingUpdate = update?.pending ? update : null;
    try { if (pendingUpdate) localStorage.setItem('transcoderd.pending-update', JSON.stringify(pendingUpdate)); else localStorage.removeItem('transcoderd.pending-update'); } catch {}
  }
  const u = pendingUpdate;
  $('update-alert').hidden = !u?.pending;
  document.body.classList.toggle('update-pending', Boolean(u?.pending));
  if (u?.pending) {
    const phases = {pending:'Preparing update',prepared:'Preparing installation',downloading:'Downloading update',draining:'Waiting for accepted jobs',installing:'Installing update',verifying:'Verifying installation',recovering:'Recovering installation',retrying:'Update will retry automatically','restart-required':'Windows restart required'};
    $('update-title').textContent = `${phases[u.phase] || 'Mandatory update pending'} · ${u.targetVersion || 'new version'}`;
    $('update-message').textContent = u.message || 'New jobs are blocked. All accepted jobs and uploads will finish before installation.';
    const details = [`Installed: ${u.installedVersion || current?.version || 'unknown'}`];
    if (u.total > 0) details.push(`${fileSize(u.downloaded || 0) || '0 MB'} of ${fileSize(u.total)}`);
    if (current?.worker) details.push(`${current.worker.activeJobs || 0} accepted jobs · ${current.worker.reservedJobs || 0} requests finishing`);
    if (u.nextRetry && !u.nextRetry.startsWith('0001')) details.push(`Next attempt: ${new Date(u.nextRetry).toLocaleString('en-US')}`);
    $('update-detail').textContent = details.join(' · ');
    $('update-progress').style.width = `${u.total > 0 ? Math.min(100, Math.max(0, (u.downloaded || 0) * 100 / u.total)) : 0}%`;
  }
}
renderUpdate(); lockConfiguration(true);
function request(command, data = {}) {
  if (configurationLocked && (command === 'configure' || command === 'set-max-jobs')) return Promise.reject(new Error('Configuration is locked until the mandatory update completes or service status is confirmed.'));
  return new Promise((resolve, reject) => {
    if (!window.chrome?.webview) {
      (async () => {
        let token = localStorage.getItem('transcoderd.dashboard.token');
        if (!token) { if (!dashboardPromptAllowed) throw new Error('A dashboard token is required. Click refresh to enter it.'); dashboardPromptAllowed = false; token = window.prompt('Enter the worker dashboard access token'); if (!token) throw new Error('A dashboard token is required. Click refresh to enter it.'); localStorage.setItem('transcoderd.dashboard.token', token); }
        const url = command === 'status' ? '/api/status' : command === 'set-max-jobs' ? '/api/worker/max-jobs' : null;
        if (!url) throw new Error('This action is only available in the Windows desktop app.');
        const response = await fetch(url, {method:command === 'status' ? 'GET' : 'POST', headers:{'Authorization':`Bearer ${token}`,'Content-Type':'application/json'}, ...(command === 'status' ? {} : {body:JSON.stringify({maxJobs:data.maxJobs ?? (data.cpuJobs + data.gpuJobs)})})});
        if (response.status === 401) { localStorage.removeItem('transcoderd.dashboard.token'); dashboardPromptAllowed = false; throw new Error('Invalid dashboard token. Click refresh to enter it again.'); }
        if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error || `Dashboard request failed (${response.status}).`); }
        resolve(await response.json());
      })().catch(reject);
      return;
    }
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
function fileSize(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '';
  const units = ['MB','GB','TB'];
  const power = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log(bytes) / Math.log(1024)) - 2));
  const decimals = power === 0 && bytes < 1024 ** 2 ? 2 : 1;
  return `${(bytes / 1024 ** (power + 2)).toFixed(decimals)} ${units[power]}`;
}
function formatDuration(seconds) {
  if (!Number.isFinite(Number(seconds)) || seconds < 0) return '';
  const total = Math.floor(Number(seconds));
  const hours = Math.floor(total / 3600), minutes = Math.floor(total % 3600 / 60), remainder = total % 60;
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return `${minutes}m ${remainder}s`;
  return `${remainder}s`;
}
function formatFrames(value) { return Number(value || 0).toLocaleString('en-US'); }
function jobProgressText(progress) {
  const details = [];
  if ((progress.phase === 'Download' || progress.phase === 'Upload') && progress.total > 0) {
    details.push(`${fileSize(progress.value)} of ${fileSize(progress.total)}`);
    if (progress.phase === 'Download' && progress.bytesPerSecond > 0) details.push(`${fileSize(progress.bytesPerSecond)}/s`);
  } else if (progress.phase === 'FFMPEG') {
    if (progress.frames > 0) details.push(`${formatFrames(progress.frames)} frames`);
    if (progress.framesPerSecond > 0) details.push(`${progress.framesPerSecond.toFixed(1)} fps`);
    if (progress.encoderSpeed) details.push(progress.encoderSpeed);
  } else if (progress.total > 0) {
    details.push(`${formatFrames(progress.value)} of ${formatFrames(progress.total)}`);
  }
  if (progress.elapsedSeconds > 0) details.push(`Elapsed ${formatDuration(progress.elapsedSeconds)}`);
  if (progress.etaSeconds > 0) details.push(`ETA ${formatDuration(progress.etaSeconds)}`);
  return details.join(' · ');
}
function phaseDescription(phase) {
  return ({Download:'Downloading source', MKVExtract:'Extracting subtitles', PGS:'Reading subtitles (OCR)', FFMPEG:'Encoding video', Upload:'Uploading result', JobVerify:'Verifying result'})[phase] || 'Working';
}
function jobLabel(job) {
  if (job.state === 'queued') return job.downloaded && job.phase === 'FFMPEG' ? ['Ready to encode','ready'] : ['Queued',job.downloaded ? 'ready' : ''];
  return ({FFMPEG:['Encoding',''], Download:['Downloading','download'], PGS:['Subtitle OCR',''], MKVExtract:['Extracting subtitles',''], Upload:['Uploading','download'], JobVerify:['Verifying','']})[job.phase] || ['Working',''];
}
function encoderLabel(job, progress) {
  const codec = progress?.codec || job.codec || '';
  const profile = (progress?.profile || job.profile || (codec === 'av1_nvenc' ? 'gpu' : codec ? 'cpu' : '')).toLowerCase();
  if (!profile && !codec) return '';
  const names = {av1_nvenc:'NVENC · AV1',libsvtav1:'SVT-AV1',libx265:'x265'};
  const name = names[codec] || codec.toUpperCase();
  const label = profile ? `${profile.toUpperCase()}${name ? ` · ${name}` : ''}` : name;
  return `<span class="encoder-chip ${profile === 'gpu' ? 'gpu' : 'cpu'}"><span class="encoder-icon">${profile === 'gpu' ? '◈' : '◉'}</span>${escapeHTML(label)}</span>`;
}
function render(status) {
  current = status;
  renderUpdate(status.update || null);
  lockConfiguration(Boolean(status.update?.pending));
  $('check-update').hidden = !window.chrome?.webview || !status.update;
  document.body.classList.toggle('browser-host', !window.chrome?.webview);
  $('app-version').textContent = `VERSION ${status.version || '—'}`;
  if (!window.chrome?.webview) {
    document.querySelectorAll('.nav').forEach(el => { if (el.dataset.page === 'settings') el.hidden = true; });
    document.querySelectorAll('[data-goto="settings"]').forEach(el => { el.hidden = true; });
    if (status.worker?.maxActiveJobs > 0) lastBrowserLimit = status.worker.maxActiveJobs;
  }
  const worker = status.worker, jobs = worker?.jobs || [], progress = worker?.progress || [];
  const admission = status.update?.pending ? 'Mandatory update' : !status.configured ? 'Service connected' : !worker ? 'Worker starting' : worker.maxActiveJobs === 0 ? 'Paused' : worker.withinSchedule === false ? 'Outside schedule' : worker.accepting ? (worker.maxAcceptedJobs > 0 && (worker.activeJobs + worker.reservedJobs) >= worker.maxAcceptedJobs ? 'At capacity' : 'Accepting jobs') : 'Waiting for settings';
  $('connection').className = 'connection online'; $('connection').innerHTML = `<i></i> ${admission}`;
  const isPaused = worker?.maxActiveJobs === 0;
  if (status.settings) lastJobs = {cpu:status.settings.cpuJobs, gpu:status.settings.gpuJobs};
  $('stat-encoding').textContent = worker ? worker.activeEncodes : '—';
  $('stat-encoding-caption').textContent = worker ? `of ${worker.maxEncodes} concurrent encodes` : 'Active encodes';
  $('stat-ready').textContent = worker ? jobs.filter(j => j.downloaded && j.state === 'queued' && j.phase === 'FFMPEG').length : '—';
  $('stat-downloading').textContent = worker ? jobs.filter(j => j.phase === 'Download' && j.state === 'running').length : '—';
  const visibleJobs = TranscoderDashboard.filterJobs(jobs, selectedFilters);
  $('local-count').textContent = selectedFilters.size ? `${visibleJobs.length}/${jobs.length}` : jobs.length;
  $('job-list').innerHTML = visibleJobs.length ? visibleJobs.map(j => {
    const p = progress.find(x => x.jobId === j.jobId && x.phase === j.phase);
    const [label, badge] = jobLabel(j); const name = j.name || p?.name || `Job ${j.jobId.slice(0,8)}`;
    const pct = p ? Math.min(100, Math.max(0, Number(p.percent) || 0)) : 0;
    let details = p ? jobProgressText(p) : '';
    if (!details && j.state === 'queued') details = j.downloaded && j.phase === 'FFMPEG' ? 'Downloaded and ready to encode' : !j.downloaded && j.phase === 'Download' ? 'Waiting to download' : phaseDescription(j.phase);
    if (!details) details = phaseDescription(j.phase);
    return `<article class="job"><div class="job-head"><div class="file-icon">▶</div><div class="job-info"><div class="job-title" title="${escapeHTML(name)}">${escapeHTML(name)}</div><div class="job-meta">${escapeHTML(details)}</div>${encoderLabel(j,p)}</div><span class="badge ${badge}">${label}</span></div>${p ? `<div class="job-progress"><div class="track"><div class="fill" style="width:${pct}%"></div></div><strong>${pct.toFixed(0)}%</strong></div>` : ''}</article>`;
  }).join('') : jobs.length ? empty('No matching activity', 'Choose another filter or select all activity.') : empty(status.configured ? 'All quiet here' : 'Set up your worker', status.configured ? 'New jobs will appear here automatically.' : 'Configure the connection to get started.');
  const queue = status.queue;
  $('stat-queue').textContent = queue ? queue.total : '—'; $('queue-count').textContent = queue ? queue.total : '—';
  $('stat-queue-caption').textContent = queue ? 'Awaiting assignment' : 'Unavailable';
  $('server-queue').innerHTML = queue ? (queue.jobs?.length ? queue.jobs.map((j, i) => `<div class="queue-item"><span class="queue-num">${String(i+1).padStart(2,'0')}</span><div class="queue-info"><div class="queue-name" title="${escapeHTML(j.name)}">${escapeHTML(j.name)}</div><div class="queue-meta">${escapeHTML(fileSize(j.size) ? `Awaiting assignment · ${fileSize(j.size)}` : 'Awaiting assignment')}</div></div><span class="badge ready">Queued</span></div>`).join('') : empty('Queue empty','There are no jobs waiting to be assigned.',true)) : empty('Queue unavailable',status.queueError === 'unsupported' ? 'Update the server to view its queue.' : 'The queue will appear when the server is available.',true);
  const notice = !status.configured ? 'Configure the connection to start your worker.' : status.pending ? 'Changes are pending and will be applied when current jobs finish.' : status.message || (worker?.maxActiveJobs === 0 ? 'New assignments are paused. Current jobs will continue.' : worker?.withinSchedule === false ? 'Outside the work schedule. New assignments will resume during the next window.' : '');
  $('notice').classList.toggle('hidden', !notice); $('notice-text').textContent = notice; $('notice-start').classList.add('hidden'); $('notice-settings').classList.remove('hidden');
  $('toggle-pause').disabled = configurationLocked || !worker || status.pending;
  $('toggle-pause').textContent = isPaused ? '▶  Resume' : 'Ⅱ  Pause new jobs';
  if (!dirty && status.settings) { const s = status.settings; $('domain').value = s.domain; $('name').value = s.name; $('working-directory').value = s.workingDirectory || ''; $('cpu-codec').value = s.cpuCodec; $('cpu-jobs').value = s.cpuJobs; $('gpu-codec').value = s.gpuCodec; $('gpu-jobs').value = s.gpuJobs; $('schedule').value = (s.schedule || []).join('\n'); $('token-note').textContent = s.tokenSet ? 'Token saved. Leave blank to keep it.' : 'Your token is stored securely on this machine.'; }
  if (!status.configured && !shownSetup) { shownSetup = true; page('settings'); request('show-window').catch(() => {}); }
}
async function refresh() {
  if (polling) return; polling = true;
  try { render(await request('status')); window.__lastServiceError = null; }
  catch (error) { window.__lastServiceError = String(error?.message || error); $('connection').className = 'connection error'; $('connection').innerHTML = '<i></i> Service disconnected'; $('notice').classList.remove('hidden'); $('notice-text').textContent = window.chrome?.webview ? 'The Windows service is unavailable.' : (error?.message || 'The worker dashboard is unavailable.'); $('notice-start').classList.toggle('hidden', !window.chrome?.webview); $('notice-settings').classList.add('hidden'); $('toggle-pause').disabled = true; }
  finally { if (window.__lastServiceError) lockConfiguration(true); polling = false; }
}
$('check-update').addEventListener('click', async () => { $('check-update').disabled = true; try { await request('check-update'); toast('Checking for updates…'); } catch (err) { toast(err.message); } finally { $('check-update').disabled = false; } });
$('refresh').addEventListener('click', () => { if (!window.chrome?.webview) dashboardPromptAllowed = true; refresh(); });
$('notice-start').addEventListener('click', async () => { try { await request('start-service'); toast('Starting service…'); setTimeout(refresh, 2000); } catch (err) { toast(err.message); } });
$('settings-form').addEventListener('input', () => { dirty = true; });
$('settings-form').addEventListener('submit', async e => {
  e.preventDefault(); if (configurationLocked) { toast('Configuration is locked while the mandatory update completes.'); return; } $('save').disabled = true;
  const settings = {domain:$('domain').value.trim(), token:$('token').value, name:$('name').value.trim(), workingDirectory:$('working-directory').value.trim(), cpuCodec:$('cpu-codec').value, cpuJobs:Number($('cpu-jobs').value), gpuCodec:$('gpu-codec').value, gpuJobs:Number($('gpu-jobs').value), schedule:$('schedule').value.split(/\r?\n/).map(x => x.trim()).filter(Boolean)};
  try { const status = await request('configure', {settings}); $('token').value = ''; dirty = false; render(status); toast(status.pending ? 'Changes saved; they will apply when current jobs finish.' : 'Settings saved.'); page('overview'); }
  catch (err) { toast(err.message); } finally { $('save').disabled = configurationLocked; }
});
$('toggle-pause').addEventListener('click', async () => { try { const paused = current?.worker?.maxActiveJobs === 0; if (!window.chrome?.webview) { render(await request('set-max-jobs', {maxJobs:paused ? lastBrowserLimit : 0})); } else { const cpuJobs = paused ? lastJobs.cpu : 0, gpuJobs = paused ? lastJobs.gpu : 0; render(await request('set-max-jobs', {cpuJobs,gpuJobs})); } toast(paused ? 'Worker profiles resumed.' : 'New assignments paused for both profiles.'); } catch (err) { toast(err.message); } });
document.querySelectorAll('[data-filter]').forEach(button => button.addEventListener('click', () => {
  const filter = button.dataset.filter;
  if (filter === 'all') selectedFilters.clear();
  else if (selectedFilters.has(filter)) selectedFilters.delete(filter);
  else selectedFilters.add(filter);
  document.querySelectorAll('[data-filter]').forEach(chip => chip.classList.toggle('active', chip.dataset.filter === 'all' ? selectedFilters.size === 0 : selectedFilters.has(chip.dataset.filter)));
  if (current) render(current);
}));
$('show-token').addEventListener('click', () => { if (!configurationLocked) $('token').type = $('token').type === 'password' ? 'text' : 'password'; });
refresh(); setInterval(refresh, 2000);
