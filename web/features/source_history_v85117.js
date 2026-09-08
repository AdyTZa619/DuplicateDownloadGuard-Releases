// TEST117 — isolated source-link history.
// Uses only existing public DDG APIs + localStorage. It does not override core UI,
// preview, MEGA, JDownloader or selection functions.
(() => {
  'use strict';

  const NS = 'ddgSourceHistoryV85117';
  const STORE_KEY = 'ddg.sourceHistory.v1';
  const MAX_LINKS = 400;
  const MAX_SNAPSHOTS = 12;
  const TRACKING_KEYS = new Set(['fbclid','gclid','dclid','msclkid','mc_cid','mc_eid']);
  const recordedRevisions = new Map();
  let pending = null;
  let knownRevision = 0;
  let revisionReady = false;
  let inputTimer = 0;

  function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  function canonicalURL(raw) {
    try {
      let value = String(raw || '').trim();
      if (!value) return '';
      const u = new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`);
      u.protocol = u.protocol.toLowerCase();
      u.hostname = u.hostname.toLowerCase().replace(/^www\./, '');
      if ((u.protocol === 'https:' && u.port === '443') || (u.protocol === 'http:' && u.port === '80')) u.port = '';
      const isMega = /(^|\.)mega\.(?:nz|co\.nz)$/i.test(u.hostname);
      if (!isMega) u.hash = '';
      for (const key of [...u.searchParams.keys()]) {
        const lower = key.toLowerCase();
        if (lower.startsWith('utm_') || TRACKING_KEYS.has(lower)) u.searchParams.delete(key);
      }
      u.searchParams.sort();
      if (u.pathname.length > 1) u.pathname = u.pathname.replace(/\/+$/, '');
      return u.toString();
    } catch (_) {
      return '';
    }
  }

  function loadStore() {
    try {
      const raw = JSON.parse(localStorage.getItem(STORE_KEY) || '{}');
      if (raw && raw.version === 1 && raw.links && typeof raw.links === 'object') return raw;
    } catch (_) {}
    return {version:1, links:{}};
  }

  function saveStore(store) {
    try {
      const rows = Object.entries(store.links || {}).sort((a,b) => Number(b[1]?.lastAt || 0) - Number(a[1]?.lastAt || 0));
      if (rows.length > MAX_LINKS) store.links = Object.fromEntries(rows.slice(0, MAX_LINKS));
      localStorage.setItem(STORE_KEY, JSON.stringify(store));
    } catch (_) {}
  }

  function getEntry(raw) {
    const key = canonicalURL(raw);
    if (!key) return {key:'', entry:null};
    const store = loadStore();
    return {key, entry:store.links[key] || null};
  }

  function formatDate(ts) {
    const n = Number(ts || 0);
    if (!n) return '—';
    try { return new Date(n).toLocaleString('ro-RO', {dateStyle:'short', timeStyle:'short'}); }
    catch (_) { return new Date(n).toLocaleString('ro-RO'); }
  }

  async function api(url) {
    const r = await fetch(url, {cache:'no-store'});
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  }

  async function readRevision() {
    try {
      const data = await api('/api/results/summary');
      const rev = Number(data?.revision || 0);
      if (rev >= 0) {
        knownRevision = rev;
        revisionReady = true;
      }
      return data;
    } catch (_) {
      return null;
    }
  }

  function statusCounts(summary) {
    const eff = summary?.effective || {};
    const wf = summary?.workflow || {};
    const local = Number(eff.HAVE || 0) + Number(eff.VERIFIED || 0) + Number(eff.SAMPLED || 0);
    return {
      total: Number(summary?.total || 0),
      local,
      missing: Number(eff.MISSING || 0),
      review: Number(wf.REVIEW || 0),
      manual: Number(wf.MANUAL || 0)
    };
  }

  async function currentResultsBelongTo(raw) {
    const key = canonicalURL(raw);
    if (!key) return false;
    try {
      const d = await api('/api/results?status=ALL&q=&offset=0&limit=1&sort=path&order=asc');
      const row = Array.isArray(d?.rows) ? d.rows[0] : null;
      const source = row?.remote?.url || '';
      return !!source && canonicalURL(source) === key;
    } catch (_) {
      return false;
    }
  }

  async function completedInternalDownloads(raw) {
    const key = canonicalURL(raw);
    if (!key) return 0;
    try {
      const q = await api('/api/queue/list');
      const seen = new Set();
      for (const job of (q?.jobs || [])) {
        if (String(job?.status || '').toLowerCase() !== 'completed') continue;
        const origin = job?.remote?.url || job?.url || '';
        if (canonicalURL(origin) !== key) continue;
        const id = String(job?.outputPath || job?.id || `${origin}\u001f${job?.name || ''}`);
        seen.add(id);
      }
      return seen.size;
    } catch (_) {
      return 0;
    }
  }

  function ensureBox(input, id) {
    if (!input) return null;
    let box = document.getElementById(id);
    if (box) return box;
    box = document.createElement('div');
    box.id = id;
    box.className = 'ddgSourceHistoryBox';
    box.style.display = 'none';
    if (input.id === 'directUrl') {
      const provider = document.getElementById('universalProviderState');
      if (provider) provider.insertAdjacentElement('afterend', box);
      else input.insertAdjacentElement('afterend', box);
    } else {
      input.insertAdjacentElement('afterend', box);
    }
    return box;
  }

  function ensureStyle() {
    if (document.getElementById(`${NS}Style`)) return;
    const style = document.createElement('style');
    style.id = `${NS}Style`;
    style.textContent = `
      .ddgSourceHistoryBox{margin-top:8px;padding:10px 12px;border:1px solid #314357;border-radius:9px;background:#0b141d;color:#cbd8e7}
      .ddgSourceHistoryBox.good{border-color:#285c49;background:#0d1d18}.ddgSourceHistoryBox.changed{border-color:#665624;background:#201c0e}
      .ddgSourceHistoryTop{display:flex;align-items:center;gap:7px;flex-wrap:wrap}.ddgSourceHistoryTop b{color:#eef6ff}
      .ddgSourceHistoryStats{margin-top:6px;line-height:1.55}.ddgSourceHistoryDelta{margin-top:5px;font-weight:700;color:#ffd979}
      .ddgSourceHistoryNote{margin-top:5px;color:#8fa4ba;font-size:11px}.ddgSourceHistoryNone{color:#8fa4ba}
    `;
    document.head.appendChild(style);
  }

  function renderEntry(raw, box, internalCompleted = null) {
    if (!box) return;
    const {entry} = getEntry(raw);
    if (!canonicalURL(raw)) {
      box.style.display = 'none';
      box.innerHTML = '';
      return;
    }
    box.style.display = '';
    box.className = 'ddgSourceHistoryBox';
    if (!entry || !entry.checks) {
      box.innerHTML = '<span class="ddgSourceHistoryNone"><b>Istoric link:</b> nu a mai fost verificat de DDG pe acest PC.</span>';
      return;
    }
    const last = entry.snapshots?.[entry.snapshots.length - 1] || {};
    const prev = entry.snapshots?.[entry.snapshots.length - 2] || null;
    const allLocal = Number(last.total || 0) > 0 && Number(last.local || 0) >= Number(last.total || 0);
    if (allLocal) box.classList.add('good');
    let deltaHTML = '';
    if (prev && Number(prev.total || 0) !== Number(last.total || 0)) {
      const delta = Number(last.total || 0) - Number(prev.total || 0);
      box.classList.add('changed');
      deltaHTML = `<div class="ddgSourceHistoryDelta">Față de verificarea anterioară: ${delta > 0 ? '+' : ''}${delta.toLocaleString('ro-RO')} fișiere (${Number(prev.total || 0).toLocaleString('ro-RO')} → ${Number(last.total || 0).toLocaleString('ro-RO')}).</div>`;
    }
    const completedText = internalCompleted === null ? '' : ` • descărcări interne DDG finalizate: <b>${Number(internalCompleted).toLocaleString('ro-RO')}</b>`;
    const badge = allLocal ? '<span class="badge VERIFIED">TOATE PE PC</span>' : '<span class="badge HAVE">CUNOSCUT</span>';
    box.innerHTML = `
      <div class="ddgSourceHistoryTop"><b>Istoric link</b>${badge}<span class="sourcePill">verificat ${Number(entry.checks).toLocaleString('ro-RO')}×</span></div>
      <div class="ddgSourceHistoryStats">Ultima verificare: <b>${esc(formatDate(last.at || entry.lastAt))}</b> • atunci avea <b>${Number(last.total || 0).toLocaleString('ro-RO')}</b> fișiere • găsite/confirmate local: <b>${Number(last.local || 0).toLocaleString('ro-RO')}</b> • lipsă: <b>${Number(last.missing || 0).toLocaleString('ro-RO')}</b> • de verificat: <b>${Number(last.review || 0).toLocaleString('ro-RO')}</b>${completedText}</div>
      ${deltaHTML}
      <div class="ddgSourceHistoryNote">„Pe PC” înseamnă găsit/confirmat local de comparația DDG. O trimitere externă în JDownloader nu este considerată finalizată până când fișierul apare local la o verificare ulterioară.</div>`;
  }

  async function renderForInput(input) {
    if (!input) return;
    const box = ensureBox(input, `${NS}-${input.id}`);
    renderEntry(input.value, box, null);
    const raw = String(input.value || '').trim();
    if (!canonicalURL(raw)) return;
    const completed = await completedInternalDownloads(raw);
    if (String(input.value || '').trim() === raw) renderEntry(raw, box, completed);
  }

  function scheduleRender() {
    clearTimeout(inputTimer);
    inputTimer = setTimeout(() => {
      renderForInput(document.getElementById('directUrl'));
      renderForInput(document.getElementById('megaUrl'));
    }, 180);
  }

  function saveSnapshot(raw, revision, summary, internalCompleted) {
    const key = canonicalURL(raw);
    if (!key) return null;
    const counts = statusCounts(summary);
    if (counts.total <= 0) return null;
    if (recordedRevisions.get(key) === revision) return getEntry(raw).entry;
    recordedRevisions.set(key, revision);

    const store = loadStore();
    const now = Date.now();
    const old = store.links[key] || {url:raw, firstAt:now, lastAt:0, checks:0, snapshots:[]};
    const snapshots = Array.isArray(old.snapshots) ? old.snapshots.slice(-MAX_SNAPSHOTS + 1) : [];
    snapshots.push({
      at: now,
      revision,
      total: counts.total,
      local: counts.local,
      missing: counts.missing,
      review: counts.review,
      manual: counts.manual,
      internalCompleted: Number(internalCompleted || 0)
    });
    store.links[key] = {
      url: raw,
      firstAt: Number(old.firstAt || now),
      lastAt: now,
      checks: Number(old.checks || 0) + 1,
      snapshots
    };
    saveStore(store);
    return store.links[key];
  }

  async function tryRecordPending() {
    const p = pending;
    if (!p || p.recording || p.done) return false;
    p.recording = true;
    try {
      const data = await readRevision();
      if (!data) return false;
      const revision = Number(data.revision || 0);
      if (revision <= Number(p.baseline || 0)) return false;
      if (!(await currentResultsBelongTo(p.raw))) return false;
      const completed = await completedInternalDownloads(p.raw);
      const entry = saveSnapshot(p.raw, revision, data.summary || {}, completed);
      if (!entry) return false;
      p.done = true;
      const input = document.getElementById(p.inputId);
      if (input && canonicalURL(input.value) === canonicalURL(p.raw)) renderEntry(p.raw, ensureBox(input, `${NS}-${input.id}`), completed);
      return true;
    } finally {
      p.recording = false;
    }
  }

  function arm(raw, inputId, kind) {
    const key = canonicalURL(raw);
    if (!key) return;
    pending = {
      raw:String(raw || '').trim(), key, inputId, kind,
      baseline: revisionReady ? knownRevision : 0,
      armedAt:Date.now(), recording:false, done:false
    };
    // A short fallback poll catches very fast HTTP scans whose core path does not emit a toast.
    const loop = async () => {
      const p = pending;
      if (!p || p.done || p.key !== key) return;
      if (Date.now() - p.armedAt > (kind === 'mega' ? 20 * 60_000 : 4 * 60_000)) return;
      await tryRecordPending().catch(() => {});
      if (pending === p && !p.done) setTimeout(loop, kind === 'mega' ? 900 : 500);
    };
    setTimeout(loop, kind === 'mega' ? 900 : 350);
  }

  function bindScanCapture() {
    if (document.documentElement.dataset.ddgSourceHistoryCaptureV85117 === '1') return;
    document.documentElement.dataset.ddgSourceHistoryCaptureV85117 = '1';
    document.addEventListener('click', event => {
      const button = event.target?.closest?.('button');
      if (!button) return;
      if (button.id === 'universalScanButton' || button.getAttribute('onclick') === 'scanUniversal()' || button.getAttribute('onclick') === 'scanURL()') {
        const input = document.getElementById('directUrl');
        arm(input?.value, 'directUrl', 'source');
        return;
      }
      if (button.getAttribute('onclick') === 'scanMega()') {
        const input = document.getElementById('megaUrl');
        arm(input?.value, 'megaUrl', 'mega');
      }
    }, true);
  }

  function bindInputs() {
    for (const id of ['directUrl','megaUrl']) {
      const input = document.getElementById(id);
      if (!input || input.dataset.ddgSourceHistoryV85117 === '1') continue;
      input.dataset.ddgSourceHistoryV85117 = '1';
      input.addEventListener('input', scheduleRender);
      input.addEventListener('paste', () => setTimeout(scheduleRender, 0));
      input.addEventListener('change', scheduleRender);
    }
  }

  function bindStatusObserver() {
    const top = document.getElementById('topStatus');
    if (!top || top.dataset.ddgSourceHistoryObserverV85117 === '1') return;
    top.dataset.ddgSourceHistoryObserverV85117 = '1';
    const observer = new MutationObserver(() => {
      if (!pending || pending.done) return;
      const text = String(top.textContent || '').toLowerCase();
      if (text.includes('eroare')) return;
      if (text.includes('analiză terminată') || text.includes('ultima operație: reușită') || text.includes('fișier(e) detectate') || text.includes('fișiere comparate')) {
        setTimeout(() => tryRecordPending().catch(() => {}), 80);
      }
    });
    observer.observe(top, {childList:true, characterData:true, subtree:true});
  }

  function bind() {
    ensureStyle();
    bindInputs();
    bindScanCapture();
    bindStatusObserver();
    readRevision();
    scheduleRender();
    setInterval(readRevision, 2500);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true});
  else bind();
  setTimeout(bind, 600);

  window.ddgSourceHistoryV85117 = {canonicalURL, render: scheduleRender};
})();
