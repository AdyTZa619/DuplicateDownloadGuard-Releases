// TEST124 — persistent source-link history integrated into Source Intelligence.
// Isolated feature: reads existing DDG APIs and persists only through the
// dedicated loopback history service. It does not override preview/JD/selection.
(() => {
  'use strict';

  const NS = 'ddgSourceHistoryV85124';
  const SERVICE = 'ddg-source-history-v1';
  const PORT_BASE = 38650;
  const PORT_SPAN = 200;
  const PORT_ATTEMPTS = 8;
  const TRACKING_KEYS = new Set(['fbclid','gclid','dclid','msclkid','mc_cid','mc_eid']);

  let serviceURL = '';
  let servicePromise = null;
  let pending = null;
  let knownRevision = 0;
  let revisionReady = false;
  let inputTimer = 0;
  const renderSeq = new Map();

  const $ = id => document.getElementById(id);
  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

  function canonicalURL(raw) {
    try {
      const value = String(raw || '').trim();
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

  async function api(url, options) {
    const r = await fetch(url, {...options, cache:'no-store'});
    if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
    const type = r.headers.get('content-type') || '';
    return type.includes('json') ? r.json() : r.text();
  }

  function fnv1a32(value) {
    let hash = 0x811c9dc5;
    const bytes = new TextEncoder().encode(String(value || '').toLowerCase());
    for (const b of bytes) {
      hash ^= b;
      hash = Math.imul(hash, 0x01000193) >>> 0;
    }
    return hash >>> 0;
  }

  function seedVariants(appDir) {
    const raw = String(appDir || '').trim();
    if (!raw) return [];
    const clean = raw.replace(/[\\/]+$/, '');
    const out = [clean];
    if (/[\\/]data$/i.test(clean)) out.push(clean.replace(/[\\/]data$/i, ''));
    else out.push(clean + (clean.includes('\\') ? '\\data' : '/data'));
    return [...new Set(out.map(x => x.toLowerCase()))];
  }

  async function discoverService() {
    if (serviceURL) return serviceURL;
    if (servicePromise) return servicePromise;
    servicePromise = (async () => {
      let about = null;
      try { about = await api('/api/about'); } catch (_) {}
      const bases = [];
      for (const seed of seedVariants(about?.appDir)) {
        const base = PORT_BASE + (fnv1a32(seed) % PORT_SPAN);
        if (!bases.includes(base)) bases.push(base);
      }
      if (!bases.length) bases.push(PORT_BASE);

      for (const base of bases) {
        for (let i = 0; i < PORT_ATTEMPTS; i++) {
          const candidate = `http://127.0.0.1:${base + i}`;
          try {
            const ctl = new AbortController();
            const timer = setTimeout(() => ctl.abort(), 450);
            const r = await fetch(candidate + '/health', {cache:'no-store', signal:ctl.signal});
            clearTimeout(timer);
            if (!r.ok) continue;
            const data = await r.json();
            if (data?.service === SERVICE && data?.ok) {
              serviceURL = candidate;
              return serviceURL;
            }
          } catch (_) {}
        }
      }
      return '';
    })().finally(() => { servicePromise = null; });
    return servicePromise;
  }

  async function historyGet(raw) {
    const base = await discoverService();
    if (!base) return {available:false, known:false, entry:null};
    try {
      const data = await api(`${base}/history?url=${encodeURIComponent(String(raw || '').trim())}`);
      return {available:true, known:!!data?.known, entry:data?.entry || null};
    } catch (_) {
      serviceURL = '';
      return {available:false, known:false, entry:null};
    }
  }

  function statusCounts(summary) {
    const eff = summary?.effective || {};
    const wf = summary?.workflow || {};
    const local = Number(eff.HAVE || 0) + Number(eff.VERIFIED || 0) + Number(eff.SAMPLED || 0);
    return {
      total:Number(summary?.total || 0),
      local,
      missing:Number(eff.MISSING || 0),
      review:Number(wf.REVIEW || 0),
      manual:Number(wf.MANUAL || 0)
    };
  }

  async function historySave(raw, revision, summary, internalCompleted) {
    const counts = statusCounts(summary);
    if (counts.total <= 0) return null;
    const base = await discoverService();
    if (!base) return null;
    try {
      return await api(base + '/snapshot', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({
          url:String(raw || '').trim(),
          revision:Number(revision || 0),
          total:counts.total,
          local:counts.local,
          missing:counts.missing,
          review:counts.review,
          manual:counts.manual,
          internalCompleted:Number(internalCompleted || 0)
        })
      });
    } catch (_) {
      serviceURL = '';
      return null;
    }
  }

  function formatDate(ts) {
    const n = Number(ts || 0);
    if (!n) return '—';
    try { return new Date(n).toLocaleString('ro-RO', {dateStyle:'short', timeStyle:'short'}); }
    catch (_) { return new Date(n).toLocaleString('ro-RO'); }
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

  async function completedInternalDownloads(raw) {
    const key = canonicalURL(raw);
    if (!key) return 0;
    try {
      const q = await api('/api/queue/list');
      const seen = new Set();
      for (const job of (q?.jobs || [])) {
        if (String(job?.status || '').toLowerCase() !== 'completed') continue;
        const origin = job?.remote?.source || job?.remote?.url || job?.url || '';
        if (canonicalURL(origin) !== key) continue;
        seen.add(String(job?.outputPath || job?.id || `${origin}\u001f${job?.name || ''}`));
      }
      return seen.size;
    } catch (_) {
      return 0;
    }
  }

  function ensureStandaloneBox(input) {
    if (!input) return null;
    const id = `${NS}-${input.id}`;
    let box = $(id);
    if (box) return box;
    box = document.createElement('div');
    box.id = id;
    box.className = 'ddgSourceHistoryBoxV85124';
    box.style.display = 'none';
    input.insertAdjacentElement('afterend', box);
    return box;
  }

  function historyBoxFor(input) {
    if (!input) return null;
    if (input.id === 'directUrl') return $('ddgSourceHistorySlotV85124') || ensureStandaloneBox(input);
    return ensureStandaloneBox(input);
  }

  function ensureStyle() {
    if ($(`${NS}Style`)) return;
    const style = document.createElement('style');
    style.id = `${NS}Style`;
    style.textContent = `
      .ddgSourceHistoryBoxV85124{margin-top:8px;padding:10px 12px;border:1px solid #314357;border-radius:9px;background:#0b141d;color:#cbd8e7}
      .ddgSourceHistoryInnerV85124{min-width:0;color:#cbd8e7}
      .ddgSourceHistoryInnerV85124.good,.ddgSourceHistoryBoxV85124.good{--historyAccent:#2f8061}
      .ddgSourceHistoryInnerV85124.changed,.ddgSourceHistoryBoxV85124.changed{--historyAccent:#b98d2f}
      .ddgSourceHistoryTopV85124{display:flex;align-items:center;gap:7px;flex-wrap:wrap}
      .ddgSourceHistoryStatsV85124{margin-top:6px;line-height:1.55;color:#afc0d2}
      .ddgSourceHistoryDeltaV85124{margin-top:6px;font-weight:800;color:#ffd979}
      .ddgSourceHistoryNoteV85124{margin-top:5px;color:#8097ad;font-size:10px}
      .ddgSourceHistoryNoneV85124{color:#8fa4ba}
    `;
    document.head.appendChild(style);
  }

  function renderHistory(box, state, internalCompleted = null) {
    if (!box) return;
    box.style.display = '';
    box.classList.remove('good','changed');
    box.classList.add(box.id === 'ddgSourceHistorySlotV85124' ? 'ddgSourceHistoryInnerV85124' : 'ddgSourceHistoryBoxV85124');

    if (!state?.available) {
      box.innerHTML = '<span class="ddgSourceHistoryNoneV85124">Istoricul persistent nu răspunde momentan. Scanarea rămâne disponibilă.</span>';
      return;
    }

    const entry = state.entry;
    if (!state.known || !entry?.checks) {
      box.innerHTML = '<span class="ddgSourceHistoryNoneV85124">Link nou pentru această instalare DDG. După scanare îi salvez automat situația.</span>';
      return;
    }

    const snaps = Array.isArray(entry.snapshots) ? entry.snapshots : [];
    const last = snaps[snaps.length - 1] || {};
    const prev = snaps[snaps.length - 2] || null;
    const total = Number(last.total || 0);
    const local = Number(last.local || 0);
    const missing = Number(last.missing || 0);
    const allLocal = total > 0 && local >= total;
    const noneLocal = total > 0 && local === 0;
    if (allLocal) box.classList.add('good');

    let deltaHTML = '';
    if (prev && Number(prev.total || 0) !== total) {
      const old = Number(prev.total || 0);
      const delta = total - old;
      box.classList.add('changed');
      deltaHTML = `<div class="ddgSourceHistoryDeltaV85124">Acum: ${total.toLocaleString('ro-RO')} • ${delta > 0 ? '+' : ''}${delta.toLocaleString('ro-RO')} față de scanarea precedentă (${old.toLocaleString('ro-RO')} → ${total.toLocaleString('ro-RO')})</div>`;
    }

    const downloaded = internalCompleted === null ? Number(last.internalCompleted || 0) : Number(internalCompleted || 0);
    const badge = allLocal
      ? '<span class="badge VERIFIED">TOT CONȚINUTUL CUNOSCUT ESTE PE PC</span>'
      : noneLocal
        ? '<span class="badge POSSIBLE">VERIFICAT, NIMIC CONFIRMAT LOCAL</span>'
        : '<span class="badge HAVE">LINK CUNOSCUT</span>';

    box.innerHTML = `
      <div class="ddgSourceHistoryTopV85124">${badge}<span class="sourcePill">${Number(entry.checks).toLocaleString('ro-RO')} verificări</span></div>
      <div class="ddgSourceHistoryStatsV85124">Ultima: <b>${esc(formatDate(last.at || entry.lastAt))}</b> • total <b>${total.toLocaleString('ro-RO')}</b> • pe PC <b>${local.toLocaleString('ro-RO')}</b> • lipsă <b>${missing.toLocaleString('ro-RO')}</b> • de verificat <b>${Number(last.review || 0).toLocaleString('ro-RO')}</b>${downloaded ? ` • DDG finalizate <b>${downloaded.toLocaleString('ro-RO')}</b>` : ''}</div>
      ${deltaHTML}
      <div class="ddgSourceHistoryNoteV85124">Trimiterea în JDownloader nu înseamnă automat „descărcat”; DDG confirmă când fișierul apare efectiv într-un folder indexat.</div>`;
  }

  async function renderForInput(input) {
    if (!input) return;
    const box = historyBoxFor(input);
    if (!box) return;
    const raw = String(input.value || '').trim();
    if (!canonicalURL(raw)) {
      box.style.display = 'none';
      box.innerHTML = '';
      return;
    }
    const seq = (renderSeq.get(input.id) || 0) + 1;
    renderSeq.set(input.id, seq);
    box.style.display = '';
    box.innerHTML = '<span class="ddgSourceHistoryNoneV85124">Citesc istoricul linkului…</span>';
    const [state, completed] = await Promise.all([historyGet(raw), completedInternalDownloads(raw)]);
    if (renderSeq.get(input.id) !== seq || String(input.value || '').trim() !== raw) return;
    renderHistory(box, state, completed);
  }

  function scheduleRender() {
    clearTimeout(inputTimer);
    inputTimer = setTimeout(() => {
      renderForInput($('directUrl'));
      renderForInput($('megaUrl'));
    }, 180);
  }

  async function tryRecordPending() {
    const p = pending;
    if (!p || p.recording || p.done) return false;
    p.recording = true;
    try {
      const input = $(p.inputId);
      if (!input || canonicalURL(input.value) !== p.key) return false;
      const data = await readRevision();
      if (!data) return false;
      const revision = Number(data.revision || 0);
      if (revision <= Number(p.baseline || 0)) return false;
      const completed = await completedInternalDownloads(p.raw);
      const saved = await historySave(p.raw, revision, data.summary || {}, completed);
      if (!saved?.entry) return false;
      p.done = true;
      if (canonicalURL(input.value) === p.key) renderHistory(historyBoxFor(input), {available:true, known:true, entry:saved.entry}, completed);
      window.dispatchEvent(new CustomEvent('ddg:source-history-updated', {detail:{url:p.raw, entry:saved.entry}}));
      return true;
    } finally {
      p.recording = false;
    }
  }

  function arm(raw, inputId, kind) {
    const key = canonicalURL(raw);
    if (!key) return;
    pending = {
      raw:String(raw || '').trim(),
      key,
      inputId,
      kind,
      baseline:revisionReady ? knownRevision : 0,
      armedAt:Date.now(),
      recording:false,
      done:false
    };
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
    if (document.documentElement.dataset.ddgSourceHistoryCaptureV85124 === '1') return;
    document.documentElement.dataset.ddgSourceHistoryCaptureV85124 = '1';
    document.addEventListener('click', event => {
      const button = event.target?.closest?.('button');
      if (!button) return;
      if (button.id === 'universalScanButton' || button.getAttribute('onclick') === 'scanUniversal()' || button.getAttribute('onclick') === 'scanURL()') {
        arm($('directUrl')?.value, 'directUrl', 'source');
      } else if (button.getAttribute('onclick') === 'scanMega()') {
        arm($('megaUrl')?.value, 'megaUrl', 'mega');
      }
    }, true);
  }

  function bindInputs() {
    for (const id of ['directUrl','megaUrl']) {
      const input = $(id);
      if (!input || input.dataset.ddgSourceHistoryV85124 === '1') continue;
      input.dataset.ddgSourceHistoryV85124 = '1';
      input.addEventListener('input', scheduleRender);
      input.addEventListener('paste', () => setTimeout(scheduleRender, 0));
      input.addEventListener('change', scheduleRender);
      input.addEventListener('keydown', event => {
        if (event.key === 'Enter') arm(input.value, id, id === 'megaUrl' ? 'mega' : 'source');
      }, true);
    }
  }

  function bindStatusObserver() {
    const top = $('topStatus');
    if (!top || top.dataset.ddgSourceHistoryObserverV85124 === '1') return;
    top.dataset.ddgSourceHistoryObserverV85124 = '1';
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
    discoverService().then(scheduleRender);
    readRevision();
    scheduleRender();
    setInterval(readRevision, 2500);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true});
  else bind();
  setTimeout(bind, 600);

  window.ddgSourceHistoryV85124 = {canonicalURL, render:scheduleRender, service:discoverService};
})();
