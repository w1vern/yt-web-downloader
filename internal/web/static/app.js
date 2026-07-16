'use strict';

/* ---------- range sync helpers (mirror of Go internal/ranges) ---------- */

function parseRanges(s, max) { // mirror of Go ranges.Parse; throws Error on bad input
  s = s.trim();
  if (!s) return null; // null = all
  const seen = new Set();
  for (let tok of s.split(',')) {
    tok = tok.trim();
    const m = /^(\d+)(?:\s*-\s*(\d+))?$/.exec(tok);
    if (!m) throw new Error(`bad token "${tok}"`);
    const a = +m[1], b = m[2] ? +m[2] : a;
    if (a < 1 || b < a || b > max) throw new Error(`bad range "${tok}"`);
    for (let i = a; i <= b; i++) seen.add(i);
  }
  return [...seen].sort((x, y) => x - y);
}

function formatRanges(items) { // [1,2,3,5] -> "1-3, 5"
  const parts = [];
  for (const v of items) {
    const last = parts[parts.length - 1];
    if (last && last[1] === v - 1) last[1] = v;
    else parts.push([v, v]);
  }
  return parts.map(([a, b]) => (a === b ? `${a}` : `${a}-${b}`)).join(', ');
}

/* ---------- generic helpers ---------- */

function $(sel) { return document.querySelector(sel); }
function $all(sel) { return document.querySelectorAll(sel); }

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function fmtDuration(sec) {
  sec = Math.round(sec || 0);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const ss = String(s).padStart(2, '0');
  if (h) return `${h}:${String(m).padStart(2, '0')}:${ss}`;
  return `${m}:${ss}`;
}

function fmtSize(n) {
  if (n === undefined || n === null) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return (i === 0 ? v : v.toFixed(1)) + ' ' + units[i];
}

function fmtDate(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  return d.toLocaleString();
}

/* ---------- API helper ---------- */

async function api(path, opts = {}) {
  const fetchOpts = {
    method: opts.method || 'GET',
    credentials: 'same-origin',
  };
  if (opts.body !== undefined) {
    fetchOpts.headers = { 'Content-Type': 'application/json' };
    fetchOpts.body = JSON.stringify(opts.body);
  }
  const res = await fetch(path, fetchOpts);
  if (res.status === 401) {
    if (path !== '/api/login') {
      stopHomeTimer();
      stopJobES();
      showView('login');
    }
    let msg = 'unauthorized';
    try { const j = await res.json(); if (j.error) msg = j.error; } catch (e) { /* ignore */ }
    throw new Error(msg);
  }
  if (!res.ok) {
    let msg = res.statusText || `request failed (${res.status})`;
    try { const j = await res.json(); if (j.error) msg = j.error; } catch (e) { /* ignore */ }
    throw new Error(msg);
  }
  if (res.status === 204) return null;
  const ct = res.headers.get('Content-Type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

/* ---------- view switching / routing ---------- */

const VIEWS = ['login', 'home', 'options', 'job'];

function showView(name) {
  for (const v of VIEWS) $('#view-' + v).hidden = v !== name;
}

let homeTimer = null;
let jobES = null;

function stopHomeTimer() { if (homeTimer) { clearInterval(homeTimer); homeTimer = null; } }
function stopJobES() { if (jobES) { jobES.close(); jobES = null; } }

async function route() {
  stopHomeTimer();
  stopJobES();
  const hash = location.hash.replace(/^#/, '') || '/';
  const jobMatch = hash.match(/^\/job\/(.+)$/);
  if (jobMatch) {
    await renderJob(decodeURIComponent(jobMatch[1]));
  } else {
    await renderHome();
  }
}

/* ---------- login view ---------- */

$('#login-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const errEl = $('#login-error');
  errEl.textContent = '';
  const login = $('#login-login').value;
  const password = $('#login-password').value;
  try {
    await api('/api/login', { method: 'POST', body: { login, password } });
    $('#login-password').value = '';
    try {
      const cfg = await api('/api/config');
      appConfig.hasApiKey = !!cfg.has_api_key;
    } catch (e2) { /* ignore */ }
    if (location.hash === '' || location.hash === '#/' || location.hash === '#') {
      await route();
    } else {
      location.hash = '#/';
    }
  } catch (err) {
    errEl.textContent = err.message;
  }
});

$('#logout-btn').addEventListener('click', async () => {
  try { await api('/api/logout', { method: 'POST' }); } catch (e) { /* ignore */ }
  stopHomeTimer();
  stopJobES();
  showView('login');
});

/* ---------- home view ---------- */

async function renderHome() {
  showView('home');
  await refreshJobs();
  homeTimer = setInterval(refreshJobs, 5000);
}

async function refreshJobs() {
  let data;
  try {
    data = await api('/api/jobs');
  } catch (e) {
    return;
  }
  renderJobsList(data.jobs || []);
}

const STATE_LABEL = { queued: 'queued', running: 'running', done: 'done', error: 'error' };
const MODE_LABEL = { audio: 'Audio', video: 'Video', both: 'Audio + video', merged: 'Merged (mkv)' };

function renderJobsList(jobs) {
  const container = $('#jobs-list');
  if (!jobs.length) {
    container.innerHTML = '<p class="muted">No jobs yet</p>';
    return;
  }
  const sorted = [...jobs].sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
  container.innerHTML = '';
  for (const j of sorted) {
    const row = document.createElement('div');
    row.className = 'job-row';
    row.innerHTML = `
      <a class="job-title" href="#/job/${encodeURIComponent(j.id)}">${escapeHtml(j.title)}</a>
      <span class="badge badge-${j.state}">${STATE_LABEL[j.state] || escapeHtml(j.state)}</span>
      <span class="job-time">${fmtDate(j.created_at)}</span>
      <button class="btn btn-danger btn-small" type="button">Delete</button>
    `;
    row.querySelector('button').addEventListener('click', () => deleteJob(j.id));
    container.appendChild(row);
  }
}

async function deleteJob(id) {
  if (!confirm('Delete this job and its files?')) return;
  try {
    await api('/api/jobs/' + encodeURIComponent(id), { method: 'DELETE' });
    refreshJobs();
  } catch (e) {
    alert(e.message);
  }
}

$('#analyze-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const errEl = $('#analyze-error');
  errEl.textContent = '';
  const url = $('#url-input').value.trim();
  if (!url) return;
  const analyzeBtn = $('#analyze-form').querySelector('button[type=submit]');
  let info;
  try {
    if (analyzeBtn) analyzeBtn.disabled = true;
    info = await api('/api/resolve?url=' + encodeURIComponent(url));
  } catch (err) {
    errEl.textContent = err.message;
    return;
  } finally {
    if (analyzeBtn) analyzeBtn.disabled = false;
  }
  openOptions(url, info);
});

/* ---------- options view ---------- */

let optState = null; // { url, info }
let jobSubmitting = false;

// Keeps the audio/video/merge checkboxes, the resolution row and the
// download button consistent with each other. Renamed from the old
// updateModeVisibility now that it also owns merge-availability and the
// download button's disabled state, not just row visibility.
function syncModeControls() {
  const audio = $('#opt-audio').checked;
  const video = $('#opt-video').checked;
  const mergeCb = $('#opt-merge');
  const canMerge = audio && video;
  mergeCb.disabled = !canMerge;
  if (!canMerge) mergeCb.checked = false;
  $('#video-group').hidden = !video;
  $('#download-btn').disabled = jobSubmitting || !(audio || video);
}

$('#opt-audio').addEventListener('change', syncModeControls);
$('#opt-video').addEventListener('change', syncModeControls);

function openOptions(url, info) {
  optState = { url, info };
  stopHomeTimer();

  $('#options-title').textContent = info.title || url;
  const parts = [];
  if (info.channel) parts.push(info.channel);
  if (info.type === 'playlist') parts.push(`${(info.entries || []).length} videos`);
  else parts.push(fmtDuration(info.duration));
  $('#options-subtitle').textContent = parts.join(' · ');

  $('#opt-audio').checked = true;
  $('#opt-video').checked = false;
  $('#opt-merge').checked = false;
  $('#video-res-select').value = 'best';
  syncModeControls();

  $('#opt-thumbnail').checked = true;
  $('#opt-tags').checked = true;
  $('#opt-url-date').checked = false;

  const isPlaylist = info.type === 'playlist';
  const ptLabel = $('#playlist-tags-label');
  const ptHint = $('#playlist-tags-hint');
  const ptCheckbox = $('#opt-playlist-tags');
  ptLabel.hidden = !isPlaylist;
  ptHint.hidden = !isPlaylist || appConfig.hasApiKey;
  ptCheckbox.checked = false;
  ptCheckbox.disabled = !appConfig.hasApiKey;

  const entriesGroup = $('#entries-group');
  entriesGroup.hidden = !isPlaylist;
  if (isPlaylist) renderEntriesTable(info.entries || []);

  $('#options-error').textContent = '';
  showView('options');
}

function renderEntriesTable(entries) {
  const tbody = $('#entries-tbody');
  tbody.innerHTML = '';
  for (const e of entries) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td><input type="checkbox" checked data-index="${e.index}"></td>
      <td>${e.index}</td>
      <td>${escapeHtml(e.title)}</td>
      <td>${escapeHtml(e.channel)}</td>
      <td>${fmtDuration(e.duration)}</td>
    `;
    tbody.appendChild(tr);
  }
  tbody.querySelectorAll('input[type=checkbox]').forEach(cb => cb.addEventListener('change', updateRangesFromCheckboxes));
  updateRangesFromCheckboxes();
}

function collectCheckedIndices() {
  return Array.from($all('#entries-tbody input[type=checkbox]'))
    .filter(cb => cb.checked)
    .map(cb => Number(cb.dataset.index))
    .sort((a, b) => a - b);
}

function setCheckedIndices(indices) {
  const set = new Set(indices);
  $all('#entries-tbody input[type=checkbox]').forEach(cb => {
    cb.checked = set.has(Number(cb.dataset.index));
  });
}

function updateRangesFromCheckboxes() {
  const input = $('#ranges-input');
  input.classList.remove('invalid');
  input.value = formatRanges(collectCheckedIndices());
}

$('#ranges-input').addEventListener('input', (e) => {
  const max = (optState && optState.info.entries) ? optState.info.entries.length : 0;
  try {
    const parsed = parseRanges(e.target.value, max);
    const indices = parsed === null ? Array.from({ length: max }, (_, i) => i + 1) : parsed;
    e.target.classList.remove('invalid');
    setCheckedIndices(indices);
  } catch (err) {
    e.target.classList.add('invalid');
  }
});

$('#select-all').addEventListener('click', (e) => {
  e.preventDefault();
  $all('#entries-tbody input[type=checkbox]').forEach(cb => { cb.checked = true; });
  updateRangesFromCheckboxes();
});

$('#select-none').addEventListener('click', (e) => {
  e.preventDefault();
  $all('#entries-tbody input[type=checkbox]').forEach(cb => { cb.checked = false; });
  updateRangesFromCheckboxes();
});

$('#options-back').addEventListener('click', () => { location.hash = '#/'; });
$('#job-back').addEventListener('click', () => { location.hash = '#/'; });

$('#download-btn').addEventListener('click', async () => {
  const errEl = $('#options-error');
  errEl.textContent = '';
  if (!optState) return;

  const audio = $('#opt-audio').checked;
  const video = $('#opt-video').checked;
  const merge = $('#opt-merge').checked;
  if (!audio && !video) return;
  const mode = audio && video ? (merge ? 'merged' : 'both') : (audio ? 'audio' : 'video');
  const isPlaylist = optState.info.type === 'playlist';
  let items = [];
  if (isPlaylist) {
    const rangesInput = $('#ranges-input');
    if (rangesInput.classList.contains('invalid')) {
      errEl.textContent = 'Fix the ranges field before continuing.';
      return;
    }
    const max = (optState.info.entries || []).length;
    const checked = collectCheckedIndices();
    items = checked.length === max ? [] : checked;
  }

  const options = {
    url: optState.url,
    mode,
    video_res: $('#video-res-select').value,
    embed_thumbnail: $('#opt-thumbnail').checked,
    embed_metadata: $('#opt-tags').checked,
    tag_url_date: $('#opt-url-date').checked,
    tag_playlist: isPlaylist ? $('#opt-playlist-tags').checked : false,
    is_playlist: isPlaylist,
    items,
  };
  const title = optState.info.title || optState.url;

  const downloadBtn = $('#download-btn');
  try {
    jobSubmitting = true;
    downloadBtn.disabled = true;
    const job = await api('/api/jobs', { method: 'POST', body: { title, options } });
    location.hash = '#/job/' + encodeURIComponent(job.id);
  } catch (err) {
    errEl.textContent = err.message;
  } finally {
    jobSubmitting = false;
    syncModeControls();
  }
});

/* ---------- job view ---------- */

let jobState = null; // { id, progress: Map<videoId, rowEl>, logLines: [] }

async function renderJob(id) {
  showView('job');
  jobState = { id, progress: new Map(), logLines: [] };
  $('#job-log').textContent = '';
  $('#job-progress').innerHTML = '';
  $('#job-files-card').hidden = true;
  $('#job-error-box').hidden = true;
  $('#job-title').textContent = '';
  $('#job-badge').textContent = '';
  $('#job-badge').className = 'badge';

  const logDetails = $('#job-log')?.parentElement?.closest('details');
  if (logDetails && !logDetails._scrollListener) {
    logDetails._scrollListener = true;
    logDetails.addEventListener('toggle', () => {
      if (logDetails.open) {
        const pre = $('#job-log');
        if (pre) pre.scrollTop = pre.scrollHeight;
      }
    }, { once: false });
  }

  let detail;
  try {
    detail = await api('/api/jobs/' + encodeURIComponent(id));
  } catch (e) {
    return;
  }
  applyJobDetail(detail);

  if (detail.state === 'queued' || detail.state === 'running') {
    connectJobEvents(id);
  }
}

function setBadge(state) {
  const el = $('#job-badge');
  el.textContent = STATE_LABEL[state] || state || '';
  el.className = 'badge badge-' + state;
}

function applyJobDetail(detail) {
  $('#job-title').textContent = detail.title;
  setBadge(detail.state);
  // Note: the job detail API only exposes `mode`, not video_res, so the
  // summary line cannot include resolution.
  $('#job-options-summary').textContent = `Mode: ${MODE_LABEL[detail.mode] || detail.mode} · Created: ${fmtDate(detail.created_at)}`;

  if (Array.isArray(detail.log)) {
    jobState.logLines = detail.log.slice(-300);
    const pre = $('#job-log');
    pre.textContent = jobState.logLines.join('\n');
    pre.scrollTop = pre.scrollHeight;
  }

  const errBox = $('#job-error-box');
  if (detail.state === 'error') {
    errBox.hidden = false;
    errBox.textContent = detail.error || 'Job failed';
  } else {
    errBox.hidden = true;
  }

  if (detail.state === 'done') {
    renderJobFiles(detail);
  } else {
    $('#job-files-card').hidden = true;
  }
}

function upsertProgressRow(ev) {
  if (!jobState) return;
  const container = $('#job-progress');
  let row = jobState.progress.get(ev.video_id);
  if (!row) {
    row = document.createElement('div');
    row.className = 'progress-row';
    row.innerHTML = '<span class="progress-id"></span><span class="progress-percent"></span><span class="progress-speed"></span><span class="progress-eta"></span>';
    container.appendChild(row);
    jobState.progress.set(ev.video_id, row);
  }
  row.querySelector('.progress-id').textContent = ev.video_id || '';
  row.querySelector('.progress-percent').textContent = ev.percent || '';
  row.querySelector('.progress-speed').textContent = ev.speed || '';
  row.querySelector('.progress-eta').textContent = ev.eta ? ('ETA ' + ev.eta) : '';
}

function appendLog(line) {
  if (line === undefined || !jobState) return;
  jobState.logLines.push(line);
  if (jobState.logLines.length > 300) jobState.logLines.shift();
  const pre = $('#job-log');
  pre.textContent = jobState.logLines.join('\n');
  pre.scrollTop = pre.scrollHeight;
}

function handleJobEvent(ev) {
  if (!jobState) return;
  if (ev.type === 'progress') {
    upsertProgressRow(ev);
  } else if (ev.type === 'log') {
    appendLog(ev.line);
  } else if (ev.type === 'state') {
    setBadge(ev.state);
    if (ev.state === 'done' || ev.state === 'error') {
      const id = jobState.id;
      stopJobES();
      api('/api/jobs/' + encodeURIComponent(id)).then(applyJobDetail).catch(() => { /* ignore */ });
    }
  }
}

function connectJobEvents(id) {
  stopJobES();
  jobES = new EventSource('/api/jobs/' + encodeURIComponent(id) + '/events');
  jobES.onmessage = (ev) => {
    let data;
    try { data = JSON.parse(ev.data); } catch (e) { return; }
    handleJobEvent(data);
  };
  jobES.onerror = async () => {
    stopJobES();
    try {
      const detail = await api('/api/jobs/' + encodeURIComponent(id));
      applyJobDetail(detail);
      if (detail.state === 'queued' || detail.state === 'running') {
        connectJobEvents(id);
      }
    } catch (e) { /* ignore, will retry on next view visit */ }
  };
}

function renderJobFiles(detail) {
  const card = $('#job-files-card');
  const list = $('#job-files-list');
  list.innerHTML = '';
  for (const f of detail.files || []) {
    const li = document.createElement('li');
    const a = document.createElement('a');
    a.href = `/api/jobs/${encodeURIComponent(detail.id)}/files/${encodeURIComponent(f.name)}`;
    a.textContent = f.name;
    a.setAttribute('download', '');
    li.appendChild(a);
    const size = document.createElement('span');
    size.className = 'file-size';
    size.textContent = fmtSize(f.size);
    li.appendChild(size);
    list.appendChild(li);
  }
  $('#job-zip-link').href = `/api/jobs/${encodeURIComponent(detail.id)}/zip`;
  card.hidden = false;
}

$('#job-download-all-btn').addEventListener('click', () => {
  // Browsers throttle/prompt for multiple auto-triggered downloads, so fire
  // them staggered rather than all at once; each link already carries its
  // own href + download attribute from renderJobFiles.
  const links = [...$('#job-files-list').querySelectorAll('a[download]')];
  links.forEach((a, i) => setTimeout(() => a.click(), i * 300));
});

$('#job-delete-btn').addEventListener('click', async () => {
  if (!jobState) return;
  if (!confirm('Delete this job and its files?')) return;
  try {
    await api('/api/jobs/' + encodeURIComponent(jobState.id), { method: 'DELETE' });
    location.hash = '#/';
  } catch (e) {
    alert(e.message);
  }
});

/* ---------- init ---------- */

const appConfig = { hasApiKey: false };

async function init() {
  window.addEventListener('hashchange', route);
  try {
    await api('/api/me');
  } catch (e) {
    return;
  }
  try {
    const cfg = await api('/api/config');
    appConfig.hasApiKey = !!cfg.has_api_key;
  } catch (e) { /* ignore */ }
  await route();
}

init();
