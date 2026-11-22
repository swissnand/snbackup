/* SWISSNAND Backup – Web-Konsole (Vanilla JS, keine externen Abhängigkeiten) */
'use strict';

/* ------------------------------------------------------------------ helpers */
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const P = {
  dash: '<rect x="3" y="3" width="7" height="9" rx="1.5"/><rect x="14" y="3" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/>',
  jobs: '<path d="m12 2 9 5-9 5-9-5 9-5z"/><path d="m3 12 9 5 9-5"/><path d="m3 17 9 5 9-5"/>',
  cal: '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>',
  db: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5"/><path d="M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>',
  restore: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  settings: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
  play: '<polygon points="6 3 20 12 6 21 6 3"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6"/>',
  copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>',
  shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>',
  lock: '<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.7-9h1.8a4.5 4.5 0 1 1 0 9z"/>',
  server: '<rect x="2" y="3" width="20" height="8" rx="2"/><rect x="2" y="13" width="20" height="8" rx="2"/><path d="M6 7h.01M6 17h.01"/>',
  hdd: '<path d="M22 12H2M5.5 5h13L22 12v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6z"/><path d="M6 16h.01M10 16h.01"/>',
  globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
  term: '<rect x="2" y="3" width="20" height="18" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/>',
  box: '<path d="M21 8 12 3 3 8v8l9 5 9-5z"/><path d="m3 8 9 5 9-5M12 13v8"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  mirror: '<path d="m17 3 4 4-4 4M3 7h18M7 21l-4-4 4-4M21 17H3"/>',
  archive: '<rect x="2" y="3" width="20" height="5" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8M10 12h4"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
  download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>',
  search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
  bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.9 1.9 0 0 0 3.4 0"/>',
  key: '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6M15.5 7.5l3 3L22 7l-3-3"/>',
  zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  alert: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01"/>',
  info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>',
  chev: '<path d="m9 18 6-6-6-6"/>',
  pause: '<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>',
  tag: '<path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L2 12V2h10l8.6 8.6a2 2 0 0 1 0 2.8z"/><path d="M7 7h.01"/>',
  layers: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
  sweep: '<path d="m19 5-7 7M14.5 9.5l-9 9 2 2 9-9M3 21h6"/><path d="m16 3 5 5"/>',
  cpu: '<rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/>',
  list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  power: '<path d="M18.4 6.6a9 9 0 1 1-12.8 0M12 2v10"/>',
  agent: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/><circle cx="12" cy="10" r="2"/>',
  vm: '<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4M7 8h4M7 12h10"/>',
  disk: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="2"/><path d="M12 3v3"/>',
  eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
};
const icon = (n, cls = '') => `<svg class="i ${cls}" viewBox="0 0 24 24">${P[n] || P.info}</svg>`;

const KIB = 1024;
function bytes(n) {
  n = Number(n || 0);
  if (n < KIB) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let i = -1;
  do { n /= KIB; i++; } while (n >= KIB && i < u.length - 1);
  return (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2)) + ' ' + u[i];
}
const pad = (n) => String(n).padStart(2, '0');
function dt(t, withYear) {
  if (!t) return '–';
  const d = new Date(t);
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${withYear ? d.getFullYear() + ' ' : ''} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
function rel(t) {
  if (!t) return '–';
  const s = (new Date(t) - Date.now()) / 1000;
  const a = Math.abs(s);
  let v;
  if (a < 60) v = 'gleich';
  else if (a < 3600) v = Math.round(a / 60) + ' min';
  else if (a < 86400) v = Math.round(a / 3600) + ' h';
  else { const n = Math.round(a / 86400); v = n === 1 ? '1 Tag' : n + ' Tagen'; }
  if (v === 'gleich') return s > 0 ? 'gleich' : 'gerade eben';
  return s > 0 ? 'in ' + v : 'vor ' + v;
}
function dur(ms) {
  if (ms == null) return '–';
  const s = Math.round(ms / 1000);
  if (s < 60) return s + ' s';
  if (s < 3600) return Math.floor(s / 60) + ' min ' + (s % 60) + ' s';
  return Math.floor(s / 3600) + ' h ' + Math.floor((s % 3600) / 60) + ' min';
}
const WD = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa'];
const WDL = ['Sonntag', 'Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag'];

const STATUS = {
  success: ['ok', 'Erfolgreich'], warning: ['warn', 'Warnung'], failed: ['err', 'Fehler'],
  cancelled: ['', 'Abgebrochen'], running: ['run', 'Läuft'], '': ['', 'Noch nie'],
};
const statusBadge = (s) => { const [c, l] = STATUS[s || ''] || ['', s]; return `<span class="badge ${c}"><span class="d"></span>${l}</span>`; };

const TYPES = {
  files: { t: 'backup', k: 'files', icon: 'folder', label: 'Datei-Backup', desc: 'Ordner & Dateien, verschlüsselt und dedupliziert – für immer inkrementell.' },
  postgres: { t: 'backup', k: 'postgres', icon: 'db', label: 'PostgreSQL', desc: 'Konsistente Dumps einzelner oder aller Datenbanken (auch im Docker-Container).' },
  mysql: { t: 'backup', k: 'mysql', icon: 'db', label: 'MySQL / MariaDB', desc: 'Transaktionssichere Dumps inkl. Routinen, Trigger und Events.' },
  docker: { t: 'backup', k: 'docker', icon: 'box', label: 'Docker-Volumes', desc: 'Volumes sichern, Container optional kurz stoppen.' },
  command: { t: 'backup', k: 'command', icon: 'term', label: 'Befehl / Stream', desc: 'Ausgabe beliebiger Programme sichern (z. B. LDAP, Redis, Exporte).' },
  proxmox: { t: 'backup', k: 'proxmox', icon: 'vm', label: 'Proxmox VE', desc: 'VMs und Container per vzdump, dedupliziert und verschlüsselt.' },
  hyperv: { t: 'backup', k: 'hyperv', icon: 'vm', label: 'Hyper-V', desc: 'VMs im laufenden Betrieb exportieren (Agent auf dem Host).' },
  image: { t: 'backup', k: 'image', icon: 'disk', label: 'Disk-Image', desc: 'Ganze Disks oder LVM-Volumes blockweise, optional per LVM-Snapshot.' },
  sysimage: { t: 'backup', k: 'sysimage', icon: 'shield', label: 'Windows Bare Metal', desc: 'Bootfähiges Systemabbild via Windows Server-Sicherung.' },
  mirror: { t: 'mirror', icon: 'mirror', label: 'Spiegelung (Sync)', desc: '1:1-Kopie auf NAS oder USB-Disk, optional mit Papierkorb.' },
  archive: { t: 'archive', icon: 'archive', label: 'Archiv', desc: 'Eigenständige .tar.gz-Datei, optional AES-256-verschlüsselt.' },
  copy: { t: 'copy', icon: 'layers', label: 'Backup-Kopie (3-2-1)', desc: 'Snapshots auf ein zweites Ziel kopieren, z. B. Offsite/Cloud.' },
  verify: { t: 'verify', icon: 'shield', label: 'Integritätsprüfung', desc: 'Regelmässig beweisen, dass Backups lesbar und unverändert sind.' },
  prune: { t: 'prune', icon: 'sweep', label: 'Bereinigung', desc: 'Aufbewahrung anwenden und Speicher freigeben.' },
};
function typeOf(j) {
  if (j.type === 'backup') return TYPES[j.source?.kind || 'files'] || TYPES.files;
  return TYPES[j.type] || TYPES.files;
}
const BK = { local: ['hdd', 'Lokal / NAS'], s3: ['cloud', 'S3-kompatibel'], webdav: ['globe', 'WebDAV'], rclone: ['link', 'rclone'] };

/* ------------------------------------------------------------------ state & api */
const S = { me: null, csrf: '', timers: [], cache: {} };

async function api(method, url, body) {
  const opt = { method, headers: {}, credentials: 'same-origin' };
  if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  if (method !== 'GET' && S.csrf) opt.headers['X-CSRF-Token'] = S.csrf;
  const r = await fetch(url, opt);
  const ct = r.headers.get('content-type') || '';
  const data = ct.includes('json') ? await r.json() : await r.text();
  if (r.status === 401 && url !== '/api/login') { S.me = null; renderLogin(); throw new Error('Nicht angemeldet'); }
  if (r.status === 403 && data && data.error === 'password_change_required') { renderForceChange(); throw new Error('Passwortänderung nötig'); }
  if (!r.ok) throw new Error((data && data.error) || ('HTTP ' + r.status));
  return data;
}
const GET = (u) => api('GET', u);
const POST = (u, b) => api('POST', u, b ?? {});
const PUT = (u, b) => api('PUT', u, b);
const DEL = (u) => api('DELETE', u);

function toast(msg, kind = 'ok') {
  const el = document.createElement('div');
  el.className = 'toast ' + kind;
  el.innerHTML = icon(kind === 'ok' ? 'check' : 'alert') + '<div>' + esc(msg) + '</div>';
  $('#toasts').appendChild(el);
  setTimeout(() => el.remove(), kind === 'ok' ? 3500 : 7000);
}
const fail = (e) => toast(e.message || String(e), 'err');

function modal({ title, body, foot = '', size = '', onMount, icon: ic }) {
  const ov = document.createElement('div');
  ov.className = 'overlay';
  ov.innerHTML = `<div class="modal ${size}" role="dialog" aria-modal="true">
    <div class="modal-h">${ic ? `<div class="typeicon">${icon(ic)}</div>` : ''}<h3>${esc(title)}</h3><button class="btn ghost icon x" data-close>${icon('x')}</button></div>
    <div class="modal-b">${body}</div>${foot ? `<div class="modal-f">${foot}</div>` : ''}</div>`;
  const close = () => { ov.remove(); document.removeEventListener('keydown', onKey); };
  const onKey = (e) => { if (e.key === 'Escape') close(); };
  ov.addEventListener('click', (e) => { if (e.target === ov || e.target.closest('[data-close]')) close(); });
  document.addEventListener('keydown', onKey);
  document.body.appendChild(ov);
  const m = { el: ov, close };
  if (onMount) onMount(m);
  return m;
}
function confirmDlg(text, ok = 'Bestätigen', danger = false) {
  return new Promise((res) => {
    const m = modal({ title: 'Bitte bestätigen', body: `<p>${text}</p>`, foot: `<button class="btn" data-close>Abbrechen</button><button class="btn ${danger ? 'danger' : 'primary'}" id="cok">${esc(ok)}</button>` });
    $('#cok', m.el).onclick = () => { m.close(); res(true); };
    m.el.addEventListener('click', (e) => { if (e.target.closest('[data-close]')) res(false); });
  });
}

/* form helpers: name="a.b.c" data-type=int|bool|lines|intlist|float */
function readForm(root, base = {}) {
  const out = JSON.parse(JSON.stringify(base));
  const set = (path, v) => { const ks = path.split('.'); let o = out; ks.slice(0, -1).forEach((k) => { o[k] = o[k] && typeof o[k] === 'object' ? o[k] : {}; o = o[k]; }); o[ks[ks.length - 1]] = v; };
  const lists = {};
  $$('[name]', root).forEach((el) => {
    const t = el.dataset.type || '';
    const n = el.name;
    if (t === 'intlist') { lists[n] = lists[n] || []; if (el.checked) lists[n].push(parseInt(el.value, 10)); return; }
    if (el.type === 'radio') { if (el.checked) set(n, el.value); return; }
    let v;
    if (t === 'bool' || el.type === 'checkbox') v = el.checked;
    else if (t === 'int') v = parseInt(el.value, 10) || 0;
    else if (t === 'lines') v = el.value.split('\n').map((s) => s.trim()).filter(Boolean);
    else if (t === 'csv') v = el.value.split(',').map((s) => s.trim()).filter(Boolean);
    else v = el.value;
    set(n, v);
  });
  Object.entries(lists).forEach(([n, v]) => set(n, v));
  return out;
}
const get = (o, path) => path.split('.').reduce((a, k) => (a == null ? a : a[k]), o);
const val = (o, p, d = '') => { const v = get(o, p); return v == null ? d : v; };
const inp = (o, name, opts = {}) => {
  const v = val(o, name, opts.def ?? '');
  const t = opts.type || 'text';
  const dv = opts.data ? ` data-type="${opts.data}"` : t === 'number' ? ' data-type="int"' : '';
  return `<input type="${t}" name="${name}"${dv} value="${esc(Array.isArray(v) ? v.join(', ') : v)}" placeholder="${esc(opts.ph || '')}"${opts.min != null ? ` min="${opts.min}"` : ''}${opts.max != null ? ` max="${opts.max}"` : ''}${opts.attrs || ''}>`;
};
const area = (o, name, ph = '', rows = 4) => `<textarea name="${name}" data-type="lines" rows="${rows}" placeholder="${esc(ph)}">${esc((val(o, name, []) || []).join('\n'))}</textarea>`;
const sel = (o, name, options, def = '') => `<select name="${name}">${options.map(([v, l]) => `<option value="${esc(v)}"${String(val(o, name, def)) === String(v) ? ' selected' : ''}>${esc(l)}</option>`).join('')}</select>`;
const chk = (o, name, title, sub = '') => `<label class="check"><input type="checkbox" name="${name}" data-type="bool"${val(o, name, false) ? ' checked' : ''}><div><b>${title}</b>${sub ? `<span>${sub}</span>` : ''}</div></label>`;
const field = (label, control, hint = '') => `<div class="field"><label>${label}</label>${control}${hint ? `<div class="hint">${hint}</div>` : ''}</div>`;

/* ------------------------------------------------------------------ shell */
const NAV = [
  ['dashboard', 'Übersicht', 'dash'],
  ['jobs', 'Backup-Jobs', 'jobs'],
  ['schedule', 'Zeitplan', 'cal'],
  ['targets', 'Backup-Ziele', 'db'],
  ['restore', 'Wiederherstellen', 'restore'],
  ['activity', 'Aktivität', 'activity'],
  ['agents', 'Agenten', 'agent'],
];
const logo = '<img class="logo" src="/logo.svg" alt="">';
const brand = `<div class="brand">${logo}<div class="wm"><b>SWISSNAND</b><span><em>Backup</em> · Tools</span></div></div>`;

function clearTimers() { S.timers.forEach(clearInterval); S.timers = []; }
function every(ms, fn) { S.timers.push(setInterval(fn, ms)); }

function shell(route) {
  const lic = S.me?.license || {};
  const root = $('#root');
  if (!$('.app', root)) {
    root.innerHTML = `<div class="app"><aside class="sidebar">${brand}<nav class="nav" id="nav"></nav>
      <div class="side-foot"><div class="server-pill"><span class="dot"></span><div class="t"><b id="srv"></b><span id="srvsub"></span></div></div>
      <div class="edition"><span>Edition</span><b id="ed"></b></div></div></aside>
      <main class="main"><header class="topbar"><div><h1 id="title"></h1><div class="crumb" id="crumb"></div></div><div class="spacer"></div><div class="flex" id="topact"></div>
      <button class="btn ghost icon" data-act="theme" title="Hell/Dunkel">${icon(document.documentElement.dataset.theme === 'light' ? 'moon' : 'sun')}</button>
      <button class="btn ghost" data-act="profile">${icon('user')}<span id="uname"></span></button>
      <button class="btn ghost icon" data-act="logout" title="Abmelden">${icon('logout')}</button></header>
      <div class="content" id="page"></div></main></div>`;
  }
  $('#nav').innerHTML = NAV.map(([r, l, i]) => `<a href="#/${r}" class="${route === r ? 'active' : ''}">${icon(i)}${l}<span class="badge run hide" id="nb-${r}"></span></a>`).join('') +
    `<div class="sec">Verwaltung</div><a href="#/settings" class="${route === 'settings' ? 'active' : ''}">${icon('settings')}Einstellungen</a>`;
  $('#srv').textContent = S.me?.server || '';
  $('#srvsub').textContent = 'v' + (S.me?.version || '') + ' · online';
  $('#ed').textContent = lic.edition || 'Community';
  $('#uname').textContent = S.me?.username || '';
  $('#topact').innerHTML = '';
}
function setTitle(t, crumb = '') { $('#title').textContent = t; $('#crumb').textContent = crumb; document.title = t + ' · SWISSNAND Backup'; }
const page = () => $('#page');

async function route() {
  clearTimers();
  if (!S.me) return;
  const h = location.hash.replace(/^#\/?/, '') || 'dashboard';
  const [r, ...rest] = h.split('/');
  shell(r);
  const fn = { dashboard: pgDashboard, jobs: pgJobs, schedule: pgSchedule, targets: pgTargets, restore: pgRestore, activity: pgActivity, agents: pgAgents, settings: pgSettings }[r] || pgDashboard;
  try { await fn(rest); } catch (e) { if (e.message !== 'Nicht angemeldet') page().innerHTML = `<div class="alert err">${icon('alert')}${esc(e.message)}</div>`; }
}
window.addEventListener('hashchange', route);

/* ------------------------------------------------------------------ login */
function renderLogin(err = '') {
  clearTimers();
  $('#root').innerHTML = `<div class="login"><section class="hero"><div class="grid-bg"></div>${brand}
    <h2>Ihre Daten.<br><em>Sicher gesichert.</em></h2>
    <p>Verschlüsselte, deduplizierte Backups für Linux- und Windows-Server – geplant, geprüft und jederzeit wiederherstellbar.</p>
    <ul><li>${icon('lock')}AES-256-GCM Ende-zu-Ende</li><li>${icon('zap')}Forever-Incremental & Dedup</li><li>${icon('cloud')}S3, NAS, WebDAV, 70+ Clouds</li><li>${icon('shield')}Object Lock gegen Ransomware</li><li>${icon('cal')}Flexible Zeitpläne & GFS</li><li>${icon('db')}PostgreSQL, MySQL, Docker</li></ul>
    <div class="muted">SWISSNAND Tools · Swiss made software</div></section>
    <section class="panel"><form id="lf" autocomplete="on"><h3>Anmelden</h3><p>Melde dich an der Backup-Konsole an.</p>
    ${err ? `<div class="alert err">${icon('alert')}${esc(err)}</div>` : ''}
    <div class="field"><label>Benutzername</label><input type="text" name="username" autocomplete="username" required autofocus></div>
    <div class="field"><label>Passwort</label><input type="password" name="password" autocomplete="current-password" required></div>
    <div class="field hide" id="totpf"><label>Bestätigungscode (Authenticator-App)</label><input type="text" name="totp" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="123456"></div>
    <button class="btn primary" type="submit">${icon('lock')}Anmelden</button>
    <div class="foot">Verbindung TLS-verschlüsselt · Fehlversuche werden gesperrt und protokolliert</div></form></section></div>`;
  $('#lf').onsubmit = async (e) => {
    e.preventDefault();
    const f = readForm(e.target);
    try {
      const r = await api('POST', '/api/login', f);
      if (r.totpRequired) { $('#totpf').classList.remove('hide'); $('[name=totp]').focus(); return; }
      S.csrf = r.csrf;
      await boot();
    } catch (er) { renderLogin(er.message); }
  };
}

function renderForceChange() {
  clearTimers();
  $('#root').innerHTML = `<div class="login"><section class="hero"><div class="grid-bg"></div>${brand}<h2>Willkommen bei<br><em>SWISSNAND Backup</em></h2><p>Bitte lege zuerst ein persönliches, starkes Passwort fest. Das Startpasswort wird danach ungültig.</p></section>
  <section class="panel"><form id="pf"><h3>Neues Passwort</h3><p>Mindestens 12 Zeichen, 3 Zeichenarten.</p><div id="perr"></div>
  <div class="field"><label>Aktuelles Passwort</label><input type="password" name="current" required></div>
  <div class="field"><label>Neues Passwort</label><input type="password" name="new" required minlength="12" autocomplete="new-password"></div>
  <div class="field"><label>Wiederholen</label><input type="password" name="new2" required></div>
  <button class="btn primary">${icon('check')}Speichern & weiter</button></form></section></div>`;
  $('#pf').onsubmit = async (e) => {
    e.preventDefault();
    const f = readForm(e.target);
    if (f.new !== f.new2) { $('#perr').innerHTML = `<div class="alert err">${icon('alert')}Passwörter stimmen nicht überein</div>`; return; }
    try { await POST('/api/me/password', { current: f.current, new: f.new }); toast('Passwort geändert'); await boot(); }
    catch (er) { $('#perr').innerHTML = `<div class="alert err">${icon('alert')}${esc(er.message)}</div>`; }
  };
}

async function boot() {
  try {
    const me = await fetch('/api/me', { credentials: 'same-origin' });
    if (me.status !== 200) return renderLogin();
    S.me = await me.json();
    S.csrf = S.me.csrf || S.csrf;
    if (S.me.mustChange) return renderForceChange();
    $('#root').innerHTML = '';
    route();
  } catch (e) { renderLogin(); }
}

/* ------------------------------------------------------------------ global actions */
document.addEventListener('click', async (e) => {
  const a = e.target.closest('[data-act]');
  if (!a) return;
  const act = a.dataset.act;
  const id = a.dataset.id;
  try {
    if (act === 'theme') {
      const t = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
      document.documentElement.dataset.theme = t;
      try { localStorage.setItem('snb-theme', t); } catch (er) { /* ignore */ }
      a.innerHTML = icon(t === 'light' ? 'moon' : 'sun');
    } else if (act === 'logout') { await POST('/api/logout'); S.me = null; renderLogin(); }
    else if (act === 'profile') { location.hash = '#/settings/security'; }
    else if (act === 'run') { await POST(`/api/jobs/${id}/run`, { full: a.dataset.full === '1' }); toast('Job gestartet'); setTimeout(route, 400); }
    else if (act === 'stop') { await POST(`/api/jobs/${id}/stop`); toast('Abbruch angefordert'); }
    else if (act === 'toggle') { const r = await POST(`/api/jobs/${id}/toggle`); toast(r.enabled ? 'Job aktiviert' : 'Job pausiert'); route(); }
    else if (act === 'edit-job') { openJobWizard(id); }
    else if (act === 'new-job') { openJobWizard(null, a.dataset.type); }
    else if (act === 'clone-job') { await POST(`/api/jobs/${id}/clone`); toast('Job dupliziert (pausiert)'); route(); }
    else if (act === 'del-job') { if (await confirmDlg('Job wirklich löschen? Bereits gesicherte Daten bleiben im Ziel erhalten.', 'Löschen', true)) { await DEL('/api/jobs/' + id); toast('Job gelöscht'); route(); } }
    else if (act === 'log') { openLog(id, a.dataset.title); }
    else if (act === 'job-detail') { openJobDetail(id); }
  } catch (er) { fail(er); }
});

/* ------------------------------------------------------------------ charts */
function barChart(days) {
  const W = 720, H = 220, L = 34, B = 26, T = 10;
  const max = Math.max(1, ...days.map((d) => d.success + d.warning + d.failed));
  const step = (W - L - 8) / days.length;
  const bw = Math.min(28, step * 0.55);
  const y = (v) => H - B - (v / max) * (H - B - T);
  let g = '';
  const ticks = [0, Math.ceil(max / 2), max];
  ticks.forEach((t) => { g += `<line class="gl" x1="${L}" x2="${W}" y1="${y(t)}" y2="${y(t)}"/><text x="${L - 8}" y="${y(t) + 4}" text-anchor="end">${t}</text>`; });
  days.forEach((d, i) => {
    const x = L + i * step + (step - bw) / 2;
    let base = 0;
    [['success', '#2fbf71'], ['warning', '#f0a23b'], ['failed', '#f0596a']].forEach(([k, c]) => {
      const v = d[k];
      if (!v) return;
      const y0 = y(base), y1 = y(base + v);
      g += `<rect x="${x}" y="${y1}" width="${bw}" height="${Math.max(2, y0 - y1 - 1)}" rx="4" fill="${c}"><title>${d.date}: ${v} ${STATUS[k][1]}</title></rect>`;
      base += v;
    });
    if (!base) g += `<rect x="${x}" y="${H - B - 3}" width="${bw}" height="3" rx="1.5" fill="var(--border-2)"/>`;
    const dd = new Date(d.date + 'T12:00:00');
    if (i % 2 === 1 || days.length <= 7) g += `<text x="${x + bw / 2}" y="${H - 8}" text-anchor="middle">${WD[dd.getDay()]} ${dd.getDate()}.</text>`;
  });
  return `<svg class="chart" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none">${g}</svg>`;
}
function donut(pct, color = 'url(#dg)') {
  const r = 52, c = 2 * Math.PI * r, v = Math.max(0, Math.min(100, pct));
  return `<svg class="donut" viewBox="0 0 132 132"><defs><linearGradient id="dg" x1="0" x2="1"><stop offset="0" stop-color="#1f6fb5"/><stop offset="1" stop-color="#4da3ff"/></linearGradient></defs>
  <circle cx="66" cy="66" r="${r}" fill="none" stroke="var(--surface-3)" stroke-width="12"/>
  <circle cx="66" cy="66" r="${r}" fill="none" stroke="${color}" stroke-width="12" stroke-linecap="round" stroke-dasharray="${(c * v) / 100} ${c}" transform="rotate(-90 66 66)"/>
  <text x="66" y="66" text-anchor="middle" dominant-baseline="central" font-size="24" font-weight="750" fill="var(--text)">${v.toFixed(v >= 99.95 || v === 0 ? 0 : 1)}%</text></svg>`;
}
const progressBar = (p, striped) => `<div class="progress ${striped ? 'striped' : ''}"><i style="width:${Math.max(2, Math.min(100, p)).toFixed(1)}%"></i></div>`;

function liveCard(l) {
  const p = l.progress || {};
  const pct = p.totalBytes ? (p.bytes / p.totalBytes) * 100 : 0;
  const t = TYPES[l.run.jobType] || (l.run.jobType === 'restore' ? { icon: 'restore' } : TYPES.files);
  return `<div class="run-card"><div class="top"><div class="typeicon">${icon(t.icon)}</div><div style="flex:1;min-width:0"><b>${esc(l.run.jobName)}</b><div class="muted" style="font-size:12.5px">${esc(l.phase)}</div></div>
    ${statusBadge('running')}<button class="btn sm" data-act="log" data-id="${l.run.id}" data-title="${esc(l.run.jobName)}">${icon('list', 'sm')}Protokoll</button>
    ${l.run.jobType !== 'restore' ? `<button class="btn sm danger" data-act="stop" data-id="${l.run.jobId}">${icon('stop', 'sm')}Stopp</button>` : ''}</div>
    ${progressBar(pct || 8, true)}
    <div class="meta"><span>${pct ? pct.toFixed(0) + ' %' : 'wird berechnet…'}</span><span>${p.files || 0}${p.totalFiles ? ' / ' + p.totalFiles : ''} Dateien</span><span>${bytes(p.bytes)}${p.totalBytes ? ' / ' + bytes(p.totalBytes) : ''}</span><span>neu ${bytes(p.newBytes)}</span><span>übertragen ${bytes(p.storedBytes)}</span><span>seit ${rel(l.run.start).replace('vor ', '')}</span></div>
    ${p.current ? `<div class="cur">${esc(p.current)}</div>` : ''}</div>`;
}

/* ------------------------------------------------------------------ dashboard */
async function pgDashboard() {
  setTitle('Übersicht', 'Zustand aller Backups auf einen Blick');
  $('#topact').innerHTML = `<button class="btn primary" data-act="new-job">${icon('plus')}Neuer Job</button>`;
  const render = async () => {
    const d = await GET('/api/dashboard');
    const dedup = d.stored ? d.logical / d.stored : 0;
    const failing = d.failing ? `<span class="badge err">${d.failing} mit Fehler</span>` : `<span class="badge ok"><span class="d"></span>Alles in Ordnung</span>`;
    page().innerHTML = `
    ${!d.license.commercial ? `<div class="alert info">${icon('info')}<div><b>Community Edition</b> – kostenlos für private und nicht-kommerzielle Nutzung. Für den Einsatz im Unternehmen <a href="#/settings/license">Business-Lizenz aktivieren</a>.</div></div>` : ''}
    <div class="grid g4">
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('shield')}</span>Geschützte Daten</div><div class="val">${bytes(d.logical)}</div><div class="foot">auf ${d.repos ? d.repos.length : 0} Zielen</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('db')}</span>Belegter Speicher</div><div class="val">${bytes(d.stored)}</div><div class="foot">${dedup > 1.05 ? `Dedup & Kompression <b style="color:var(--ok)">${dedup.toFixed(1)}×</b>` : 'verschlüsselt mit AES-256'}</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('jobs')}</span>Backup-Jobs</div><div class="val">${d.enabled}<small>/ ${d.jobs} aktiv</small></div><div class="foot">${failing}</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('activity')}</span>Erfolgsquote 7 Tage</div><div class="val">${d.successRate.toFixed(1)}<small>%</small></div><div class="foot">${d.runs7} Läufe · ${bytes(d.transferred7)} übertragen</div></div>
    </div>
    ${d.running.length ? `<div class="card mt"><div class="card-h">${icon('zap')}<h3>Läuft gerade</h3><span class="badge run"><span class="d"></span>${d.running.length} aktiv</span></div>${d.running.map(liveCard).join('')}</div>` : ''}
    <div class="grid g-main mt">
      <div class="card"><div class="card-h"><h3>Backup-Läufe</h3><span class="sub">letzte 14 Tage</span><div class="r legend"><span><i style="background:#2fbf71"></i>Erfolg</span><span><i style="background:#f0a23b"></i>Warnung</span><span><i style="background:#f0596a"></i>Fehler</span></div></div><div class="card-b">${barChart(d.days)}</div></div>
      <div class="card"><div class="card-h"><h3>Gesundheit</h3><span class="sub">7 Tage</span></div><div class="card-b split">${donut(d.successRate, d.successRate < 90 ? '#f0596a' : undefined)}
        <div class="mini-legend" style="flex:1">${(() => { const t = d.days.slice(-7).reduce((a, x) => ({ s: a.s + x.success, w: a.w + x.warning, f: a.f + x.failed }), { s: 0, w: 0, f: 0 }); return `<div><span>Erfolgreich</span><b>${t.s}</b></div><div><span>Mit Warnung</span><b>${t.w}</b></div><div><span>Fehlgeschlagen</span><b>${t.f}</b></div><div><span>Neue Daten</span><b>${bytes(d.days.slice(-7).reduce((a, x) => a + x.bytes, 0))}</b></div>`; })()}</div></div></div>
    </div>
    <div class="grid g3 mt">
      <div class="card"><div class="card-h">${icon('clock')}<h3>Als Nächstes</h3><div class="r"><a href="#/schedule" class="btn sm ghost">Zeitplan ${icon('chev', 'sm')}</a></div></div>
        ${d.next && d.next.length ? d.next.map((n) => `<div class="list-item"><div class="typeicon">${icon((TYPES[n.type] || TYPES.files).icon)}</div><div class="grow"><b>${esc(n.name)}</b><span>${WDL[new Date(n.time).getDay()]}, ${dt(n.time)}</span></div><div class="when"><b>${rel(n.time)}</b></div></div>`).join('') : `<div class="empty"><h3>Keine geplanten Läufe</h3>Lege einen Job mit Zeitplan an.</div>`}</div>
      <div class="card"><div class="card-h">${icon('activity')}<h3>Letzte Aktivität</h3><div class="r"><a href="#/activity" class="btn sm ghost">Alle ${icon('chev', 'sm')}</a></div></div>
        ${d.recent && d.recent.length ? d.recent.slice(0, 6).map((r) => `<div class="list-item click" data-act="log" data-id="${r.id}" data-title="${esc(r.jobName)}"><div class="grow"><b>${esc(r.jobName)}</b><span>${dt(r.start)} · ${esc(r.message || '')}</span></div>${statusBadge(r.status)}</div>`).join('') : `<div class="empty">Noch keine Läufe</div>`}</div>
      <div class="card"><div class="card-h">${icon('db')}<h3>Backup-Ziele</h3><div class="r"><a href="#/targets" class="btn sm ghost">Verwalten ${icon('chev', 'sm')}</a></div></div>
        ${d.repos && d.repos.length ? d.repos.map((r) => { const u = r.usage || {}; const ratio = u.bytes ? u.logical / u.bytes : 0; const mx = Math.max(1, ...d.repos.map((x) => (x.usage || {}).bytes || 0)); return `<div class="list-item"><div class="typeicon">${icon((BK[r.kind] || BK.local)[0])}</div><div class="grow"><b>${esc(r.name)}</b><span>${bytes(u.bytes)} belegt · ${u.snapshots || 0} Snapshots${ratio > 1.05 ? ` · ${ratio.toFixed(1)}× gespart` : ''}${u.latest ? ' · zuletzt ' + rel(u.latest) : ''}</span>${progressBar(((u.bytes || 0) / mx) * 100)}</div></div>`; }).join('') : `<div class="empty"><h3>Noch kein Ziel</h3><a href="#/targets">Backup-Ziel hinzufügen</a></div>`}</div>
    </div>`;
    const nb = $('#nb-activity');
    if (nb) { nb.textContent = d.running.length; nb.classList.toggle('hide', !d.running.length); }
  };
  await render();
  every(4000, () => render().catch(() => {}));
}

/* ------------------------------------------------------------------ jobs */
async function pgJobs() {
  setTitle('Backup-Jobs', 'Was, wohin und wann gesichert wird');
  $('#topact').innerHTML = `<button class="btn primary" data-act="new-job">${icon('plus')}Neuer Job</button>`;
  let filter = '';
  const render = async () => {
    const jobs = (await GET('/api/jobs')) || [];
    if (!jobs.length) {
      page().innerHTML = `<div class="card"><div class="empty"><div class="ill">${icon('jobs', 'lg')}</div><h3>Noch keine Backup-Jobs</h3><p>Starte mit einem Datei-Backup – der Assistent führt dich in wenigen Schritten durch.</p><button class="btn primary" data-act="new-job">${icon('plus')}Ersten Job erstellen</button></div></div>
      <h3 class="mt mb">Vorlagen</h3><div class="types">${Object.entries(TYPES).map(([k, t]) => `<div class="type" data-act="new-job" data-type="${k}"><div class="typeicon">${icon(t.icon)}</div><b>${t.label}</b><span>${t.desc}</span></div>`).join('')}</div>`;
      return;
    }
    const f = jobs.filter((j) => !filter || (j.name + ' ' + j.repoName + ' ' + typeOf(j).label).toLowerCase().includes(filter));
    page().innerHTML = `<div class="toolbar"><div class="search">${icon('search', 'sm')}<input type="text" id="jq" placeholder="Jobs durchsuchen…" value="${esc(filter)}"></div><div class="grow"></div>
      <span class="muted">${jobs.filter((j) => j.enabled).length} aktiv · ${jobs.length} total</span></div>
      <div class="card"><table class="t"><thead><tr><th>Job</th><th>Ziel</th><th>Zeitplan</th><th>Letzter Lauf</th><th>Nächster Lauf</th><th>Status</th><th></th></tr></thead><tbody>
      ${f.map((j) => { const t = typeOf(j); return `<tr class="click" data-act="job-detail" data-id="${j.id}">
        <td><div class="t-name"><div class="typeicon">${icon(t.icon)}</div><div><b>${esc(j.name)}</b><span>${t.label} · ${j.agentName ? `${icon('agent', 'sm')} ${esc(j.agentName)}${j.agentOnline ? '' : ' <span style="color:var(--warn)">(offline)</span>'}` : 'Backup-Server'}${j.options?.tags?.length ? ' · ' + j.options.tags.map(esc).join(', ') : ''}</span></div></div></td>
        <td>${esc(j.repoName || j.targetPath || '–')}</td>
        <td><span class="${j.enabled ? '' : 'muted'}">${esc(j.scheduleText)}</span></td>
        <td>${j.lastRun ? dt(j.lastRun) + `<div class="muted" style="font-size:12px">${rel(j.lastRun)}</div>` : '<span class="muted">–</span>'}</td>
        <td>${j.enabled ? (j.nextRun ? dt(j.nextRun) + `<div class="muted" style="font-size:12px">${rel(j.nextRun)}</div>` : '<span class="muted">manuell</span>') : '<span class="badge"><span class="d"></span>pausiert</span>'}</td>
        <td>${j.busy ? statusBadge('running') : statusBadge(j.lastStatus)}</td>
        <td><div class="actions">
          ${j.busy ? `<button class="btn sm danger" data-act="stop" data-id="${j.id}" title="Stoppen">${icon('stop', 'sm')}</button>` : `<button class="btn sm" data-act="run" data-id="${j.id}" title="Jetzt ausführen">${icon('play', 'sm')}Start</button>`}
          <button class="btn sm icon" data-act="toggle" data-id="${j.id}" title="${j.enabled ? 'Pausieren' : 'Aktivieren'}">${icon(j.enabled ? 'pause' : 'power', 'sm')}</button>
          <button class="btn sm icon" data-act="edit-job" data-id="${j.id}" title="Bearbeiten">${icon('edit', 'sm')}</button>
        </div></td></tr>`; }).join('')}
      </tbody></table></div>`;
    const q = $('#jq');
    q.oninput = () => { filter = q.value.toLowerCase(); const pos = q.selectionStart; render().then(() => { const n = $('#jq'); n.focus(); n.setSelectionRange(pos, pos); }); };
    $$('.actions', page()).forEach((el) => el.addEventListener('click', (e) => e.stopPropagation(), true));
  };
  await render();
  every(5000, () => { if (document.activeElement?.id !== 'jq') render().catch(() => {}); });
}

async function openJobDetail(id) {
  const { job: j, runs } = await GET('/api/jobs/' + id);
  const t = typeOf(j);
  const ok = runs.filter((r) => r.status === 'success').length;
  modal({
    title: j.name, icon: t.icon, size: 'wide',
    body: `<div class="grid g4 mb">
      <div class="stat"><span>Typ</span><b>${t.label}</b></div><div class="stat"><span>Zeitplan</span><b>${esc(j.scheduleText)}</b></div>
      <div class="stat"><span>Nächster Lauf</span><b>${j.nextRun ? dt(j.nextRun) : '–'}</b></div><div class="stat"><span>Erfolgsquote</span><b>${runs.length ? Math.round((ok / runs.length) * 100) + ' %' : '–'}</b></div></div>
      <table class="t"><thead><tr><th>Start</th><th>Auslöser</th><th>Dauer</th><th>Daten</th><th>Ergebnis</th><th>Status</th></tr></thead><tbody>
      ${runs.length ? runs.map((r) => `<tr class="click" data-act="log" data-id="${r.id}" data-title="${esc(r.jobName)}"><td>${dt(r.start, true)}</td><td class="muted">${esc(r.trigger)}</td><td>${r.end ? dur(new Date(r.end) - new Date(r.start)) : '–'}</td><td>${r.stats ? bytes(r.stats.newBytes || r.stats.bytes) : '–'}</td><td class="muted">${esc(r.message || '')}</td><td>${statusBadge(r.status)}</td></tr>`).join('') : '<tr><td colspan="6" class="muted">Noch keine Läufe</td></tr>'}
      </tbody></table>`,
    foot: `<button class="btn danger" data-act="del-job" data-id="${j.id}" data-close>${icon('trash', 'sm')}Löschen</button><button class="btn" data-act="clone-job" data-id="${j.id}" data-close>${icon('copy', 'sm')}Duplizieren</button><div style="flex:1"></div>
      ${j.type === 'backup' ? `<button class="btn" data-act="run" data-id="${j.id}" data-full="1" data-close>${icon('zap', 'sm')}Vollbackup</button>` : ''}
      <button class="btn" data-act="edit-job" data-id="${j.id}" data-close>${icon('edit', 'sm')}Bearbeiten</button><button class="btn primary" data-act="run" data-id="${j.id}" data-close>${icon('play', 'sm')}Jetzt ausführen</button>`,
  });
}

async function openLog(runId, title) {
  let text = '';
  const live = () => (S.cache.running || []).find((l) => l.run.id === runId);
  const color = (t) => esc(t).split('\n').map((l) => l.includes('[ERROR]') ? `<span class="e">${l}</span>` : l.includes('[WARN]') ? `<span class="w">${l}</span>` : l.includes('▶') ? `<span class="p">${l}</span>` : l.includes('beendet: success') ? `<span class="s">${l}</span>` : l).join('\n');
  const m = modal({ title: 'Protokoll · ' + (title || ''), icon: 'list', size: 'wide', body: `<div class="log" id="logbox">Lade…</div>`, foot: `<button class="btn" id="logdl">${icon('download', 'sm')}Herunterladen</button><button class="btn primary" data-close>Schliessen</button>` });
  const load = async () => {
    try {
      const running = await GET('/api/activity');
      S.cache.running = running;
      const l = live();
      text = l ? l.log.join('\n') : await GET(`/api/runs/${runId}/log`);
    } catch (e) { text = e.message; }
    const box = $('#logbox', m.el);
    if (!box) return false;
    const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 30;
    box.innerHTML = color(text);
    if (atEnd) box.scrollTop = box.scrollHeight;
    return !!live();
  };
  const again = await load();
  $('#logdl', m.el).onclick = () => { const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain' })); a.download = 'snbackup-' + runId + '.log'; a.click(); };
  if (again) { const t = setInterval(async () => { if (!document.body.contains(m.el) || !(await load())) clearInterval(t); }, 1500); }
}

/* ------------------------------------------------------------------ job wizard */
const DEFAULT_JOB = () => ({
  name: '', enabled: true, type: 'backup', source: { kind: 'files', paths: [], excludes: ['*.tmp', '*.swp', '.cache', 'node_modules', 'Thumbs.db', '$RECYCLE.BIN'], excludeCaches: true, oneFileSystem: false },
  schedule: { kind: 'daily', times: ['22:00'], weekdays: [1, 2, 3, 4, 5], monthDays: [1], everyMin: 60, catchUp: true, jitterMin: 0 },
  retention: { keepLast: 3, keepDaily: 7, keepWeekly: 4, keepMonthly: 12, keepYearly: 3 }, pruneAfter: true,
  options: { mode: 'incremental', compression: 'auto', workers: 4, retries: 2, retryDelayMin: 10, priority: 'normal', verifyAfter: 'quick', verifyPercent: 5, archiveKeep: 7 },
  hooks: { abortOnPre: true, timeoutMin: 30 }, notify: { onSuccess: false, onWarning: true, onFailure: true, emails: [], webhooks: [] },
});

async function openJobWizard(id, preset) {
  const [repos, jobs, ag] = await Promise.all([GET('/api/repos'), GET('/api/jobs'), GET('/api/agents')]);
  const agents = (ag.agents || []).filter((a) => !a.agent.disabled);
  const runOn = () => `<div class="section"><h4>${icon('agent', 'sm')}Ausführen auf</h4>${agents.length ? field('Rechner', sel(job, 'agentId', [['', 'Backup-Server selbst (agentenlos)'], ...agents.map((a) => [a.agent.id, a.agent.name + ' · ' + a.agent.os + (a.online ? '' : ' (offline)')])]), 'Agentenlos: Netzfreigaben (NFS/SMB) und Datenbanken übers Netz. Mit Agent: lokale Dateien, VSS, VMs, Disk-Images auf diesem Rechner.') : `<p class="muted" style="margin-top:0">Der Job läuft auf dem Backup-Server. Für andere Rechner zuerst unter <a href="#/agents" data-close>Agenten</a> einen Agent installieren.</p>`}</div>`;
  let job = DEFAULT_JOB();
  if (id) { const r = await GET('/api/jobs/' + id); job = r.job; }
  else if (preset && TYPES[preset]) { job.type = TYPES[preset].t; if (TYPES[preset].k) job.source.kind = TYPES[preset].k; }
  let typeKey = Object.keys(TYPES).find((k) => typeOf(job) === TYPES[k]) || 'files';
  const STEPS = ['Typ', 'Quelle', 'Ziel', 'Zeitplan', 'Aufbewahrung', 'Optionen', 'Benachrichtigung', 'Zusammenfassung'];
  let step = id ? 1 : 0;
  const repoOpts = repos.map((r) => [r.repo.id, r.repo.name + ' (' + (BK[r.repo.backend.kind] || BK.local)[1] + ')']);

  const m = modal({ title: id ? 'Job bearbeiten' : 'Neuer Backup-Job', icon: 'jobs', size: 'xl', body: '', foot: '' });
  const mod = $('.modal', m.el);
  mod.querySelector('.modal-b').remove();
  const wiz = document.createElement('div');
  wiz.className = 'wizard';
  mod.insertBefore(wiz, null);
  const foot = document.createElement('div');
  foot.className = 'modal-f';
  mod.appendChild(foot);

  const collect = () => { const b = $('#wb', wiz); if (b) job = readForm(b, job); };
  const isBackup = () => job.type === 'backup';
  const needsRepo = () => ['backup', 'copy', 'verify', 'prune'].includes(job.type);
  const skip = (i) => (STEPS[i] === 'Aufbewahrung' && ['mirror', 'archive', 'verify'].includes(job.type)) || (STEPS[i] === 'Quelle' && ['verify', 'prune', 'copy'].includes(job.type));

  const views = {
    Typ: () => `<h2>Was möchtest du sichern?</h2><p class="lead">Wähle die Art des Jobs. Alle Varianten lassen sich frei planen und kombinieren.</p>
      <div class="types">${Object.entries(TYPES).map(([k, t]) => `<div class="type ${k === typeKey ? 'on' : ''}" data-type="${k}"><div class="typeicon">${icon(t.icon)}</div><b>${t.label}</b><span>${t.desc}</span>${k === 'files' ? '<span class="badge brand tag">Empfohlen</span>' : ''}</div>`).join('')}</div>`,
    Quelle: () => {
      const k = job.source.kind;
      if (job.type === 'backup' && k === 'proxmox') {
        return runOn() + `<h2>Proxmox VE</h2><p class="lead">Der Job muss auf dem Proxmox-Host laufen (Agent installieren). vzdump streamt jede VM unkomprimiert – dadurch funktioniert die Deduplizierung zwischen den Läufen.</p>
        <div class="section">${field('VM- und Container-IDs (eine pro Zeile)', area(job, 'source.vmIds', '100\n101\nall', 4), '«all» = alle VMs und Container des Hosts')}${field('vzdump-Modus', sel(job, 'source.vzdumpMode', [['snapshot', 'Snapshot (ohne Unterbruch)'], ['suspend', 'Suspend (kurz pausieren)'], ['stop', 'Stop (maximal konsistent)']], 'snapshot'))}</div>
        <div class="preview-box">${icon('restore', 'sm')} Wiederherstellen: <span class="kbd">snbackup dump &lt;snapshot&gt; _streams/proxmox/vzdump-qemu-101.vma | qmrestore - 101</span></div>`;
      }
      if (job.type === 'backup' && k === 'hyperv') {
        return runOn() + `<h2>Hyper-V</h2><p class="lead">Export-VM mit Produktions-Checkpoint: die VM läuft weiter. Der Agent muss auf dem Hyper-V-Host laufen.</p>
        <div class="section">${field('VM-Namen (einer pro Zeile)', area(job, 'source.hypervVMs', 'DC01\nSQL01\nFileserver', 4))}</div>
        <div class="preview-box">${icon('restore', 'sm')} Wiederherstellen: Ordner der VM zurückholen und in Hyper-V mit «Virtuellen Computer importieren» einbinden.</div>`;
      }
      if (job.type === 'backup' && k === 'image') {
        return runOn() + `<h2>Disk-Image</h2><p class="lead">Sichert ganze Disks, Partitionen oder LVM-Volumes Block für Block – für eine komplette Wiederherstellung eines Linux-Servers.</p>
        <div class="section">${field('Geräte (eines pro Zeile)', area(job, 'source.devices', '/dev/sda\n/dev/vg0/root', 3), 'Auch Image-Dateien möglich, z. B. /var/lib/libvirt/images/vm1.qcow2 (VM vorher anhalten)')}
        ${chk(job, 'source.lvmSnapshot', 'LVM-Snapshot verwenden', 'Konsistentes Abbild im laufenden Betrieb, Snapshot wird danach entfernt')}</div>
        <div class="preview-box">${icon('restore', 'sm')} Bare-Metal-Restore aus einem Rettungssystem: <span class="kbd">snbackup dump &lt;snapshot&gt; _streams/image/dev-sda.img &gt; /dev/sda</span></div>`;
      }
      if (job.type === 'backup' && k === 'sysimage') {
        return runOn() + `<h2>Windows Bare Metal</h2><p class="lead">Erstellt mit der Windows Server-Sicherung (wbadmin) ein bootfähiges Systemabbild und sichert es verschlüsselt ins Ziel.</p>
        <div class="section">${field('Zwischenspeicher für wbadmin', inp(job, 'targetPath', { ph: 'E:   oder   \\\\nas\\wbadmin' }), 'Eigenes Volume oder Netzfreigabe, nicht das Systemlaufwerk')}${field('Umfang', inp(job, 'source.sysVolumes', { ph: '-allCritical (Standard: alles für Bare Metal)' }), 'Oder Laufwerke, z. B. C:,D:')}</div>
        <div class="preview-box">${icon('restore', 'sm')} Wiederherstellen: Abbild zurückholen, vom Windows-Setup-Medium starten, «Computerreparaturoptionen → Systemimage-Wiederherstellung».</div>`;
      }
      if (job.type === 'backup' && (k === 'postgres' || k === 'mysql')) {
        return runOn() + `<h2>${k === 'postgres' ? 'PostgreSQL' : 'MySQL / MariaDB'}</h2><p class="lead">Die Dumps werden als Stream direkt verschlüsselt ins Ziel geschrieben – keine temporären Dateien.</p>
        <div class="section"><h4>${icon('server', 'sm')}Verbindung</h4><div class="row r3">${field('Host', inp(job, 'source.dbHost', { ph: 'localhost' }))}${field('Port', inp(job, 'source.dbPort', { type: 'number', ph: k === 'postgres' ? '5432' : '3306' }))}${field('Benutzer', inp(job, 'source.dbUser', { ph: k === 'postgres' ? 'postgres' : 'root' }))}</div>
        <div class="row">${field('Passwort', `<input type="password" name="source.dbPassword" value="${esc(job.source.dbPassword || '')}" autocomplete="new-password" placeholder="wird verschlüsselt gespeichert">`)}${field('Docker-Container (optional)', inp(job, 'source.dockerContainer', { ph: 'z. B. postgres-db' }), 'Dump wird mit docker exec im Container ausgeführt')}</div></div>
        <div class="section"><h4>${icon('db', 'sm')}Datenbanken</h4>${chk(job, 'source.allDatabases', 'Alle Datenbanken sichern', k === 'postgres' ? 'pg_dumpall inkl. Rollen und Tablespaces' : '--all-databases')}
        ${field('Oder einzelne Datenbanken (eine pro Zeile)', area(job, 'source.databases', 'shop\ncrm', 3), k === 'postgres' ? 'Format: pg_dump -Fc (custom, mit pg_restore selektiv wiederherstellbar)' : '')}
        ${field('Zusätzliche Argumente', inp(job, 'source.dumpArgs', { ph: k === 'postgres' ? '--no-owner' : '--column-statistics=0' }))}</div>`;
      }
      if (job.type === 'backup' && k === 'docker') {
        return runOn() + `<h2>Docker-Volumes</h2><p class="lead">Sichert benannte Volumes. Für Datenbanken in Containern besser den Datenbank-Typ verwenden.</p>
        <div class="section">${field('Volumes (eine pro Zeile)', area(job, 'source.dockerVolumes', 'nextcloud_data\ngitea_data'))}${field('Container während des Backups stoppen', area(job, 'source.stopContainers', 'nextcloud\ngitea', 2), 'Werden danach automatisch wieder gestartet')}${field('Zusätzliche Pfade', area(job, 'source.paths', '/opt/stacks', 2))}</div>
        <div class="section">${field('Ausschlüsse', area(job, 'source.excludes', '*.log', 3))}</div>`;
      }
      if (job.type === 'backup' && k === 'command') {
        return runOn() + `<h2>Befehl / Stream</h2><p class="lead">Die Standardausgabe des Befehls wird als Datei gesichert. Exit-Code ≠ 0 lässt den Job fehlschlagen.</p>
        <div class="section">${field('Befehl', inp(job, 'source.command', { ph: 'slapcat -n 1   |   redis-cli --rdb -   |   Get-ADUser -Filter * | ConvertTo-Json' }), 'Linux: /bin/sh -c · Windows: PowerShell')}${field('Dateiname im Backup', inp(job, 'source.commandName', { ph: 'ldap.ldif' }))}${field('Zusätzliche Pfade', area(job, 'source.paths', '/etc/openldap', 2))}</div>`;
      }
      return runOn() + `<h2>Quellen</h2><p class="lead">Ordner und Dateien, die gesichert werden. Ein Pfad pro Zeile – lokale Laufwerke, gemountete Freigaben oder UNC-Pfade.</p>
        <div class="section"><h4>${icon('folder', 'sm')}Pfade</h4>${field('Quellpfade', area(job, 'source.paths', '/etc\n/home\n/var/www\nC:\\Users\nD:\\Daten', 5))}
        <div class="preset-row">${[['Linux-Server', '/etc\n/home\n/root\n/var/www\n/opt\n/srv'], ['Windows-Server', 'C:\\Users\nC:\\inetpub\nD:\\Daten'], ['Webserver', '/var/www\n/etc/nginx\n/etc/letsencrypt']].map(([l, v]) => `<button type="button" class="btn sm" data-fill="source.paths" data-val="${esc(v)}">${icon('plus', 'sm')}${l}</button>`).join('')}</div></div>
        <div class="section"><h4>${icon('x', 'sm')}Ausschlüsse</h4>${field('Muster (eines pro Zeile)', area(job, 'source.excludes', '*.tmp\nnode_modules\n/var/log/**\n**/cache', 5), '<span class="kbd">*.iso</span> Dateiname · <span class="kbd">node_modules</span> Ordner überall · <span class="kbd">/var/log/**</span> ganzer Pfad · <span class="kbd">**/cache</span> beliebige Tiefe')}
        <div class="row">${field('Dateien grösser als (MB) überspringen', inp(job, 'source.excludeLargerMB', { type: 'number', ph: '0 = keine Grenze', min: 0 }))}<div></div></div>
        ${chk(job, 'source.excludeCaches', 'Cache-Ordner auslassen', 'Ordner mit CACHEDIR.TAG werden übersprungen')}
        ${chk(job, 'source.oneFileSystem', 'Im selben Dateisystem bleiben', 'Keine Mounts (z. B. /proc, Netzlaufwerke) unterhalb der Quellen')}
        ${job.type === 'backup' ? chk(job, 'source.useVSS', 'Windows VSS-Schattenkopie verwenden', 'Konsistente Sicherung geöffneter Dateien (Datenbanken, PST, Office) – nur Windows Server') : ''}</div>`;
    },
    Ziel: () => {
      if (!repos.length && needsRepo()) return `<h2>Ziel</h2><div class="empty"><div class="ill">${icon('db', 'lg')}</div><h3>Noch kein Backup-Ziel</h3><p>Lege zuerst ein Ziel an (Lokal/NAS, S3, WebDAV oder rclone).</p><a class="btn primary" href="#/targets" data-close>${icon('plus')}Ziel hinzufügen</a></div>`;
      if (job.type === 'mirror' || job.type === 'archive') {
        return `<h2>Zielordner</h2><p class="lead">${job.type === 'mirror' ? 'Die Spiegelung schreibt eine lesbare 1:1-Kopie.' : 'Pro Lauf entsteht eine neue Archivdatei.'}</p>
        <div class="section">${field('Zielordner', inp(job, 'targetPath', { ph: '/mnt/nas/spiegel  ·  E:\\Backup  ·  \\\\nas\\backup' }), 'Lokale Disk, USB-Disk, gemountete NFS/SMB-Freigabe')}
        ${job.type === 'mirror' ? chk(job, 'options.mirrorDelete', 'Gelöschte Dateien auch am Ziel löschen', 'Echte Spiegelung – sonst werden nur Dateien hinzugefügt/aktualisiert') + chk(job, 'options.mirrorTrash', 'Statt löschen in Papierkorb verschieben', '.snbackup-trash/&lt;Datum&gt; im Zielordner') : ''}
        ${job.type === 'archive' ? `<div class="row">${field('Anzahl Archive behalten', inp(job, 'options.archiveKeep', { type: 'number', min: 0 }), '0 = alle behalten')}<div></div></div>${chk(job, 'options.archiveEncrypt', 'Archiv verschlüsseln (AES-256-GCM)', 'Datei-Endung .tar.gz.snarc – mit «snbackup archive extract» entpacken')}${field('Archiv-Passwort', `<input type="password" name="options.archivePassword" value="${esc(job.options.archivePassword || '')}" autocomplete="new-password">`)}` : ''}</div>`;
      }
      return `<h2>${job.type === 'copy' ? 'Quelle & Ziel der Kopie' : 'Backup-Ziel'}</h2><p class="lead">${job.type === 'copy' ? '3-2-1-Regel: 3 Kopien, 2 Medien, 1 Kopie ausser Haus. Die Daten werden für das Ziel neu verschlüsselt.' : 'Alle Daten werden vor dem Upload mit AES-256-GCM verschlüsselt.'}</p>
        <div class="section">${field(job.type === 'copy' ? 'Quell-Repository' : 'Ziel', sel(job, 'repoId', repoOpts, repoOpts[0]?.[0]))}
        ${job.type === 'copy' ? field('Ziel-Repository', sel(job, 'targetRepo', repoOpts, repoOpts[1]?.[0])) + field('Nur Snapshots dieses Jobs kopieren', sel(job, 'copyFilter', [['', 'Alle Jobs'], ...jobs.filter((x) => x.type === 'backup').map((x) => [x.id, x.name])])) : ''}</div>
        ${job.type === 'backup' ? `<div class="section"><h4>${icon('zap', 'sm')}Backup-Modus</h4><div class="chips">${[['incremental', 'Forever-Incremental (empfohlen)'], ['full', 'Immer aktives Vollbackup']].map(([v, l]) => `<label class="chip"><input type="radio" name="options.mode" value="${v}"${(job.options.mode || 'incremental') === v ? ' checked' : ''}><span>${l}</span></label>`).join('')}</div>
        <p class="muted" style="font-size:12.5px">Jeder Snapshot ist ein vollständiger Wiederherstellungspunkt (synthetisches Vollbackup). Inkrementell liest nur geänderte Dateien, ein aktives Vollbackup liest alles neu.</p>
        ${field('Zusätzlich periodisches Vollbackup', sel(job, 'schedule.fullEvery', [['', 'Nie'], ['weekly:0', 'Jeden Sonntag'], ['weekly:6', 'Jeden Samstag'], ['monthly:1', 'Am 1. jedes Monats']]))}</div>` : ''}`;
    },
    Zeitplan: () => {
      const sk = job.schedule.kind || 'manual';
      const kinds = [['manual', 'Manuell'], ['interval', 'Intervall'], ['daily', 'Täglich'], ['weekly', 'Wöchentlich'], ['monthly', 'Monatlich'], ['cron', 'Cron'], ['after', 'Nach Job']];
      const times = (job.schedule.times || []).join(', ');
      return `<h2>Zeitplan</h2><p class="lead">Wann soll der Job laufen? Kombiniere Uhrzeiten, Wochentage, Backup-Fenster und Pausen.</p>
      <div class="section"><div class="chips mb">${kinds.map(([v, l]) => `<label class="chip"><input type="radio" name="schedule.kind" value="${v}"${sk === v ? ' checked' : ''} data-rerender><span>${l}</span></label>`).join('')}</div>
      ${sk === 'interval' ? `<div class="row">${field('Alle … Minuten', inp(job, 'schedule.everyMin', { type: 'number', min: 5 }), '60 = stündlich, 240 = alle 4 h, 15 = alle 15 min')}<div class="field"><label>Schnellwahl</label><div class="chips">${[[15, '15 min'], [60, '1 h'], [240, '4 h'], [360, '6 h'], [720, '12 h']].map(([v, l]) => `<button type="button" class="btn sm" data-fill="schedule.everyMin" data-val="${v}">${l}</button>`).join('')}</div></div></div>` : ''}
      ${['daily', 'weekly', 'monthly'].includes(sk) ? field('Uhrzeit(en)', `<input type="text" name="schedule.times" data-type="csv" value="${esc(times)}" placeholder="22:00, 12:30">`, 'Mehrere Zeiten mit Komma trennen – z. B. «07:00, 12:00, 18:00» für 3× täglich') : ''}
      ${sk === 'weekly' ? `<div class="field"><label>Wochentage</label><div class="chips">${[1, 2, 3, 4, 5, 6, 0].map((d) => `<label class="chip"><input type="checkbox" name="schedule.weekdays" data-type="intlist" value="${d}"${(job.schedule.weekdays || []).includes(d) ? ' checked' : ''}><span>${WD[d]}</span></label>`).join('')}</div></div>` : ''}
      ${sk === 'monthly' ? `<div class="field"><label>Tage im Monat</label><div class="chips">${[...Array(31).keys()].map((i) => i + 1).concat([-1]).map((d) => `<label class="chip"><input type="checkbox" name="schedule.monthDays" data-type="intlist" value="${d}"${(job.schedule.monthDays || []).includes(d) ? ' checked' : ''}><span>${d === -1 ? 'Letzter' : d}</span></label>`).join('')}</div></div>` : ''}
      ${sk === 'cron' ? field('Cron-Ausdruck', inp(job, 'schedule.cron', { ph: '30 2 * * 1-5' }), 'Minute Stunde Tag Monat Wochentag · auch @daily, @weekly, @hourly · z. B. <span class="kbd">0 */4 * * *</span> alle 4 h') : ''}
      ${sk === 'after' ? field('Ausführen nach', sel(job, 'schedule.afterJob', jobs.filter((x) => x.id !== job.id).map((x) => [x.id, x.name]))) + chk(job, 'schedule.onlySuccess', 'Nur wenn der vorherige Job erfolgreich war') : ''}
      </div>
      ${['interval', 'daily', 'weekly', 'monthly', 'cron'].includes(sk) ? `<div class="section"><h4>${icon('clock', 'sm')}Erweitert</h4>
        <div class="row r4">${field('Backup-Fenster von', inp(job, 'schedule.windowStart', { type: 'time' }))}${field('bis', inp(job, 'schedule.windowEnd', { type: 'time' }), 'Starts ausserhalb werden verschoben')}${field('Zufällige Verzögerung (min)', inp(job, 'schedule.jitterMin', { type: 'number', min: 0 }), 'Verteilt Last auf Speicher')}<div></div></div>
        <div class="row r4">${field('Pause von', inp(job, 'schedule.pauseFrom', { type: 'date' }))}${field('Pause bis', inp(job, 'schedule.pauseUntil', { type: 'date' }), 'z. B. Betriebsferien, Migration')}</div>
        ${chk(job, 'schedule.catchUp', 'Verpasste Läufe nachholen', 'Wenn der Server zur geplanten Zeit aus war, wird beim Start nachgeholt')}</div>
        <div class="preview-box" id="schedprev">${icon('cal', 'sm')} Vorschau wird berechnet…</div>` : ''}`;
    },
    Aufbewahrung: () => {
      const g = [['keepLast', 'Letzte'], ['keepHourly', 'Stündlich'], ['keepDaily', 'Täglich'], ['keepWeekly', 'Wöchentlich'], ['keepMonthly', 'Monatlich'], ['keepYearly', 'Jährlich'], ['keepWithinDays', 'Alle der letzten … Tage']];
      return `<h2>Aufbewahrung (GFS)</h2><p class="lead">Grossvater-Vater-Sohn: Wie viele Wiederherstellungspunkte pro Zeitraum behalten werden. Leer = nichts wird gelöscht.</p>
      <div class="preset-row">${[['Standard', { keepLast: 3, keepDaily: 7, keepWeekly: 4, keepMonthly: 12, keepYearly: 3 }], ['Kompakt', { keepLast: 3, keepDaily: 7, keepWeekly: 4 }], ['Stündlich', { keepHourly: 24, keepDaily: 14, keepWeekly: 8, keepMonthly: 6 }], ['Revisionssicher 10 J.', { keepDaily: 30, keepWeekly: 12, keepMonthly: 24, keepYearly: 10 }], ['30 Tage alles', { keepWithinDays: 30 }]].map(([l, p]) => `<button type="button" class="btn sm" data-retention='${JSON.stringify(p)}'>${l}</button>`).join('')}</div>
      <div class="section"><div class="gfs">${g.map(([k, l]) => field(l, inp(job, 'retention.' + k, { type: 'number', min: 0 }))).join('')}</div>
      ${chk(job, 'pruneAfter', 'Speicher danach automatisch freigeben (Prune)', 'Daten, die von keinem Snapshot mehr gebraucht werden, werden gelöscht')}</div>
      <div class="preview-box">${icon('info', 'sm')} Beispiel «Standard»: die letzten 3 Läufe, 7 Tagesstände, 4 Wochenstände, 12 Monatsstände und 3 Jahresstände – total maximal ~29 Snapshots, dank Deduplikation meist nur wenig mehr Speicher als ein Vollbackup.</div>`;
    },
    Optionen: () => `<h2>Optionen</h2><p class="lead">Leistung, Zuverlässigkeit und Skripte.</p>
      <div class="section"><h4>${icon('cpu', 'sm')}Leistung</h4><div class="row r4">
        ${field('Kompression', sel(job, 'options.compression', [['auto', 'Automatisch'], ['fast', 'Schnell'], ['max', 'Maximal'], ['off', 'Aus']]))}
        ${field('Parallele Uploads', inp(job, 'options.workers', { type: 'number', min: 1, max: 32 }))}
        ${field('Bandbreite (KB/s)', inp(job, 'options.bandwidthKBs', { type: 'number', min: 0, ph: '0 = unbegrenzt' }))}
        ${field('Priorität', sel(job, 'options.priority', [['normal', 'Normal'], ['low', 'Niedrig'], ['idle', 'Nur im Leerlauf']]))}</div></div>
      <div class="section"><h4>${icon('refresh', 'sm')}Zuverlässigkeit</h4><div class="row r4">
        ${field('Wiederholungen', inp(job, 'options.retries', { type: 'number', min: 0, max: 10 }))}${field('Pause dazwischen (min)', inp(job, 'options.retryDelayMin', { type: 'number', min: 1 }))}${field('Zeitlimit (min)', inp(job, 'options.timeoutMin', { type: 'number', min: 0, ph: '0 = keins' }))}
        ${field('Prüfung nach Backup', sel(job, 'options.verifyAfter', [['none', 'Keine'], ['quick', 'Stichprobe'], ['full', 'Vollständig (100 %)']]))}</div>
        <div class="row r4">${field('Stichprobe (% der Daten)', inp(job, 'options.verifyPercent', { type: 'number', min: 1, max: 100 }))}${field('Tags', inp(job, 'options.tags', { data: 'csv', ph: 'produktion, kunde-a' }))}</div></div>
      <div class="section"><h4>${icon('term', 'sm')}Skripte (Pre/Post-Hooks)</h4>
        ${field('Vor dem Backup', inp(job, 'hooks.pre', { ph: 'systemctl stop myapp   ·   Stop-Service MyApp' }))}
        ${field('Nach dem Backup', inp(job, 'hooks.post', { ph: 'systemctl start myapp' }), 'Läuft immer, auch bei Fehlern. Variablen: SNB_JOB_NAME, SNB_STATUS')}
        ${field('Bei Fehler', inp(job, 'hooks.onFailure', { ph: '/usr/local/bin/alarm.sh' }))}
        <div class="row">${chk(job, 'hooks.abortOnPre', 'Abbrechen, wenn Pre-Hook fehlschlägt')}${field('Hook-Zeitlimit (min)', inp(job, 'hooks.timeoutMin', { type: 'number', min: 1 }))}</div></div>`,
    Benachrichtigung: () => `<h2>Benachrichtigungen</h2><p class="lead">E-Mail (SMTP in den Einstellungen) und Webhooks für Slack, Microsoft Teams, Discord, ntfy, Gotify oder eigene Systeme.</p>
      <div class="section"><h4>${icon('bell', 'sm')}Wann benachrichtigen?</h4><div class="row r3">${chk(job, 'notify.onFailure', 'Bei Fehler')}${chk(job, 'notify.onWarning', 'Bei Warnung')}${chk(job, 'notify.onSuccess', 'Bei Erfolg')}</div></div>
      <div class="section">${field('E-Mail-Empfänger (eine pro Zeile)', area(job, 'notify.emails', 'it@firma.ch', 3), 'Zusätzlich zu den Standard-Empfängern aus den Einstellungen')}${field('Webhook-URLs', area(job, 'notify.webhooks', 'https://hooks.slack.com/services/…\nhttps://ntfy.sh/mein-thema', 3))}</div>`,
    Zusammenfassung: () => {
      const t = TYPES[typeKey];
      const r = repos.find((x) => x.repo.id === job.repoId);
      const rt = job.retention || {};
      return `<h2>Fast fertig</h2><p class="lead">Gib dem Job einen Namen und prüfe die Einstellungen.</p>
      <div class="section"><div class="row">${field('Name', inp(job, 'name', { ph: 'z. B. Webserver täglich', attrs: ' required maxlength="80"' }))}${field('Beschreibung', inp(job, 'description', { ph: 'optional' }))}</div>${chk(job, 'enabled', 'Job aktiv', 'Pausierte Jobs laufen nur manuell')}</div>
      <div class="grid g2"><div class="card pad"><dl class="summary"><dt>Typ</dt><dd>${t.label}</dd><dt>Quelle</dt><dd>${esc((job.source.paths || []).join(', ') || (job.source.databases || []).join(', ') || job.source.command || (job.source.allDatabases ? 'Alle Datenbanken' : '–'))}</dd><dt>Ziel</dt><dd>${esc(r ? r.repo.name : job.targetPath || '–')}</dd></dl></div>
      <div class="card pad"><dl class="summary"><dt>Zeitplan</dt><dd id="sumsched">…</dd><dt>Aufbewahrung</dt><dd>${['keepLast', 'keepDaily', 'keepWeekly', 'keepMonthly', 'keepYearly'].filter((k) => rt[k]).map((k) => rt[k] + ' ' + { keepLast: 'letzte', keepDaily: 'tägl.', keepWeekly: 'wöch.', keepMonthly: 'monatl.', keepYearly: 'jährl.' }[k]).join(' · ') || 'unbegrenzt'}</dd><dt>Sicherheit</dt><dd>AES-256-GCM · ${job.options.verifyAfter === 'none' ? 'ohne Prüfung' : 'mit Integritätsprüfung'}</dd></dl></div></div>`;
    },
  };

  const schedPreview = async () => {
    const box = $('#schedprev', wiz) || $('#sumsched', wiz);
    if (!box) return;
    try {
      const r = await POST('/api/schedule/preview', job.schedule);
      if ($('#sumsched', wiz)) { $('#sumsched', wiz).textContent = r.text; return; }
      box.innerHTML = r.error ? `<span style="color:var(--err)">${icon('alert', 'sm')} ${esc(r.error)}</span>` : `<b>${icon('cal', 'sm')} ${esc(r.text)}</b> – nächste Läufe:<ul>${(r.times || []).map((t) => `<li>${WDL[new Date(t).getDay()]}, ${dt(t, true)} <span class="muted">(${rel(t)})</span></li>`).join('') || '<li>keine</li>'}</ul>`;
    } catch (e) { box.textContent = e.message; }
  };

  const render = () => {
    const name = STEPS[step];
    wiz.innerHTML = `<div class="steps">${STEPS.map((s, i) => skip(i) ? '' : `<div class="step ${i === step ? 'on' : i < step ? 'done' : ''}" data-step="${i}"><span class="n">${i < step ? '✓' : i + 1}</span>${s}</div>`).join('')}</div><div class="wiz-body"><form id="wb">${views[name]()}</form></div>`;
    foot.innerHTML = `<button class="btn" data-close>Abbrechen</button><div style="flex:1"></div>${step > 0 ? `<button class="btn" id="wprev">${icon('chev', 'sm')}Zurück</button>` : ''}${step < STEPS.length - 1 ? `<button class="btn primary" id="wnext">Weiter</button>` : `<button class="btn primary" id="wsave">${icon('check', 'sm')}${id ? 'Speichern' : 'Job erstellen'}</button>`}`;
    if (step > 0) $('#wprev', foot).querySelector('svg').style.transform = 'rotate(180deg)';
    $$('.type', wiz).forEach((el) => el.onclick = () => { typeKey = el.dataset.type; const t = TYPES[typeKey]; job.type = t.t; if (t.k) job.source.kind = t.k; render(); });
    $$('[data-step]', wiz).forEach((el) => el.onclick = () => { collect(); step = +el.dataset.step; render(); });
    $$('[data-rerender]', wiz).forEach((el) => el.onchange = () => { collect(); render(); });
    $$('[data-fill]', wiz).forEach((el) => el.onclick = () => { const f = $(`[name="${el.dataset.fill}"]`, wiz); f.value = el.dataset.val; f.dispatchEvent(new Event('input')); });
    $$('[data-retention]', wiz).forEach((el) => el.onclick = () => { const p = JSON.parse(el.dataset.retention); ['keepLast', 'keepHourly', 'keepDaily', 'keepWeekly', 'keepMonthly', 'keepYearly', 'keepWithinDays'].forEach((k) => { $(`[name="retention.${k}"]`, wiz).value = p[k] || 0; }); });
    $('#wb', wiz).addEventListener('input', () => { if (STEPS[step] === 'Zeitplan') { collect(); clearTimeout(render.t); render.t = setTimeout(schedPreview, 300); } });
    if (STEPS[step] === 'Zeitplan' || STEPS[step] === 'Zusammenfassung') schedPreview();
    const nx = $('#wnext', foot);
    if (nx) nx.onclick = () => { collect(); do { step++; } while (skip(step)); render(); };
    const pv = $('#wprev', foot);
    if (pv) pv.onclick = () => { collect(); do { step--; } while (step > 0 && skip(step)); render(); };
    const sv = $('#wsave', foot);
    if (sv) sv.onclick = async () => {
      collect();
      if (!job.name) { toast('Bitte einen Namen eingeben', 'err'); return; }
      sv.disabled = true;
      try { await (id ? PUT('/api/jobs/' + id, job) : POST('/api/jobs', job)); toast(id ? 'Job gespeichert' : 'Job erstellt'); m.close(); route(); }
      catch (e) { fail(e); sv.disabled = false; }
    };
  };
  render();
}

/* ------------------------------------------------------------------ schedule */
async function pgSchedule(args) {
  setTitle('Zeitplan', 'Geplante Läufe der nächsten 7 Tage');
  let offset = 0;
  const render = async () => {
    const start = new Date(); start.setHours(0, 0, 0, 0); start.setDate(start.getDate() + offset);
    const from = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())}`;
    const [evs, jobs] = await Promise.all([GET('/api/schedule?days=7&from=' + from), GET('/api/jobs')]);
    const H = 720;
    const days = [...Array(7).keys()].map((i) => { const d = new Date(start); d.setDate(d.getDate() + i); return d; });
    const today = new Date().toDateString();
    let cols = '';
    days.forEach((d) => {
      const list = (evs || []).filter((e) => new Date(e.time).toDateString() === d.toDateString());
      const isT = d.toDateString() === today;
      let inner = '';
      const used = {};
      list.forEach((e) => {
        const t = new Date(e.time); const mins = t.getHours() * 60 + t.getMinutes();
        let top = (mins / 1440) * H; const slot = Math.floor(top / 24); used[slot] = (used[slot] || 0) + 1;
        if (used[slot] > 1) top += (used[slot] - 1) * 24;
        inner += `<div class="ev t-${e.type}" style="top:${top}px" data-act="edit-job" data-id="${e.jobId}" title="${esc(e.name)} – ${pad(t.getHours())}:${pad(t.getMinutes())}"><small>${pad(t.getHours())}:${pad(t.getMinutes())}</small>${esc(e.name)}</div>`;
      });
      if (isT) { const n = new Date(); inner += `<div class="now" style="top:${((n.getHours() * 60 + n.getMinutes()) / 1440) * H}px"></div>`; }
      cols += `<div class="col ${isT ? 'today' : ''}">${inner}</div>`;
    });
    const hd = days.map((d) => `<div class="hd ${d.toDateString() === today ? 'today' : ''}"><b>${WDL[d.getDay()]}</b><span>${pad(d.getDate())}.${pad(d.getMonth() + 1)}. · ${(evs || []).filter((e) => new Date(e.time).toDateString() === d.toDateString()).length} Läufe</span></div>`).join('');
    const axis = [...Array(25).keys()].filter((h) => h % 2 === 0).map((h) => `<div style="top:${(h / 24) * H}px">${pad(h)}:00</div>`).join('');
    const manual = jobs.filter((j) => j.schedule.kind === 'manual' || j.schedule.kind === 'after' || !j.enabled);
    page().innerHTML = `<div class="toolbar"><div class="seg"><button data-o="-7">← Woche</button><button data-o="0" class="${offset === 0 ? 'on' : ''}">Heute</button><button data-o="7">Woche →</button></div>
      <b style="margin-left:6px">${dt(days[0], true).split(' ')[0]} – ${dt(days[6], true).split(' ')[0]}</b><div class="grow"></div>
      <div class="legend">${['backup', 'mirror', 'archive', 'copy', 'verify', 'prune'].map((t) => `<span><i class="ev t-${t}" style="position:static;display:inline-block;width:10px;height:10px;min-height:0;padding:0;border:0;box-shadow:none"></i>${(TYPES[t] || TYPES.files).label.replace(' (3-2-1)', '').replace(' (Sync)', '')}</span>`).join('')}</div></div>
      <div class="cal"><div class="hd" style="border-left:0"></div>${hd}<div class="axis">${axis}</div>${cols}</div>
      ${manual.length ? `<div class="card mt"><div class="card-h"><h3>Ohne festen Zeitplan</h3><span class="sub">manuell, verkettet oder pausiert</span></div>${manual.map((j) => `<div class="list-item"><div class="typeicon">${icon(typeOf(j).icon)}</div><div class="grow"><b>${esc(j.name)}</b><span>${esc(j.scheduleText)}${j.enabled ? '' : ' · pausiert'}</span></div><button class="btn sm" data-act="run" data-id="${j.id}">${icon('play', 'sm')}Start</button></div>`).join('')}</div>` : ''}`;
    $$('[data-o]', page()).forEach((b) => b.onclick = () => { const o = +b.dataset.o; offset = o === 0 ? 0 : offset + o; render(); });
  };
  await render();
  every(60000, () => render().catch(() => {}));
}

/* ------------------------------------------------------------------ targets */
async function pgTargets() {
  setTitle('Backup-Ziele', 'Verschlüsselte Repositories auf Disk, NAS und Cloud');
  $('#topact').innerHTML = `<button class="btn primary" id="addrepo">${icon('plus')}Ziel hinzufügen</button>`;
  $('#addrepo').onclick = () => openRepoDialog();
  const list = await GET('/api/repos');
  if (!list || !list.length) {
    page().innerHTML = `<div class="card"><div class="empty"><div class="ill">${icon('db', 'lg')}</div><h3>Noch kein Backup-Ziel</h3><p>Ein Ziel ist ein verschlüsseltes Repository – lokal, auf dem NAS oder in der Cloud.</p><button class="btn primary" id="add2">${icon('plus')}Ziel hinzufügen</button></div></div>
    <div class="grid g4 mt">${[['hdd', 'Lokal / NAS', 'Interne Disk, USB, NFS, SMB, iSCSI'], ['cloud', 'S3-kompatibel', 'AWS, Wasabi, MinIO, Backblaze, R2, Infomaniak, Exoscale'], ['globe', 'WebDAV', 'Nextcloud, kDrive, Synology'], ['link', 'rclone', 'SFTP, OneDrive, Google Drive, Azure, Dropbox, 70+']].map(([i, t, d]) => `<div class="card pad"><div class="typeicon mb">${icon(i)}</div><b>${t}</b><div class="muted" style="font-size:12.5px">${d}</div></div>`).join('')}</div>`;
    $('#add2').onclick = () => openRepoDialog();
    return;
  }
  page().innerHTML = `<div class="grid g3">${list.map(({ repo: r, usage: u = {}, location }) => {
    const [ic, kl] = BK[r.backend.kind] || BK.local;
    const ratio = u.bytes ? u.logical / u.bytes : 0;
    const lock = r.backend.kind === 's3' && r.backend.s3?.objectLockMode;
    return `<div class="card repo-card"><div class="hd"><div class="typeicon">${icon(ic, 'lg')}</div><div style="min-width:0;flex:1"><b>${esc(r.name)}</b><span>${esc(location)}</span></div>${u.error ? '<span class="badge err">Fehler</span>' : '<span class="badge ok"><span class="d"></span>Online</span>'}</div>
      ${u.error ? `<div class="alert err" style="margin:0">${icon('alert', 'sm')}${esc(u.error)}</div>` : ''}
      <div class="stats"><div class="stat"><span>Belegt</span><b>${bytes(u.bytes)}</b></div><div class="stat"><span>Snapshots</span><b>${u.snapshots || 0}</b></div><div class="stat"><span>Ersparnis</span><b>${ratio > 1 ? ratio.toFixed(1) + '×' : '–'}</b></div></div>
      <div class="feat"><span class="badge brand">${icon('lock', 'sm')}AES-256</span><span class="badge">${kl}</span>${lock ? `<span class="badge ok">${icon('shield', 'sm')}Object Lock ${r.backend.s3.objectLockDays} T.</span>` : ''}${u.latest ? `<span class="badge">Letztes: ${rel(u.latest)}</span>` : ''}</div>
      <div class="flex" style="flex-wrap:wrap"><a class="btn sm" href="#/restore/${r.id}">${icon('restore', 'sm')}Durchsuchen</a><button class="btn sm" data-rv="${r.id}">${icon('shield', 'sm')}Prüfen</button><button class="btn sm" data-rr="${r.id}">${icon('refresh', 'sm')}</button>
      <div style="flex:1"></div><button class="btn sm icon" data-re="${r.id}" title="Bearbeiten">${icon('edit', 'sm')}</button><button class="btn sm icon" data-rm="${r.id}" title="Mehr">${icon('settings', 'sm')}</button></div></div>`;
  }).join('')}</div>
  <div class="alert info mt">${icon('info')}<div>Das Verschlüsselungspasswort eines Ziels ist der einzige Schlüssel zu den Daten. Bewahre es zusätzlich ausserhalb dieses Servers auf (Passwort-Manager, Tresor). Ohne Passwort ist keine Wiederherstellung möglich – auch nicht durch SWISSNAND.</div></div>`;
  $$('[data-rv]').forEach((b) => b.onclick = async () => { await POST(`/api/repos/${b.dataset.rv}/verify`, { percent: 10 }); toast('Prüfung gestartet – siehe Aktivität'); });
  $$('[data-rr]').forEach((b) => b.onclick = async () => { b.disabled = true; try { await POST(`/api/repos/${b.dataset.rr}/refresh`); route(); } catch (e) { fail(e); b.disabled = false; } });
  $$('[data-re]').forEach((b) => b.onclick = () => openRepoDialog(list.find((x) => x.repo.id === b.dataset.re).repo));
  $$('[data-rm]').forEach((b) => b.onclick = () => repoMore(list.find((x) => x.repo.id === b.dataset.rm).repo));
}

function repoMore(r) {
  const m = modal({
    title: r.name, icon: 'db', body: `<div class="list-item"><div class="grow"><b>Integrität vollständig prüfen</b><span>Alle Daten lesen, entschlüsseln und Prüfsummen kontrollieren</span></div><button class="btn sm" id="m1">Starten</button></div>
    <div class="list-item"><div class="grow"><b>Speicher bereinigen (Prune)</b><span>Nicht mehr referenzierte Daten löschen</span></div><button class="btn sm" id="m2">Starten</button></div>
    <div class="list-item"><div class="grow"><b>Sperren entfernen</b><span>Nach Absturz hängende Sperren lösen</span></div><button class="btn sm" id="m3">Entsperren</button></div>
    <div class="list-item"><div class="grow"><b>Ziel aus Konfiguration entfernen</b><span>Die gesicherten Daten bleiben am Speicherort erhalten</span></div><button class="btn sm danger" id="m4">Entfernen</button></div>`,
  });
  $('#m1', m.el).onclick = async () => { await POST(`/api/repos/${r.id}/verify`, { percent: 100 }).catch(fail); toast('Vollprüfung gestartet'); m.close(); };
  $('#m2', m.el).onclick = async () => { await POST(`/api/repos/${r.id}/prune`).catch(fail); toast('Bereinigung gestartet'); m.close(); };
  $('#m3', m.el).onclick = async () => { try { const x = await POST(`/api/repos/${r.id}/unlock`); toast(x.removed + ' Sperren entfernt'); } catch (e) { fail(e); } m.close(); };
  $('#m4', m.el).onclick = async () => { m.close(); if (await confirmDlg(`«${esc(r.name)}» aus der Konfiguration entfernen?`, 'Entfernen', true)) { try { await DEL('/api/repos/' + r.id); toast('Ziel entfernt'); route(); } catch (e) { fail(e); } } };
}

function openRepoDialog(existing) {
  const r = existing || { name: '', backend: { kind: 'local', s3: { region: 'eu-central-1' } } };
  let kind = r.backend.kind || 'local';
  let mode = existing ? 'connect' : 'init';
  const m = modal({ title: existing ? 'Ziel bearbeiten' : 'Backup-Ziel hinzufügen', icon: 'db', size: 'wide', body: '<div id="rb"></div>', foot: `<button class="btn" data-close>Abbrechen</button><div style="flex:1"></div><button class="btn" id="rtest">${icon('zap', 'sm')}Verbindung testen</button><button class="btn primary" id="rsave">${icon('check', 'sm')}${existing ? 'Speichern' : 'Hinzufügen'}</button>` });
  const draw = () => {
    const b = r.backend;
    $('#rb', m.el).innerHTML = `<form id="rf">
      <div class="types mb" style="grid-template-columns:repeat(4,1fr)">${Object.entries(BK).map(([k, [i, l]]) => `<div class="type ${kind === k ? 'on' : ''}" data-kind="${k}" style="padding:14px"><div class="typeicon" style="margin-bottom:8px">${icon(i)}</div><b>${l}</b></div>`).join('')}</div>
      <div class="row">${field('Name', inp(r, 'name', { ph: 'z. B. NAS Keller, Wasabi Offsite' }))}${field('Beschreibung', inp(r, 'description', { ph: 'optional' }))}</div>
      ${kind === 'local' ? `<div class="section">${field('Pfad', inp(r, 'backend.path', { ph: '/mnt/backup/repo   ·   E:\\SNBackup   ·   \\\\nas\\backup\\server1' }), 'Interne/externe Disk oder gemountete NFS/SMB/iSCSI-Freigabe. Tipp: «Hardened Repository» – Ziel auf separatem Linux-Server mit eigenem Benutzer und Object Lock.')}</div>` : ''}
      ${kind === 's3' ? `<div class="section"><div class="row">${field('Endpoint', inp(r, 'backend.s3.endpoint', { ph: 'https://s3.eu-central-1.wasabisys.com' }), 'AWS, Wasabi, MinIO, Backblaze B2, Cloudflare R2, Exoscale, Infomaniak Swiss Backup')}${field('Region', inp(r, 'backend.s3.region', { ph: 'eu-central-1' }))}</div>
        <div class="row">${field('Bucket', inp(r, 'backend.s3.bucket'))}${field('Präfix (Ordner)', inp(r, 'backend.s3.prefix', { ph: 'server1' }))}</div>
        <div class="row">${field('Access Key', inp(r, 'backend.s3.accessKey'))}${field('Secret Key', `<input type="password" name="backend.s3.secretKey" placeholder="${existing ? 'unverändert lassen' : ''}" autocomplete="new-password">`)}</div>
        <div class="row">${field('Speicherklasse', sel(r, 'backend.s3.storageClass', [['', 'Standard'], ['STANDARD_IA', 'Infrequent Access'], ['GLACIER_IR', 'Glacier Instant Retrieval'], ['ONEZONE_IA', 'One Zone IA']]))}${chk(r, 'backend.s3.pathStyle', 'Path-Style-Adressierung', 'Für MinIO und manche S3-Anbieter')}</div></div>
        <div class="section"><h4>${icon('shield', 'sm')}Unveränderbarkeit (Object Lock / WORM)</h4><div class="row">${field('Modus', sel(r, 'backend.s3.objectLockMode', [['', 'Aus'], ['GOVERNANCE', 'Governance'], ['COMPLIANCE', 'Compliance (unlöschbar)']]), 'Bucket muss mit aktiviertem Object Lock erstellt worden sein')}${field('Sperrdauer (Tage)', inp(r, 'backend.s3.objectLockDays', { type: 'number', min: 0, ph: '30' }), 'Schützt Backups vor Ransomware und versehentlichem Löschen')}</div></div>` : ''}
      ${kind === 'webdav' ? `<div class="section">${field('URL', inp(r, 'backend.webdavUrl', { ph: 'https://cloud.firma.ch/remote.php/dav/files/backup/server1' }))}<div class="row">${field('Benutzer', inp(r, 'backend.webdavUser'))}${field('Passwort / App-Token', `<input type="password" name="backend.webdavPassword" placeholder="${existing ? 'unverändert lassen' : ''}" autocomplete="new-password">`)}</div></div>` : ''}
      ${kind === 'rclone' ? `<div class="section">${field('rclone-Remote', inp(r, 'backend.rcloneRemote', { ph: 'sftp-nas:backups/server1   ·   onedrive:Backup' }), 'Remote zuerst mit <span class="kbd">rclone config</span> als Dienstbenutzer einrichten. Unterstützt SFTP, FTP, SMB, OneDrive, Google Drive, Dropbox, Azure Blob, GCS, Swift, pCloud …')}</div>` : ''}
      <div class="section"><h4>${icon('zap', 'sm')}Netzwerk</h4><div class="row">${field('Upload-Limit (KB/s)', inp(r, 'backend.bandwidthKBs', { type: 'number', min: 0, ph: '0 = unbegrenzt' }))}<div></div></div></div>
      <div class="section"><h4>${icon('key', 'sm')}Verschlüsselung</h4>
        ${existing ? '' : `<div class="chips mb">${[['init', 'Neues Repository anlegen'], ['connect', 'Bestehendes verbinden']].map(([v, l]) => `<label class="chip"><input type="radio" name="_mode" value="${v}"${mode === v ? ' checked' : ''}><span>${l}</span></label>`).join('')}</div>`}
        <div class="row">${field('Verschlüsselungspasswort', `<input type="password" name="password" autocomplete="new-password" placeholder="${existing ? 'unverändert lassen' : 'mind. 12 Zeichen'}">`, 'AES-256-GCM · Schlüsselableitung PBKDF2-SHA256 (600 000 Iterationen)')}${existing ? '<div></div>' : field('Passwort wiederholen', '<input type="password" name="_pw2" autocomplete="new-password">')}</div>
        ${existing ? '' : '<button type="button" class="btn sm" id="genpw">' + icon('key', 'sm') + 'Starkes Passwort generieren</button>'}</div></form>`;
    $$('[data-kind]', m.el).forEach((el) => el.onclick = () => { Object.assign(r, readForm($('#rf', m.el), r)); kind = el.dataset.kind; r.backend.kind = kind; draw(); });
    $$('[name=_mode]', m.el).forEach((el) => el.onchange = () => { mode = el.value; });
    const g = $('#genpw', m.el);
    if (g) g.onclick = () => {
      const a = new Uint8Array(24); crypto.getRandomValues(a);
      const pw = Array.from(a, (x) => 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_!'[x % 60]).join('');
      $('[name=password]', m.el).value = pw; $('[name=_pw2]', m.el).value = pw; $('[name=password]', m.el).type = 'text';
      toast('Passwort generiert – jetzt sicher notieren!');
    };
  };
  draw();
  const payload = () => {
    const f = readForm($('#rf', m.el), r);
    f.backend.kind = kind;
    return { name: f.name, description: f.description, backend: f.backend, password: f.password, mode, _pw2: f._pw2 };
  };
  $('#rtest', m.el).onclick = async () => {
    try { const x = await POST('/api/repos/test', payload()); toast('Verbindung OK · ' + (x.repositoryExists ? 'bestehendes Repository gefunden' : 'leer, bereit zum Anlegen')); } catch (e) { fail(e); }
  };
  $('#rsave', m.el).onclick = async (e) => {
    const p = payload();
    if (!existing && mode === 'init' && p.password !== p._pw2) { toast('Passwörter stimmen nicht überein', 'err'); return; }
    delete p._pw2;
    e.target.disabled = true;
    try { await (existing ? PUT('/api/repos/' + existing.id, p) : POST('/api/repos', p)); toast(existing ? 'Ziel gespeichert' : 'Ziel hinzugefügt'); m.close(); route(); }
    catch (er) { fail(er); e.target.disabled = false; }
  };
}

/* ------------------------------------------------------------------ restore */
async function pgRestore(args) {
  setTitle('Wiederherstellen', 'Snapshot wählen, Dateien durchsuchen, zurückholen');
  const repos = await GET('/api/repos');
  if (!repos || !repos.length) { page().innerHTML = `<div class="card"><div class="empty"><div class="ill">${icon('restore', 'lg')}</div><h3>Keine Backup-Ziele</h3><a href="#/targets">Ziel hinzufügen</a></div></div>`; return; }
  let repoId = args[0] || repos[0].repo.id;
  let snaps = [];
  let cur = null, path = '', q = '';
  const picked = new Set();
  const draw = async () => {
    page().innerHTML = `<div class="toolbar"><div class="seg">${repos.map((r) => `<button data-repo="${r.repo.id}" class="${r.repo.id === repoId ? 'on' : ''}">${esc(r.repo.name)}</button>`).join('')}</div><div class="grow"></div></div>
    <div class="browser"><div class="card"><div class="card-h"><h3>Snapshots</h3><span class="sub" id="sc"></span></div><div id="slist" style="max-height:640px;overflow:auto"><div class="empty">Lade…</div></div></div>
    <div class="card" id="fb"><div class="empty"><div class="ill">${icon('folder', 'lg')}</div><h3>Snapshot wählen</h3>Links einen Wiederherstellungspunkt auswählen.</div></div></div>`;
    $$('[data-repo]').forEach((b) => b.onclick = () => { repoId = b.dataset.repo; cur = null; path = ''; picked.clear(); location.hash = '#/restore/' + repoId; });
    try { snaps = await GET(`/api/repos/${repoId}/snapshots`); } catch (e) { $('#slist').innerHTML = `<div class="alert err" style="margin:14px">${esc(e.message)}</div>`; return; }
    $('#sc').textContent = snaps.length + ' Wiederherstellungspunkte';
    const groups = {};
    snaps.forEach((s) => { const k = s.jobName || s.kind; (groups[k] = groups[k] || []).push(s); });
    $('#slist').innerHTML = snaps.length ? Object.entries(groups).map(([g, list]) => `<div style="padding:10px 16px 4px;font-size:11.5px;text-transform:uppercase;letter-spacing:.08em;color:var(--text-3)">${esc(g)}</div>` + list.map((s) => `<div class="snap ${cur && cur.id === s.id ? 'on' : ''}" data-snap="${s.id}"><span class="tl"></span><div class="grow"><b>${WD[new Date(s.time).getDay()]}, ${dt(s.time, true)}</b><span>${s.stats.files} Dateien · ${bytes(s.stats.bytes)} · ${s.host}${s.origin ? ' · Kopie' : ''}</span></div>${s.mode === 'full' ? '<span class="badge brand">Voll</span>' : ''}</div>`).join('')).join('') : `<div class="empty">Noch keine Snapshots</div>`;
    $$('[data-snap]').forEach((el) => el.onclick = () => { cur = snaps.find((s) => s.id === el.dataset.snap); path = ''; q = ''; picked.clear(); $$('.snap').forEach((x) => x.classList.toggle('on', x === el)); browse(); });
    if (cur) browse();
  };
  const browse = async () => {
    const fb = $('#fb');
    fb.innerHTML = `<div class="empty">Lade Dateiliste…</div>`;
    let r;
    try { r = await GET(`/api/repos/${repoId}/snapshots/${cur.id}/ls?path=${encodeURIComponent(path)}&q=${encodeURIComponent(q)}`); } catch (e) { fb.innerHTML = `<div class="alert err" style="margin:14px">${esc(e.message)}</div>`; return; }
    const parts = path ? path.split('/') : [];
    const crumbs = `<a data-path="">${icon('hdd', 'sm')}</a>` + parts.map((p, i) => `${icon('chev', 'sm')}<a data-path="${esc(parts.slice(0, i + 1).join('/'))}">${esc(p)}</a>`).join('');
    fb.innerHTML = `<div class="card-h" style="flex-wrap:wrap"><div class="crumbs">${crumbs}</div><div class="r"><div class="search">${icon('search', 'sm')}<input type="text" id="fq" placeholder="Im Snapshot suchen…" value="${esc(q)}" style="width:220px"></div></div></div>
      <div style="max-height:520px;overflow:auto"><table class="t"><thead><tr><th><input type="checkbox" id="pall"></th><th>Name</th><th>Grösse</th><th>Geändert</th><th></th></tr></thead><tbody>
      ${r.entries.length ? r.entries.map((e) => `<tr class="file-row ${e.type === 'd' ? 'dir' : ''}"><td><input type="checkbox" data-pick="${esc(e.path)}"${picked.has(e.path) ? ' checked' : ''}></td>
      <td><div class="flex"><span class="fi">${icon(e.type === 'd' ? 'folder' : e.type === 'l' ? 'link' : 'file', 'sm')}</span>${e.type === 'd' ? `<a class="nm" data-path="${esc(e.path)}">${esc(e.name)}</a>` : `<span>${esc(q ? e.path : e.name)}</span>`}</div></td>
      <td class="muted">${e.type === 'f' ? bytes(e.size) : ''}</td><td class="muted">${e.mtime ? dt(e.mtime, true) : ''}</td>
      <td><div class="actions"><a class="btn sm icon" href="/api/repos/${repoId}/snapshots/${cur.id}/download?path=${encodeURIComponent(e.path)}" title="${e.type === 'd' ? 'Als ZIP herunterladen' : 'Herunterladen'}">${icon('download', 'sm')}</a></div></td></tr>`).join('') : '<tr><td colspan="5" class="muted" style="text-align:center;padding:30px">Leer</td></tr>'}
      </tbody></table></div>
      <div class="modal-f" style="border-radius:0 0 var(--radius) var(--radius)"><span class="muted" id="pc">${picked.size ? picked.size + ' ausgewählt' : 'Nichts ausgewählt = ganzer Snapshot'}</span><div style="flex:1"></div>
      <a class="btn" href="/api/repos/${repoId}/snapshots/${cur.id}/download?path=${encodeURIComponent(path)}">${icon('download', 'sm')}Ordner als ZIP</a><button class="btn primary" id="dorestore">${icon('restore', 'sm')}Wiederherstellen…</button></div>`;
    $$('[data-path]', fb).forEach((a) => a.onclick = () => { path = a.dataset.path; q = ''; browse(); });
    $$('[data-pick]', fb).forEach((c) => c.onchange = () => { c.checked ? picked.add(c.dataset.pick) : picked.delete(c.dataset.pick); $('#pc').textContent = picked.size ? picked.size + ' ausgewählt' : 'Nichts ausgewählt = ganzer Snapshot'; });
    $('#pall', fb).onchange = (e) => $$('[data-pick]', fb).forEach((c) => { c.checked = e.target.checked; c.onchange(); });
    const fq = $('#fq', fb);
    fq.onkeydown = (e) => { if (e.key === 'Enter') { q = fq.value; browse(); } };
    $('#dorestore', fb).onclick = () => restoreDialog(repoId, cur, [...picked]);
  };
  await draw();
}

async function restoreDialog(repoId, snap, include) {
  const isAdmin = S.me.role === 'admin';
  let agents = [];
  try { agents = ((await GET('/api/agents')).agents || []).filter((a) => !a.agent.disabled); } catch (e) { agents = []; }
  const match = agents.find((a) => a.agent.hostname === snap.host);
  const m = modal({
    title: 'Wiederherstellen', icon: 'restore',
    body: `<div class="alert info">${icon('info')}<div>Snapshot vom <b>${dt(snap.time, true)}</b> · ${include.length ? include.length + ' ausgewählte Einträge' : 'kompletter Snapshot (' + snap.stats.files + ' Dateien, ' + bytes(snap.stats.bytes) + ')'}</div></div>
    <form id="rsf">${agents.length ? field('Auf welchem Rechner?', `<select name="agentId"><option value="">Backup-Server</option>${agents.map((a) => `<option value="${a.agent.id}"${match && match.agent.id === a.agent.id ? ' selected' : ''}>${esc(a.agent.name)}${a.online ? '' : ' (offline)'}</option>`).join('')}</select>`, match ? 'Vorausgewählt: der Rechner, von dem das Backup stammt' : '') : ''}<div class="field"><label>Wohin?</label><div class="chips"><label class="chip"><input type="radio" name="where" value="alt" checked><span>Anderer Ordner (sicher)</span></label>${isAdmin ? '<label class="chip"><input type="radio" name="where" value="orig"><span>Originalort</span></label>' : ''}</div></div>
    <div class="field" id="tgt"><label>Zielordner</label><input type="text" name="target" value="${esc(snap.os === 'windows' ? 'C:\\Restore\\' + snap.id.slice(0, 8) : '/tmp/restore-' + snap.id.slice(0, 8))}"><div class="hint">Die ursprüngliche Ordnerstruktur wird darunter angelegt.</div></div>
    <div class="field"><label>Wenn die Datei schon existiert</label><select name="overwrite"><option value="rename">Umbenennen (.restored-1)</option><option value="always">Überschreiben</option><option value="newer">Nur wenn Backup neuer</option><option value="never">Überspringen</option></select></div>
    <label class="check"><input type="checkbox" name="verify" checked><div><b>Prüfsummen kontrollieren</b><span>SHA-256 jeder Datei nach dem Schreiben verifizieren</span></div></label>
    <label class="check"><input type="checkbox" name="restoreOwner"><div><b>Besitzer & Gruppe wiederherstellen</b><span>Linux, Dienst muss als root laufen</span></div></label></form>`,
    foot: `<button class="btn" data-close>Abbrechen</button><button class="btn primary" id="rgo">${icon('restore', 'sm')}Wiederherstellung starten</button>`,
  });
  $$('[name=where]', m.el).forEach((r) => r.onchange = () => $('#tgt', m.el).classList.toggle('hide', r.value === 'orig' && r.checked));
  $('#rgo', m.el).onclick = async () => {
    const f = readForm($('#rsf', m.el));
    if (f.where === 'orig' && !(await confirmDlg('Dateien am Originalort werden je nach Einstellung überschrieben. Fortfahren?', 'Ja, wiederherstellen', true))) return;
    try {
      await POST('/api/restore', { repoId, snapshotId: snap.id, include, target: f.where === 'orig' ? '' : f.target, overwrite: f.overwrite, verify: f.verify, restoreOwner: f.restoreOwner, agentId: f.agentId || '' });
      toast('Wiederherstellung gestartet'); m.close(); location.hash = '#/activity';
    } catch (e) { fail(e); }
  };
}

/* ------------------------------------------------------------------ activity */
async function pgActivity() {
  setTitle('Aktivität', 'Laufende Jobs und Verlauf');
  let status = '';
  const render = async () => {
    const [running, runs] = await Promise.all([GET('/api/activity'), GET('/api/runs?limit=200' + (status ? '&status=' + status : ''))]);
    S.cache.running = running;
    const nb = $('#nb-activity');
    if (nb) { nb.textContent = running.length; nb.classList.toggle('hide', !running.length); }
    page().innerHTML = `${running.length ? `<div class="card mb"><div class="card-h">${icon('zap')}<h3>Läuft gerade</h3></div>${running.map(liveCard).join('')}</div>` : ''}
    <div class="toolbar"><div class="seg">${[['', 'Alle'], ['success', 'Erfolgreich'], ['warning', 'Warnungen'], ['failed', 'Fehler'], ['cancelled', 'Abgebrochen']].map(([v, l]) => `<button data-s="${v}" class="${status === v ? 'on' : ''}">${l}</button>`).join('')}</div></div>
    <div class="card"><table class="t"><thead><tr><th>Job</th><th>Start</th><th>Dauer</th><th>Dateien</th><th>Neu / Übertragen</th><th>Ergebnis</th><th>Status</th></tr></thead><tbody>
    ${(runs || []).filter((r) => r.status !== 'running').map((r) => { const t = TYPES[r.jobType] || (r.jobType === 'restore' ? { icon: 'restore', label: 'Wiederherstellung' } : TYPES.files); return `<tr class="click" data-act="log" data-id="${r.id}" data-title="${esc(r.jobName)}">
      <td><div class="t-name"><div class="typeicon">${icon(t.icon)}</div><div><b>${esc(r.jobName)}</b><span>${esc(r.trigger)}${r.agent ? ' · ' + esc(r.agent) : ''}${r.attempt > 1 ? ' · Versuch ' + r.attempt : ''}</span></div></div></td>
      <td>${dt(r.start, true)}</td><td>${r.end ? dur(new Date(r.end) - new Date(r.start)) : '–'}</td><td>${r.stats ? r.stats.files : '–'}</td>
      <td>${r.stats ? bytes(r.stats.newBytes) + ' / ' + bytes(r.stats.storedBytes) : '–'}</td><td class="muted" style="max-width:340px">${esc(r.message || '')}</td><td>${statusBadge(r.status)}</td></tr>`; }).join('') || '<tr><td colspan="7" class="muted" style="text-align:center;padding:30px">Keine Einträge</td></tr>'}
    </tbody></table></div>`;
    $$('[data-s]').forEach((b) => b.onclick = () => { status = b.dataset.s; render(); });
  };
  await render();
  every(3000, () => render().catch(() => {}));
}


/* ------------------------------------------------------------------ agents */
const OSL = { linux: 'Linux', windows: 'Windows', darwin: 'macOS' };
async function pgAgents() {
  setTitle('Agenten', 'Rechner, die dieser Backup-Server sichert');
  const admin = S.me.role === 'admin';
  $('#topact').innerHTML = admin ? `<button class="btn primary" id="addag">${icon('plus')}Agent hinzufügen</button>` : '';
  if (admin) $('#addag').onclick = () => openEnroll();
  const render = async () => {
    const d = await GET('/api/agents');
    const list = d.agents || [];
    const on = list.filter((a) => a.online).length;
    page().innerHTML = `
    <div class="grid g4 mb">
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('agent')}</span>Agenten</div><div class="val">${list.length}</div><div class="foot">registriert</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('activity')}</span>Online</div><div class="val">${on}<small>/ ${list.length}</small></div><div class="foot">${list.length - on ? `<span class="badge warn">${list.length - on} offline</span>` : '<span class="badge ok"><span class="d"></span>alle erreichbar</span>'}</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('jobs')}</span>Jobs auf Agenten</div><div class="val">${list.reduce((a, x) => a + x.jobs, 0)}</div><div class="foot">${list.reduce((a, x) => a + x.running, 0)} laufen gerade</div></div>
      <div class="card kpi"><div class="lbl"><span class="ic">${icon('lock')}</span>Verbindung</div><div class="val" style="font-size:20px;margin-top:16px">HTTPS, ausgehend</div><div class="foot">Agenten brauchen keine offenen Ports</div></div>
    </div>
    ${list.length ? `<div class="card"><table class="t"><thead><tr><th>Agent</th><th>System</th><th>Version</th><th>IP</th><th>Zuletzt gesehen</th><th>Jobs</th><th>Status</th><th></th></tr></thead><tbody>
    ${list.map(({ agent: a, online, running, jobs }) => `<tr><td><div class="t-name"><div class="typeicon">${icon(a.os === 'windows' ? 'vm' : 'server')}</div><div><b>${esc(a.name)}</b><span>${esc(a.hostname)}</span></div></div></td>
      <td>${OSL[a.os] || esc(a.os)} · ${esc(a.arch)}</td><td class="muted">v${esc(a.version)}</td><td class="mono muted">${esc(a.ip || '')}</td><td class="muted">${a.lastSeen ? rel(a.lastSeen) : 'nie'}</td>
      <td>${jobs}${running ? ` <span class="badge run"><span class="d"></span>${running} aktiv</span>` : ''}</td>
      <td>${a.disabled ? '<span class="badge err">gesperrt</span>' : online ? '<span class="badge ok"><span class="d"></span>Online</span>' : '<span class="badge warn">Offline</span>'}</td>
      <td><div class="actions">${admin ? `<button class="btn sm icon" data-ae="${a.id}" title="Bearbeiten">${icon('edit', 'sm')}</button><button class="btn sm icon danger" data-ad="${a.id}" title="Entfernen">${icon('trash', 'sm')}</button>` : ''}</div></td></tr>`).join('')}
    </tbody></table></div>` : `<div class="card"><div class="empty"><div class="ill">${icon('agent', 'lg')}</div><h3>Noch keine Agenten</h3><p>Installiere den Agent auf den Servern, die du sichern willst. Er meldet sich verschlüsselt beim Backup-Server und holt seine Aufträge ab.</p>${admin ? `<button class="btn primary" id="add2">${icon('plus')}Agent hinzufügen</button>` : ''}</div></div>`}
    <div class="grid g3 mt">
      <div class="card pad"><div class="typeicon mb">${icon('server')}</div><b>Agentenlos</b><p class="muted" style="font-size:13px;margin-bottom:0">Der Backup-Server sichert selbst: gemountete NFS/SMB-Freigaben, Datenbanken übers Netz, Docker-Volumes auf dem eigenen Host.</p></div>
      <div class="card pad"><div class="typeicon mb">${icon('agent')}</div><b>Mit Agent</b><p class="muted" style="font-size:13px;margin-bottom:0">Nötig für lokale Dateien mit Rechten, Windows VSS, Hyper-V, Proxmox, Disk-Images und Bare Metal. Zeitplan, Verlauf und Meldungen bleiben zentral.</p></div>
      <div class="card pad"><div class="typeicon mb">${icon('shield')}</div><b>Sicher verbunden</b><p class="muted" style="font-size:13px;margin-bottom:0">Einmal-Token zur Registrierung, Zertifikats-Pinning, eigenes Geheimnis pro Agent. Agenten lassen sich jederzeit sperren.</p></div>
    </div>`;
    const a2 = $('#add2');
    if (a2) a2.onclick = () => openEnroll();
    $$('[data-ae]').forEach((b) => b.onclick = () => {
      const a = list.find((x) => x.agent.id === b.dataset.ae).agent;
      const m = modal({ title: 'Agent bearbeiten', icon: 'agent', body: `<form id="af">${field('Name', `<input type="text" name="name" value="${esc(a.name)}">`)}${field('Tags', inp(a, 'tags', { data: 'csv', ph: 'produktion, dmz' }))}${chk(a, 'disabled', 'Agent sperren', 'Gesperrte Agenten erhalten keine Aufträge mehr')}</form>`, foot: `<button class="btn" data-close>Abbrechen</button><button class="btn primary" id="as">Speichern</button>` });
      $('#as', m.el).onclick = async () => { try { await PUT('/api/agents/' + a.id, readForm($('#af', m.el))); toast('Agent gespeichert'); m.close(); render(); } catch (e) { fail(e); } };
    });
    $$('[data-ad]').forEach((b) => b.onclick = async () => { if (await confirmDlg('Agent entfernen? Er kann sich danach nicht mehr verbinden.', 'Entfernen', true)) { try { await DEL('/api/agents/' + b.dataset.ad); toast('Agent entfernt'); render(); } catch (e) { fail(e); } } });
  };
  await render();
  every(5000, () => render().catch(() => {}));
}

function openEnroll() {
  const m = modal({ title: 'Agent hinzufügen', icon: 'agent', size: 'wide', body: `<form id="ef"><div class="row">${field('Bezeichnung', '<input type="text" name="name" placeholder="z. B. Webserver, Hyper-V-Host">')}${field('Gültig (Stunden)', '<input type="number" name="hours" data-type="int" value="24" min="1">')}</div>
    ${chk({}, 'reuse', 'Mehrfach verwendbar', 'Für Docker, Kubernetes-DaemonSets oder das Ausrollen auf viele Server (jeder Rechner erhält trotzdem ein eigenes Geheimnis)')}
    ${field('Adresse des Backup-Servers (aus Sicht des Agenten)', `<input type="text" name="server" value="${esc(location.origin)}">`)}</form><div id="eout"></div>`,
    foot: `<button class="btn" data-close>Schliessen</button><button class="btn primary" id="eg">${icon('key', 'sm')}Installationsbefehle erzeugen</button>` });
  $('#eg', m.el).onclick = async () => {
    const f = readForm($('#ef', m.el));
    let r;
    try { r = await POST('/api/agents/enroll-token', { name: f.name, hours: f.hours, reuse: f.reuse }); } catch (e) { fail(e); return; }
    const srv = f.server.replace(/\/$/, '');
    const fp = r.fingerprint || '';
    const fpArg = fp ? ` --fingerprint ${fp}` : '';
    const rel = 'https://github.com/swissnand/snbackup/releases/latest/download';
    const cmds = {
      Linux: `curl -fsSLO ${rel}/snbackup_amd64.deb\nsudo apt install ./snbackup_amd64.deb\nsudo snbackup agent enroll --server ${srv} --token ${r.token}${fpArg}\nsudo snbackup service install --agent`,
      Windows: `# PowerShell als Administrator\nInvoke-WebRequest ${rel}/snbackup-windows-amd64.zip -OutFile snbackup.zip\nExpand-Archive snbackup.zip snbackup; cd snbackup\n.\\install.ps1 -Agent -Server ${srv} -Token ${r.token}${fp ? ' -Fingerprint ' + fp : ''}`,
      Docker: `docker run -d --name snbackup-agent --restart unless-stopped --hostname $(hostname) \\\n  -v /:/host:ro -v snbackup-agent:/var/lib/snbackup-agent \\\n  -e SNB_SERVER=${srv} -e SNB_ENROLL_TOKEN=${r.token} \\\n  ${fp ? `-e SNB_FINGERPRINT=${fp} ` : ''}-e SNB_HOST_ROOT=/host \\\n  ghcr.io/swissnand/snbackup:latest agent run`,
      Kubernetes: `kubectl create namespace snbackup\nkubectl -n snbackup create secret generic snbackup-agent \\\n  --from-literal=server=${srv} --from-literal=token=${r.token} --from-literal=fingerprint=${fp}\nkubectl apply -f https://raw.githubusercontent.com/swissnand/snbackup/main/deploy/kubernetes/agent-daemonset.yaml`,
    };
    let tab = 'Linux';
    const draw = () => {
      $('#eout', m.el).innerHTML = `<div class="alert ${r.fingerprint ? 'ok' : 'warn'}">${icon(r.fingerprint ? 'lock' : 'alert')}<div>${r.fingerprint ? 'Der Agent prüft das Zertifikat des Servers über den Fingerabdruck – keine Warnung, kein Man-in-the-Middle.' : 'Kein Fingerabdruck verfügbar – der Agent prüft das Zertifikat über die Zertifizierungsstellen des Systems.'} Token gültig bis ${dt(r.expires, true)}${f.reuse ? ', mehrfach verwendbar' : ', einmalig'}.</div></div>
      <div class="seg mb">${Object.keys(cmds).map((k) => `<button data-t="${k}" class="${k === tab ? 'on' : ''}">${k}</button>`).join('')}</div>
      <div class="log" id="cmdbox" style="max-height:none">${esc(cmds[tab])}</div><div class="flex mt"><button class="btn sm" id="cp">${icon('copy', 'sm')}Kopieren</button><span class="muted" style="font-size:12.5px">Der Agent erscheint nach wenigen Sekunden in der Liste.</span></div>`;
      $$('[data-t]', m.el).forEach((b) => b.onclick = () => { tab = b.dataset.t; draw(); });
      $('#cp', m.el).onclick = async () => { try { await navigator.clipboard.writeText(cmds[tab]); toast('Kopiert'); } catch (e) { toast('Bitte manuell kopieren', 'err'); } };
    };
    draw();
    $('#eg', m.el).remove();
  };
}

/* ------------------------------------------------------------------ settings */
async function pgSettings(args) {
  const tab = args[0] || (S.me.role === 'admin' ? 'general' : 'security');
  setTitle('Einstellungen', '');
  const tabs = [['general', 'Allgemein', 'settings'], ['security', 'Sicherheit', 'shield'], ['users', 'Benutzer & API', 'user'], ['notify', 'Benachrichtigungen', 'bell'], ['license', 'Lizenz', 'key'], ['audit', 'Audit-Log', 'list'], ['about', 'System', 'info']];
  page().innerHTML = `<div class="tabs">${tabs.map(([k, l, i]) => `<a href="#/settings/${k}" class="${k === tab ? 'on' : ''}">${icon(i, 'sm')}${l}</a>`).join('')}</div><div id="st"></div>`;
  const box = $('#st');
  const admin = S.me.role === 'admin';
  let set = {}, hasPw = false;
  if (admin && ['general', 'security', 'notify'].includes(tab)) { const r = await GET('/api/settings'); set = r.settings; hasPw = r.smtpHasPassword; }
  const saveSettings = async (form) => { const n = readForm(form, set); try { await PUT('/api/settings', n); toast('Einstellungen gespeichert'); set = n; } catch (e) { fail(e); } };

  if (tab === 'general') {
    box.innerHTML = `<form id="sf" class="card pad" style="max-width:820px"><div class="row">${field('Servername', inp(set, 'serverName'), 'Erscheint in Benachrichtigungen')}${field('Sprache', sel(set, 'language', [['de', 'Deutsch (Schweiz)']]))}</div>
      <div class="row">${field('Gleichzeitige Jobs', inp(set, 'maxConcurrentJobs', { type: 'number', min: 1, max: 16 }), 'Weitere Jobs warten in der Warteschlange (Neustart nötig)')}${field('Verlauf aufbewahren (Tage)', inp(set, 'historyDays', { type: 'number', min: 7 }))}</div>
      ${chk(set, 'dailyReport', 'Täglicher Statusbericht per E-Mail', 'Zusammenfassung aller Läufe an die Standard-Empfänger')}
      <button class="btn primary" id="sv">${icon('check', 'sm')}Speichern</button></form>`;
    $('#sv').onclick = () => saveSettings($('#sf'));
  }

  if (tab === 'security') {
    const me = await GET('/api/me');
    box.innerHTML = `<div class="grid g2" style="align-items:start">
      <div class="card"><div class="card-h">${icon('lock')}<h3>Mein Konto</h3><span class="sub">${esc(me.username)} · ${esc(me.role)}</span></div><div class="card-b"><form id="pwf">
        ${field('Aktuelles Passwort', '<input type="password" name="current" autocomplete="current-password">')}${field('Neues Passwort', '<input type="password" name="new" autocomplete="new-password">', 'Mind. 12 Zeichen und 3 Zeichenarten')}
        <button class="btn" id="pwsave">${icon('check', 'sm')}Passwort ändern</button></form>
        <hr style="border:0;border-top:1px solid var(--border);margin:20px 0">
        <div class="between"><div><b>Zwei-Faktor-Authentifizierung</b><div class="muted" style="font-size:12.5px">TOTP mit Authenticator-App (Microsoft, Google, 1Password, Bitwarden …)</div></div>${me.totpEnabled ? '<span class="badge ok"><span class="d"></span>Aktiv</span>' : '<span class="badge warn">Inaktiv</span>'}</div>
        <div id="totpbox" class="mt">${me.totpEnabled ? `<button class="btn danger sm" id="t-off">2FA deaktivieren</button>` : `<button class="btn primary sm" id="t-on">${icon('shield', 'sm')}2FA einrichten</button>`}</div></div></div>
      ${admin ? `<form id="sf" class="card"><div class="card-h">${icon('shield')}<h3>Richtlinien</h3></div><div class="card-b">
        ${chk(set, 'require2FA', '2FA für alle Konten erzwingen', 'Benutzer ohne 2FA müssen sie beim nächsten Login einrichten')}
        ${field('Sitzung läuft ab nach (Minuten Inaktivität)', inp(set, 'sessionMinutes', { type: 'number', min: 5, max: 720 }), 'Maximale Sitzungsdauer: 12 Stunden')}
        ${field('IP-Allowlist für die Web-Konsole', `<textarea name="ipAllowlist" data-type="lines" rows="4" placeholder="192.168.1.0/24\n10.0.0.5">${esc((set.ipAllowlist || []).join('\n'))}</textarea>`, 'Leer = alle erlaubt. localhost ist immer erlaubt.')}
        <button class="btn primary" id="sv">${icon('check', 'sm')}Speichern</button>
        <div class="alert info mt" style="margin-bottom:0">${icon('info')}<div>Eingebaut: TLS 1.2+, HSTS, CSP, CSRF-Schutz, Brute-Force-Sperre (5 Versuche / 15 min), PBKDF2-Passwörter, verschlüsselte Zugangsdaten, Audit-Log.</div></div></div></form>` : ''}</div>`;
    $('#pwsave').onclick = async () => { const f = readForm($('#pwf')); try { await POST('/api/me/password', f); toast('Passwort geändert'); $('#pwf').reset(); } catch (e) { fail(e); } };
    if ($('#sv')) $('#sv').onclick = () => saveSettings($('#sf'));
    const on = $('#t-on');
    if (on) on.onclick = async () => {
      const r = await POST('/api/me/totp/setup');
      $('#totpbox').innerHTML = `<div class="section"><p style="margin-top:0">1. In der Authenticator-App «Schlüssel manuell eingeben» wählen:</p><div class="code-box">${esc(r.secret.replace(/(.{4})/g, '$1 ').trim())}</div>
        <p class="muted" style="font-size:12px">oder Link öffnen: <a href="${esc(r.uri)}">otpauth-Link</a></p><p>2. Angezeigten 6-stelligen Code eingeben:</p>
        <div class="flex mb"><input type="text" id="tcode" maxlength="6" inputmode="numeric" placeholder="123456" style="width:140px"><button class="btn primary" id="tok">Aktivieren</button></div></div>`;
      $('#tok').onclick = async () => { try { await POST('/api/me/totp/enable', { code: $('#tcode').value }); toast('2FA aktiviert'); route(); } catch (e) { fail(e); } };
    };
    const off = $('#t-off');
    if (off) off.onclick = () => {
      const m = modal({ title: '2FA deaktivieren', body: field('Passwort zur Bestätigung', '<input type="password" id="tpw">'), foot: `<button class="btn" data-close>Abbrechen</button><button class="btn danger" id="tgo">Deaktivieren</button>` });
      $('#tgo', m.el).onclick = async () => { try { await POST('/api/me/totp/disable', { password: $('#tpw', m.el).value }); toast('2FA deaktiviert'); m.close(); route(); } catch (e) { fail(e); } };
    };
  }

  if (tab === 'users') {
    if (!admin) { box.innerHTML = '<div class="alert warn">Nur für Administratoren.</div>'; return; }
    const [users, tokens] = await Promise.all([GET('/api/users'), GET('/api/tokens')]);
    const roleL = { admin: 'Administrator', operator: 'Operator', viewer: 'Nur lesen' };
    box.innerHTML = `<div class="card mb"><div class="card-h">${icon('user')}<h3>Benutzer</h3><span class="sub">Admin: alles · Operator: Jobs starten & wiederherstellen · Nur lesen: Einsicht</span><div class="r"><button class="btn primary sm" id="uadd">${icon('plus', 'sm')}Benutzer</button></div></div>
      <table class="t"><thead><tr><th>Benutzer</th><th>Rolle</th><th>2FA</th><th>Letzte Anmeldung</th><th>Status</th><th></th></tr></thead><tbody>
      ${users.map((u) => `<tr><td><div class="t-name"><div class="typeicon">${icon('user')}</div><b>${esc(u.username)}</b></div></td><td>${roleL[u.role]}</td><td>${u.totpEnabled ? '<span class="badge ok">aktiv</span>' : '<span class="badge">aus</span>'}</td><td class="muted">${u.lastLogin ? dt(u.lastLogin, true) : 'nie'}</td><td>${u.disabled ? '<span class="badge err">gesperrt</span>' : u.mustChange ? '<span class="badge warn">PW-Wechsel</span>' : '<span class="badge ok">aktiv</span>'}</td>
      <td><div class="actions"><button class="btn sm icon" data-ue="${esc(u.username)}">${icon('edit', 'sm')}</button>${u.username !== S.me.username ? `<button class="btn sm icon danger" data-ud="${esc(u.username)}">${icon('trash', 'sm')}</button>` : ''}</div></td></tr>`).join('')}</tbody></table></div>
      <div class="card"><div class="card-h">${icon('key')}<h3>API-Tokens</h3><span class="sub">Für Monitoring (Zabbix, PRTG, Checkmk) und Automatisierung</span><div class="r"><button class="btn sm" id="tadd">${icon('plus', 'sm')}Token</button></div></div>
      <table class="t"><thead><tr><th>Name</th><th>Rolle</th><th>Erstellt</th><th>Zuletzt benutzt</th><th>Läuft ab</th><th></th></tr></thead><tbody>
      ${(tokens || []).map((t) => `<tr><td><b>${esc(t.name)}</b></td><td>${roleL[t.role]}</td><td class="muted">${dt(t.created, true)}</td><td class="muted">${t.lastUsed ? rel(t.lastUsed) : 'nie'}</td><td class="muted">${t.expires ? dt(t.expires, true) : 'nie'}</td><td><div class="actions"><button class="btn sm icon danger" data-td="${t.id}">${icon('trash', 'sm')}</button></div></td></tr>`).join('') || '<tr><td colspan="6" class="muted">Keine Tokens</td></tr>'}</tbody></table>
      <div class="card-b"><span class="muted" style="font-size:12.5px">Beispiel: <span class="kbd">curl -H "Authorization: Bearer snb_…" https://server:8550/api/dashboard</span></span></div></div>`;
    const userDlg = (u) => {
      const m = modal({ title: u ? 'Benutzer bearbeiten' : 'Neuer Benutzer', icon: 'user', body: `<form id="uf">${field('Benutzername', `<input type="text" name="username" value="${esc(u?.username || '')}"${u ? ' readonly' : ''}>`)}${field('Rolle', sel(u || { role: 'operator' }, 'role', [['admin', 'Administrator'], ['operator', 'Operator'], ['viewer', 'Nur lesen']]))}${field(u ? 'Neues Passwort (optional)' : 'Startpasswort', '<input type="password" name="password" autocomplete="new-password">', 'Muss bei der ersten Anmeldung geändert werden')}${chk(u || {}, 'disabled', 'Konto sperren')}${u ? chk({}, 'reset2fa', '2FA zurücksetzen') : ''}</form>`, foot: `<button class="btn" data-close>Abbrechen</button><button class="btn primary" id="us">Speichern</button>` });
      $('#us', m.el).onclick = async () => { try { await POST('/api/users', readForm($('#uf', m.el))); toast('Benutzer gespeichert'); m.close(); route(); } catch (e) { fail(e); } };
    };
    $('#uadd').onclick = () => userDlg(null);
    $$('[data-ue]').forEach((b) => b.onclick = () => userDlg(users.find((u) => u.username === b.dataset.ue)));
    $$('[data-ud]').forEach((b) => b.onclick = async () => { if (await confirmDlg(`Benutzer «${esc(b.dataset.ud)}» löschen?`, 'Löschen', true)) { try { await DEL('/api/users/' + encodeURIComponent(b.dataset.ud)); route(); } catch (e) { fail(e); } } });
    $$('[data-td]').forEach((b) => b.onclick = async () => { if (await confirmDlg('Token widerrufen?', 'Widerrufen', true)) { await DEL('/api/tokens/' + b.dataset.td).catch(fail); route(); } });
    $('#tadd').onclick = () => {
      const m = modal({ title: 'API-Token erstellen', icon: 'key', body: `<form id="tf">${field('Name', '<input type="text" name="name" placeholder="z. B. Zabbix">')}${field('Rolle', sel({ role: 'viewer' }, 'role', [['viewer', 'Nur lesen'], ['operator', 'Operator'], ['admin', 'Administrator']]))}${field('Gültigkeit (Tage)', '<input type="number" name="days" data-type="int" value="365">', '0 = unbegrenzt')}</form>`, foot: `<button class="btn" data-close>Abbrechen</button><button class="btn primary" id="tc">Erstellen</button>` });
      $('#tc', m.el).onclick = async () => {
        try { const r = await POST('/api/tokens', readForm($('#tf', m.el))); $('.modal-b', m.el).innerHTML = `<div class="alert warn">${icon('alert')}<div>Der Token wird nur jetzt angezeigt. Sicher speichern!</div></div><div class="code-box">${esc(r.token)}</div>`; $('#tc', m.el).remove(); }
        catch (e) { fail(e); }
      };
      m.el.addEventListener('click', (e) => { if (e.target.closest('[data-close]')) route(); });
    };
  }

  if (tab === 'notify') {
    box.innerHTML = `<form id="sf" class="grid g2" style="align-items:start">
      <div class="card"><div class="card-h">${icon('bell')}<h3>E-Mail (SMTP)</h3></div><div class="card-b">
        <div class="row">${field('SMTP-Server', inp(set, 'smtp.host', { ph: 'mail.infomaniak.com' }))}${field('Port', inp(set, 'smtp.port', { type: 'number' }))}</div>
        <div class="row">${field('Benutzer', inp(set, 'smtp.user'))}${field('Passwort', `<input type="password" name="smtp.password" placeholder="${hasPw ? '•••••••• (unverändert)' : ''}" autocomplete="new-password">`)}</div>
        <div class="row">${field('Absender', inp(set, 'smtp.from', { ph: 'backup@firma.ch' }))}${field('Verschlüsselung', sel(set, 'smtp.tls', [['starttls', 'STARTTLS (587)'], ['tls', 'TLS (465)'], ['none', 'Keine']]))}</div>
        ${field('Standard-Empfänger', `<textarea name="defaultEmails" data-type="lines" rows="3" placeholder="it@firma.ch">${esc((set.defaultEmails || []).join('\n'))}</textarea>`, 'Erhalten Meldungen aller Jobs')}</div></div>
      <div class="card"><div class="card-h">${icon('link')}<h3>Webhooks</h3></div><div class="card-b">
        ${field('Standard-Webhooks', `<textarea name="defaultWebhooks" data-type="lines" rows="4" placeholder="https://hooks.slack.com/services/…">${esc((set.defaultWebhooks || []).join('\n'))}</textarea>`, 'Format wird automatisch erkannt: Slack, Microsoft Teams, Discord, ntfy, Gotify, sonst generisches JSON')}
        <div class="section"><h4>Test senden</h4><div class="row">${field('An E-Mail', '<input type="email" id="te" placeholder="ich@firma.ch">')}${field('An Webhook', '<input type="url" id="tw" placeholder="https://…">')}</div><button class="btn sm mb" id="tsend">${icon('zap', 'sm')}Testnachricht senden</button></div></div></div>
      <div><button class="btn primary" id="sv">${icon('check', 'sm')}Speichern</button></div></form>`;
    $('#sv').onclick = () => saveSettings($('#sf'));
    $('#tsend').onclick = async () => { try { await POST('/api/settings/test-notify', { email: $('#te').value, webhook: $('#tw').value }); toast('Testnachricht gesendet'); } catch (e) { fail(e); } };
  }

  if (tab === 'license') {
    const l = await GET('/api/license');
    box.innerHTML = `<div class="grid g-main" style="align-items:start"><div><div class="card pad mb"><div class="between"><div><div class="muted">Aktuelle Edition</div><h2 style="font-size:26px;margin-top:4px">${esc(l.edition)}</h2><div class="muted mt" style="margin-top:6px">${esc(l.message)}</div></div><div class="typeicon" style="width:60px;height:60px;border-radius:16px">${icon(l.commercial ? 'shield' : 'user', 'lg')}</div></div>
      ${l.customer ? `<div class="stats mt"><div class="stat"><span>Lizenznehmer</span><b>${esc(l.customer)}</b></div><div class="stat"><span>Server</span><b>${l.servers || 'unbegrenzt'}</b></div><div class="stat"><span>Gültig bis</span><b>${l.expires ? dt(l.expires, true).split(' ')[0] : 'unbefristet'}</b></div></div>` : ''}</div>
      <div class="pricing">
        <div class="plan ${!l.commercial ? 'hl' : ''}"><h4>Community</h4><div class="price">CHF 0</div><div class="muted">Privat & nicht-kommerziell</div><ul><li>Alle Funktionen</li><li>Unbegrenzte Jobs & Daten</li><li>Community-Support</li></ul></div>
        <div class="plan ${l.edition === 'Business' ? 'hl' : ''}"><h4>Business</h4><div class="price">CHF 149<small> / Server / Jahr</small></div><div class="muted">KMU & kommerzielle Nutzung</div><ul><li>Kommerzielle Nutzung</li><li>E-Mail-Support (2 Werktage)</li><li>Updates inklusive</li></ul></div>
        <div class="plan ${l.edition === 'Enterprise' ? 'hl' : ''}"><h4>Enterprise</h4><div class="price">CHF 990<small> / Jahr</small></div><div class="muted">Unbegrenzte Server</div><ul><li>Priorisierter Support</li><li>Onboarding & SLA</li><li>Rechnungsstellung CH</li></ul></div></div>
      <p class="muted" style="font-size:12.5px">Lizenzen: <a href="https://tools.swissnand.com" target="_blank" rel="noopener">tools.swissnand.com</a> · Lizenzschlüssel werden offline geprüft (keine Datenübermittlung).</p></div>
      <div class="card"><div class="card-h">${icon('key')}<h3>Lizenzschlüssel</h3></div><div class="card-b"><textarea id="lk" rows="6" placeholder="SNB1.…"></textarea>${admin ? `<div class="flex mt"><button class="btn primary" id="lsave">${icon('check', 'sm')}Aktivieren</button>${l.commercial ? '<button class="btn danger" id="lrem">Entfernen</button>' : ''}</div>` : ''}</div></div></div>`;
    if ($('#lsave')) $('#lsave').onclick = async () => { try { const r = await PUT('/api/license', { key: $('#lk').value }); toast('Lizenz aktiviert: ' + r.edition); S.me.license = r; route(); } catch (e) { fail(e); } };
    if ($('#lrem')) $('#lrem').onclick = async () => { if (await confirmDlg('Lizenz entfernen? Die Software läuft danach als Community Edition.', 'Entfernen', true)) { const r = await PUT('/api/license', { key: '' }); S.me.license = r; route(); } };
  }

  if (tab === 'audit') {
    if (!admin) { box.innerHTML = '<div class="alert warn">Nur für Administratoren.</div>'; return; }
    const a = await GET('/api/audit?limit=300');
    box.innerHTML = `<div class="card"><table class="t"><thead><tr><th>Zeit</th><th>Benutzer</th><th>IP</th><th>Aktion</th><th>Details</th></tr></thead><tbody>
    ${(a || []).map((e) => `<tr><td class="muted">${dt(e.time, true)}</td><td><b>${esc(e.user)}</b></td><td class="mono muted">${esc(e.ip || '')}</td><td><span class="badge ${e.action.includes('fail') || e.action.includes('blocked') ? 'err' : e.action.includes('delete') ? 'warn' : ''}">${esc(e.action)}</span></td><td class="muted">${esc(e.detail || '')}</td></tr>`).join('') || '<tr><td colspan="5" class="muted">Leer</td></tr>'}</tbody></table></div>`;
  }

  if (tab === 'about') {
    const s = await GET('/api/system');
    box.innerHTML = `<div class="grid g2" style="align-items:start"><div class="card pad"><div class="flex mb">${logo.replace('class="logo"', 'class="logo" style="width:54px;height:54px"')}<div><h2>SWISSNAND Backup</h2><div class="muted">Version ${esc(s.version)} (${esc(s.commit)})</div></div></div>
      <div class="stats" style="grid-template-columns:repeat(2,1fr)"><div class="stat"><span>Server</span><b>${esc(s.hostname)}</b></div><div class="stat"><span>System</span><b>${esc(s.os)} / ${esc(s.arch)}</b></div><div class="stat"><span>CPU-Kerne</span><b>${s.cpus}</b></div><div class="stat"><span>Aktive Sitzungen</span><b>${s.sessions}</b></div></div>
      <div class="field mt"><label>Datenverzeichnis</label><div class="code-box">${esc(s.dataDir)}</div></div></div>
      <div class="card pad"><h3 class="mb">Sicherheitsarchitektur</h3><dl class="summary"><dt>Daten</dt><dd>AES-256-GCM, authentifiziert, Ende-zu-Ende vor dem Upload</dd><dt>Schlüssel</dt><dd>Zufälliger 256-Bit Master-Key, geschützt durch PBKDF2-SHA256 (600 000 Iterationen)</dd><dt>Integrität</dt><dd>HMAC-SHA256 Chunk-IDs, SHA-256 pro Datei, regelmässige Prüfjobs</dd><dt>Ransomware-Schutz</dt><dd>S3 Object Lock (WORM), Backup-Kopien, unveränderliche Snapshots</dd><dt>Konsole</dt><dd>TLS 1.2+, CSP, CSRF, 2FA, RBAC, Brute-Force-Sperre, Audit-Log</dd><dt>Abhängigkeiten</dt><dd>Keine – nur Go-Standardbibliothek, ein einziges Programm</dd></dl></div></div>`;
  }
}

/* ------------------------------------------------------------------ start */
// Inline handlers are blocked by the Content-Security-Policy, so forms are
// never submitted natively; handlers are attached via JavaScript.
document.addEventListener('submit', (e) => e.preventDefault(), true);
boot();
