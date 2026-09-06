// Visible activity/progress + compact checked-link journal for online checks and pre-download guard.
// The progress bar never invents a percentage: it is indeterminate while the backend call is alive,
// switches to backend-reported current/total when /api/status exposes real counters, and reaches 100%
// only after the HTTP operation actually returns.
(() => {
  'use strict';

  const COOKIE = 'ddgCheckJournalV1';
  const COOKIE_MAX = 3400;
  const nativeFetch = window.fetch.bind(window);
  const active = {source: null, preflight: null};

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

  function readJournal() {
    try {
      const part = document.cookie.split('; ').find(x => x.startsWith(COOKIE + '='));
      if (!part) return [];
      const rows = JSON.parse(decodeURIComponent(part.slice(COOKIE.length + 1)));
      return Array.isArray(rows) ? rows : [];
    } catch (_) {
      return [];
    }
  }

  function writeJournal(rows) {
    let list = Array.isArray(rows) ? rows.slice(-30) : [];
    let raw = JSON.stringify(list);
    while (encodeURIComponent(raw).length > COOKIE_MAX && list.length > 1) {
      list.shift();
      raw = JSON.stringify(list);
    }
    try {
      document.cookie = `${COOKIE}=${encodeURIComponent(raw)}; Max-Age=31536000; Path=/; SameSite=Lax`;
    } catch (_) {}
    updateJournalButton();
  }

  function addJournal(row) {
    const rows = readJournal();
    rows.push({
      at: Date.now(),
      kind: row.kind || 'LINK',
      provider: row.provider || '',
      url: row.url || '',
      status: row.status || '',
      detail: row.detail || '',
      items: Number(row.items || 0),
      ms: Number(row.ms || 0)
    });
    writeJournal(rows);
  }

  function updateJournalButton() {
    const b = document.getElementById('ddgCheckJournalBtn');
    if (!b) return;
    const n = readJournal().length;
    b.textContent = `📋 Jurnal verificări${n ? ` (${n})` : ''}`;
  }

  function installStyles() {
    if (document.getElementById('ddgCheckActivityStyles')) return;
    const style = document.createElement('style');
    style.id = 'ddgCheckActivityStyles';
    style.textContent = `
      .ddgCheckToolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:9px}
      .ddgLiveProgress{margin-top:9px;padding:10px 11px;border:1px solid #304256;border-radius:9px;background:#0d151e}
      .ddgLiveProgress.hidden{display:none}.ddgLiveProgressHead{display:flex;justify-content:space-between;gap:10px;align-items:center}
      .ddgLiveProgressHead b{font-size:11px}.ddgLiveProgressPct{font-size:10px;color:#a9bed4;white-space:nowrap}
      .ddgLiveBar{height:7px;border-radius:999px;background:#172432;overflow:hidden;margin-top:7px;position:relative}
      .ddgLiveBar i{display:block;height:100%;width:0%;background:currentColor;transition:width .18s ease;border-radius:999px}
      .ddgLiveBar.indeterminate i{width:38%!important;position:absolute;animation:ddgCheckSweep 1.15s linear infinite}
      .ddgLiveProgress.running{color:#69b7ff}.ddgLiveProgress.ok{color:#3ddc97}.ddgLiveProgress.error{color:#ff7685}
      .ddgLiveProgressDetail{margin-top:6px;font-size:10px;color:#a9bed4;line-height:1.35;white-space:normal}
      @keyframes ddgCheckSweep{0%{left:-40%}100%{left:105%}}
      #ddgCheckJournalModal{position:fixed;inset:0;z-index:10080;background:rgba(4,8,13,.78);display:flex;align-items:center;justify-content:center;padding:24px}
      #ddgCheckJournalModal.hidden{display:none}#ddgCheckJournalModal .box{width:min(980px,94vw);max-height:82vh;background:#0d141d;border:1px solid #304256;border-radius:12px;display:flex;flex-direction:column;overflow:hidden}
      #ddgCheckJournalModal .head{display:flex;justify-content:space-between;align-items:center;padding:12px 14px;border-bottom:1px solid #263749}
      #ddgCheckJournalModal .body{overflow:auto;padding:10px 14px 14px}#ddgCheckJournalModal table{width:100%;border-collapse:collapse;font-size:11px}
      #ddgCheckJournalModal th,#ddgCheckJournalModal td{text-align:left;vertical-align:top;padding:7px 6px;border-bottom:1px solid #213041}
      #ddgCheckJournalModal td.url{max-width:420px;word-break:break-all}#ddgCheckJournalModal .foot{display:flex;justify-content:flex-end;gap:8px;padding:10px 14px;border-top:1px solid #263749}
    `;
    document.head.appendChild(style);
  }

  function progressMarkup(id, title) {
    return `<div class="ddgLiveProgress hidden" id="${id}">
      <div class="ddgLiveProgressHead"><b>${esc(title)}</b><span class="ddgLiveProgressPct">—</span></div>
      <div class="ddgLiveBar indeterminate"><i></i></div>
      <div class="ddgLiveProgressDetail">Aștept pornirea…</div>
    </div>`;
  }

  function installUI() {
    installStyles();
    const sourceState = document.getElementById('universalProviderState');
    if (sourceState && !document.getElementById('ddgCheckJournalBtn')) {
      sourceState.insertAdjacentHTML('afterend', `<div class="ddgCheckToolbar"><button class="btn" type="button" id="ddgCheckJournalBtn">📋 Jurnal verificări</button></div>${progressMarkup('ddgSourceCheckProgress', 'Verificare link / sursă')}`);
      document.getElementById('ddgCheckJournalBtn')?.addEventListener('click', openJournal);
    }

    const activity = document.getElementById('guardActivity');
    if (activity && !document.getElementById('ddgPreflightCheckProgress')) {
      activity.insertAdjacentHTML('afterend', progressMarkup('ddgPreflightCheckProgress', 'Verificare înainte de download'));
    } else if (!document.getElementById('ddgPreflightCheckProgress')) {
      const button = document.getElementById('downloadGuardBtn');
      if (button?.parentElement) button.parentElement.insertAdjacentHTML('afterend', progressMarkup('ddgPreflightCheckProgress', 'Verificare înainte de download'));
    }

    if (!document.getElementById('ddgCheckJournalModal')) {
      document.body.insertAdjacentHTML('beforeend', `<div id="ddgCheckJournalModal" class="hidden" role="dialog" aria-modal="true">
        <div class="box"><div class="head"><div><b>📋 Jurnal linkuri și verificări</b><div class="muted small">Ultimele verificări păstrate local de DDG.</div></div><button class="btn" id="ddgCheckJournalClose">×</button></div>
        <div class="body" id="ddgCheckJournalBody"></div><div class="foot"><button class="btn danger" id="ddgCheckJournalClear">Golește jurnalul</button><button class="btn" id="ddgCheckJournalClose2">Închide</button></div></div>
      </div>`);
      document.getElementById('ddgCheckJournalClose')?.addEventListener('click', closeJournal);
      document.getElementById('ddgCheckJournalClose2')?.addEventListener('click', closeJournal);
      document.getElementById('ddgCheckJournalClear')?.addEventListener('click', () => {
        writeJournal([]);
        renderJournal();
      });
      document.getElementById('ddgCheckJournalModal')?.addEventListener('click', e => { if (e.target?.id === 'ddgCheckJournalModal') closeJournal(); });
    }
    updateJournalButton();
  }

  function renderJournal() {
    const body = document.getElementById('ddgCheckJournalBody');
    if (!body) return;
    const rows = readJournal().slice().reverse();
    if (!rows.length) {
      body.innerHTML = '<div class="muted" style="padding:20px;text-align:center">Jurnalul este gol.</div>';
      return;
    }
    body.innerHTML = `<table><thead><tr><th>Data</th><th>Tip</th><th>Sursă</th><th>Link / operație</th><th>Rezultat</th><th>Timp</th></tr></thead><tbody>${rows.map(r => {
      const dt = new Date(Number(r.at || 0));
      const when = Number.isFinite(dt.getTime()) ? dt.toLocaleString('ro-RO') : '—';
      const items = r.items > 0 ? ` • ${r.items} fiș.` : '';
      const target = r.url ? `<span title="${esc(r.url)}">${esc(r.url)}</span>` : esc(r.detail || '—');
      return `<tr><td>${esc(when)}</td><td>${esc(r.kind || '')}</td><td>${esc(r.provider || '')}</td><td class="url">${target}</td><td><b>${esc(r.status || '')}</b>${items}${r.detail && r.url ? `<div class="muted small">${esc(r.detail)}</div>` : ''}</td><td>${r.ms > 0 ? `${(r.ms/1000).toFixed(1)}s` : '—'}</td></tr>`;
    }).join('')}</tbody></table>`;
  }

  function openJournal() {
    renderJournal();
    document.getElementById('ddgCheckJournalModal')?.classList.remove('hidden');
  }
  function closeJournal() { document.getElementById('ddgCheckJournalModal')?.classList.add('hidden'); }

  function getProgressElement(kind) {
    return document.getElementById(kind === 'source' ? 'ddgSourceCheckProgress' : 'ddgPreflightCheckProgress');
  }

  function renderProgress(kind, state) {
    const box = getProgressElement(kind);
    if (!box || !state) return;
    box.classList.remove('hidden', 'running', 'ok', 'error');
    box.classList.add(state.tone || 'running');
    const bar = box.querySelector('.ddgLiveBar');
    const fill = bar?.querySelector('i');
    const pct = box.querySelector('.ddgLiveProgressPct');
    const detail = box.querySelector('.ddgLiveProgressDetail');
    const head = box.querySelector('.ddgLiveProgressHead b');
    if (head && state.title) head.textContent = state.title;
    if (bar) bar.classList.toggle('indeterminate', state.percent == null);
    if (fill && state.percent != null) fill.style.width = `${Math.max(0, Math.min(100, state.percent))}%`;
    if (pct) pct.textContent = state.percent == null ? `${state.elapsed || 0}s • lucrează` : `${Math.round(state.percent)}% • ${state.elapsed || 0}s`;
    if (detail) detail.textContent = state.detail || 'Operație în curs…';
  }

  function finishProgress(kind, ok, detail, elapsed) {
    renderProgress(kind, {tone: ok ? 'ok' : 'error', percent: 100, elapsed, detail, title: kind === 'source' ? 'Verificare link / sursă' : 'Verificare înainte de download'});
    const token = active[kind];
    setTimeout(() => {
      if (active[kind] !== token) return;
      getProgressElement(kind)?.classList.add('hidden');
    }, ok ? 3500 : 6500);
  }

  async function readStatus() {
    try {
      const r = await nativeFetch('/api/status?_ddgActivity=' + Date.now(), {cache:'no-store'});
      if (!r.ok) return null;
      return await r.json();
    } catch (_) { return null; }
  }

  async function beginProgress(kind) {
    installUI();
    const token = {started: Date.now(), baseline: '', timer: null, polling: false};
    active[kind] = token;
    const initial = await readStatus();
    if (active[kind] !== token) return token;
    token.baseline = String(initial?.message || '');
    const tick = async () => {
      if (active[kind] !== token || token.polling) return;
      token.polling = true;
      const elapsed = Math.max(0, Math.floor((Date.now() - token.started) / 1000));
      const p = await readStatus();
      if (active[kind] !== token) return;
      let percent = null;
      let detail = kind === 'source' ? 'DDG analizează sursa; aștept răspunsul extractorului și comparația locală.' : 'Smart Guard verifică indexul local, istoricul și candidații înainte de transfer.';
      if (p && (p.active || (p.message && String(p.message) !== token.baseline))) {
        detail = [p.message, p.detail].filter(Boolean).join(' • ') || detail;
        const current = Number(p.current || 0), total = Number(p.total || 0);
        if (total > 0 && current >= 0) percent = Math.max(0, Math.min(99, current / total * 100));
        else if (Number(p.stepTotal || 0) > 0 && Number(p.step || 0) > 0) percent = Math.max(0, Math.min(99, (Number(p.step)-1) / Number(p.stepTotal) * 100));
      }
      renderProgress(kind, {tone:'running', percent, elapsed, detail});
      token.polling = false;
    };
    renderProgress(kind, {tone:'running', percent:null, elapsed:0, detail: kind === 'source' ? 'Cererea a ajuns la DDG; verificarea este activă.' : 'Cererea de pre-verificare a ajuns la DDG; Smart Guard este activ.'});
    token.timer = setInterval(tick, 650);
    tick();
    return token;
  }

  function endProgress(kind, token, ok, detail) {
    if (!token || active[kind] !== token) return;
    if (token.timer) clearInterval(token.timer);
    const elapsed = Math.max(0, Math.floor((Date.now() - token.started) / 1000));
    finishProgress(kind, ok, detail, elapsed);
  }

  function requestPath(input) {
    try {
      const raw = typeof input === 'string' ? input : input?.url;
      return new URL(raw, location.href).pathname;
    } catch (_) { return ''; }
  }

  function parseBody(init) {
    try {
      if (typeof init?.body === 'string' && init.body.trim()) return JSON.parse(init.body);
    } catch (_) {}
    return {};
  }

  function sourceKind(path) {
    return ['/api/source/scan','/api/mega/scan','/api/url/scan'].includes(path);
  }
  function preflightKind(path) {
    return ['/api/download/preflight','/api/queue/add'].includes(path);
  }

  const previousFetch = window.fetch.bind(window);
  window.fetch = async function ddgCheckActivityFetch(input, init) {
    const path = requestPath(input);
    const source = sourceKind(path);
    const preflight = preflightKind(path);
    if (!source && !preflight) return previousFetch(input, init);

    const kind = source ? 'source' : 'preflight';
    const requestData = parseBody(init);
    const token = await beginProgress(kind);
    const started = Date.now();
    try {
      const response = await previousFetch(input, init);
      const clone = response.clone();
      let payload = null;
      try {
        const ct = clone.headers.get('content-type') || '';
        payload = ct.includes('json') ? await clone.json() : await clone.text();
      } catch (_) {}
      const ok = response.ok;
      const ms = Date.now() - started;
      const detail = ok
        ? (source ? `Verificare terminată${payload?.items != null ? ` • ${payload.items} fișier(e)` : ''}.` : 'Verificarea înainte de download s-a încheiat.')
        : `Operația s-a oprit: HTTP ${response.status}${typeof payload === 'string' && payload.trim() ? ' • ' + payload.trim().slice(0,220) : ''}`;
      endProgress(kind, token, ok, detail);

      if (source) {
        const rawURL = String(requestData.url || requestData.URL || '').trim();
        addJournal({
          kind: path === '/api/mega/scan' ? 'MEGA' : 'LINK',
          provider: String(payload?.adapter || requestData.adapter || (path === '/api/mega/scan' ? 'MEGA' : '')).toUpperCase(),
          url: rawURL,
          status: ok ? 'OK' : `HTTP ${response.status}`,
          detail: ok ? `${Number(payload?.items || 0)} fișier(e) comparate` : (typeof payload === 'string' ? payload.trim().slice(0,240) : 'Eroare de verificare'),
          items: Number(payload?.items || 0),
          ms
        });
      } else if (payload && typeof payload === 'object') {
        const counts = payload.counts || payload.guard?.counts || {};
        const selected = Array.isArray(requestData.ids) ? requestData.ids.length : 0;
        addJournal({
          kind: 'PRE-DOWNLOAD',
          provider: String(requestData.engine || '').toUpperCase(),
          status: ok ? 'VERIFICAT' : `HTTP ${response.status}`,
          detail: ok ? `${selected} selectate • download ${Number(counts.DOWNLOAD || 0)} • duplicate ${Number(counts.DUPLICATE || 0)} • review ${Number(counts.REVIEW || 0)}` : 'Pre-verificare eșuată',
          items: selected,
          ms
        });
      }
      return response;
    } catch (error) {
      endProgress(kind, token, false, `Eroare: ${error?.message || error}`);
      if (source) {
        addJournal({kind:'LINK', provider:String(requestData.adapter || '').toUpperCase(), url:String(requestData.url || '').trim(), status:'EROARE', detail:String(error?.message || error), ms:Date.now()-started});
      }
      throw error;
    }
  };

  function boot() {
    installUI();
    let tries = 0;
    const timer = setInterval(() => {
      installUI();
      if (++tries > 30 || (document.getElementById('ddgSourceCheckProgress') && document.getElementById('ddgPreflightCheckProgress'))) clearInterval(timer);
    }, 300);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();

  window.ddgCheckActivityV8563 = {readJournal, openJournal};
})();