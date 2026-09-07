// TEST v8.5.68 — evidence-first Smart State Engine.
// Keeps the main DDG UI simple while preserving technical detail underneath.
// Current HDD evidence can retire stale manual MISSING/DIFFERENT decisions;
// JDownloader submissions are tracked without touching the verified one-shot handoff.
(() => {
  'use strict';

  const STORAGE_KEY = 'ddg.smartState.transfers.v1';
  const MAX_TRANSFER_AGE_MS = 14 * 24 * 60 * 60 * 1000;
  const MAX_HISTORY_AGE_MS = 45 * 24 * 60 * 60 * 1000;
  const PAGE_LIMIT = 1000;

  let cache = {at: 0, rows: []};
  let cycleTimer = 0;
  let cycleBusy = false;
  let rerun = false;
  let installed = false;

  function upper(value) { return String(value || '').trim().toUpperCase(); }
  function norm(value) { return String(value || '').trim().replace(/\\/g, '/').toLowerCase(); }
  function basename(value) {
    const parts = norm(value).split('/').filter(Boolean);
    return parts.at(-1) || '';
  }

  function remoteFilename(row) {
    return basename(row?.remote?.name || row?.remote?.path || '');
  }

  function exactName(row) {
    if (!row?.localPath) return false;
    const remote = remoteFilename(row);
    const local = basename(row.localPath);
    return Boolean(remote && local && remote === local);
  }

  function rowKey(row) {
    const remote = row?.remote || {};
    const source = upper(remote.source || 'REMOTE');
    const ref = String(remote.handle || remote.providerId || remote.path || remote.name || '').trim();
    const size = Number(remote.size || 0);
    return `${source}|${ref}|${size}`;
  }

  function stableProviderURL(row) {
    const remote = row?.remote || {};
    const source = upper(remote.source);
    try {
      const origin = new URL(remote.url || '').origin;
      if (source === 'GOFILE' && remote.providerId) {
        const u = new URL(remote.url || '');
        const parts = u.pathname.split('/').filter(Boolean);
        if (parts.length >= 2 && parts[0].toLowerCase() === 'd') {
          return `https://gofile.io/?c=${encodeURIComponent(parts[1])}#file=${encodeURIComponent(String(remote.providerId))}`;
        }
      }
      if (source === 'BUNKR' && remote.handle) return `${origin}/f/${encodeURIComponent(String(remote.handle))}`;
      if (source === 'CYBERDROP' && remote.providerId) return `${origin}/f/${encodeURIComponent(String(remote.providerId))}`;
    } catch (_) {}
    return '';
  }

  function jdURL(row) {
    const remote = row?.remote || {};
    return stableProviderURL(row) || String(remote.directUrl || remote.url || '').trim();
  }

  function loadTransfers() {
    try {
      const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      const transfers = raw && typeof raw.transfers === 'object' && raw.transfers ? raw.transfers : {};
      return {version: 1, transfers};
    } catch (_) {
      return {version: 1, transfers: {}};
    }
  }

  function saveTransfers(store) {
    try {
      const now = Date.now();
      for (const [key, item] of Object.entries(store.transfers || {})) {
        const sentAt = Number(item?.sentAt || 0);
        const completedAt = Number(item?.completedAt || 0);
        const anchor = completedAt || sentAt;
        if (!anchor || now - anchor > MAX_HISTORY_AGE_MS) delete store.transfers[key];
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
    } catch (_) {}
  }

  function pendingTransfer(row, store = loadTransfers()) {
    const item = store.transfers?.[rowKey(row)];
    if (!item || upper(item.state) !== 'PENDING') return null;
    const sentAt = Number(item.sentAt || 0);
    if (!sentAt || Date.now() - sentAt > MAX_TRANSFER_AGE_MS) return null;
    return item;
  }

  function currentAuto(row) {
    return upper(row?.autoStatus || row?.status);
  }

  function strongLocalEvidence(row) {
    if (!row?.localPath) return false;
    const guard = upper(row.guardVerdict);
    const auto = currentAuto(row);
    if (guard === 'DUPLICATE') return true;
    if (auto === 'VERIFIED' || upper(row.status) === 'VERIFIED') return true;
    if (auto === 'HAVE') {
      if (row.sameSize && (Number(row.nameScore || 0) >= 95 || exactName(row))) return true;
      if (exactName(row) && Number(row.remote?.size || 0) > 0) return true;
    }
    return false;
  }

  function exactCurrentEvidence(row) {
    if (!row?.localPath) return false;
    const guard = upper(row.guardVerdict);
    const auto = currentAuto(row);
    if (guard === 'DUPLICATE' || auto === 'VERIFIED' || upper(row.status) === 'VERIFIED') return true;
    return auto === 'HAVE' && Boolean(row.sameSize) && exactName(row);
  }

  function shouldReleaseManual(row) {
    if (!row?.manual) return false;
    const manual = upper(row.manualStatus || row.status);
    if (manual === 'MISSING') return strongLocalEvidence(row);
    if (manual === 'DIFFERENT') return exactCurrentEvidence(row);
    // HAVE is intentionally not auto-released merely because the indexed path
    // disappears: the user may know the copy lives outside the indexed roots.
    return false;
  }

  function finalState(row, store = loadTransfers()) {
    // Evidence wins over transfer/history: if the new file is already visible
    // locally, DDG should say AI DEJA even if it was submitted to JD moments ago.
    if (strongLocalEvidence(row)) return 'HAVE';
    if (pendingTransfer(row, store)) return 'IN_JD';

    const guard = upper(row?.guardVerdict);
    if (guard === 'DUPLICATE') return 'HAVE';
    if (guard === 'DOWNLOAD') return 'DOWNLOAD';
    if (guard === 'REVIEW') return 'REVIEW';

    const status = upper(row?.status);
    const auto = currentAuto(row);
    const manual = Boolean(row?.manual);
    const manualStatus = upper(row?.manualStatus || status);

    if (manual) {
      if (manualStatus === 'HAVE') return 'HAVE';
      if (manualStatus === 'MISSING' || manualStatus === 'DIFFERENT') return 'DOWNLOAD';
    }

    if (status === 'VERIFIED' || auto === 'VERIFIED') return 'HAVE';
    if (status === 'MISSING' || status === 'DIFFERENT' || auto === 'MISSING' || auto === 'DIFFERENT') return 'DOWNLOAD';
    if (status === 'HAVE' || auto === 'HAVE') return row?.localPath ? 'HAVE' : 'REVIEW';
    if (['POSSIBLE', 'SAMPLED', 'REVIEW', 'UNKNOWN', ''].includes(status) || ['POSSIBLE', 'SAMPLED'].includes(auto)) return 'REVIEW';
    return row?.localPath ? 'REVIEW' : 'DOWNLOAD';
  }

  function stateReason(row, state) {
    if (state === 'IN_JD') return 'trimis în JDownloader • aștept confirmarea locală';
    if (state === 'HAVE') {
      if (upper(row?.status) === 'VERIFIED' || currentAuto(row) === 'VERIFIED') return 'verificat exact';
      if (upper(row?.guardVerdict) === 'DUPLICATE') return 'Smart Guard confirmă duplicatul';
      if (row?.localPath) {
        const score = Number(row.matchScore || 0);
        return score ? `găsit local • potrivire ${score}%` : 'găsit local';
      }
      return row?.manual ? 'confirmat de tine' : 'copie locală identificată';
    }
    if (state === 'DOWNLOAD') {
      const manual = upper(row?.manualStatus || '');
      if (manual === 'DIFFERENT') return 'copia locală asociată este diferită';
      if (row?.localPath && upper(row?.status) === 'DIFFERENT') return 'există candidat, dar conținutul diferă';
      return row?.manual ? 'marcat pentru descărcare' : 'nu există copie locală confirmată';
    }
    if (row?.localPath) {
      const score = Number(row.matchScore || 0);
      return score ? `candidat local • scor ${score}%` : 'există candidat local • verifică';
    }
    return 'nu există suficientă dovadă pentru un verdict sigur';
  }

  function stateLabel(state) {
    return {HAVE:'AI DEJA', REVIEW:'DE VERIFICAT', DOWNLOAD:'LIPSEȘTE', IN_JD:'ÎN JD'}[state] || state;
  }

  function stateClass(state) {
    return {HAVE:'HAVE', REVIEW:'POSSIBLE', DOWNLOAD:'MISSING', IN_JD:'DDG_IN_JD'}[state] || 'POSSIBLE';
  }

  async function fetchAllRows(force = false) {
    const now = Date.now();
    if (!force && cache.rows.length && now - cache.at < 900) return cache.rows;
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 250; page++) {
      const data = await window.api(`/api/results?offset=${offset}&limit=${PAGE_LIMIT}&status=ALL&sort=path&order=asc`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      rows.push(...batch);
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    cache = {at: now, rows};
    return rows;
  }

  function invalidateCache() { cache = {at: 0, rows: []}; }

  async function restoreAuto(ids) {
    const unique = [...new Set((ids || []).map(Number).filter(Number.isFinite))];
    for (let offset = 0; offset < unique.length; offset += 200) {
      const chunk = unique.slice(offset, offset + 200);
      await window.api('/api/results/mark', {
        method: 'POST',
        headers: {'Content-Type':'application/json'},
        body: JSON.stringify({ids: chunk, status: 'AUTO', note: ''})
      });
    }
  }

  async function reconcileManual(rows) {
    const stale = (rows || []).filter(shouldReleaseManual).map(row => Number(row.id)).filter(Number.isFinite);
    if (!stale.length) return 0;
    await restoreAuto(stale);
    invalidateCache();
    window.toast?.(`Actualizat după rescanare: ${stale.length} decizie(i) vechi au fost înlocuite de starea actuală a HDD-ului.`);
    return stale.length;
  }

  function reconcileTransfers(rows) {
    const store = loadTransfers();
    let changed = false;
    const byKey = new Map((rows || []).map(row => [rowKey(row), row]));
    for (const [key, item] of Object.entries(store.transfers || {})) {
      if (upper(item?.state) !== 'PENDING') continue;
      const row = byKey.get(key);
      if (row && strongLocalEvidence(row)) {
        item.state = 'COMPLETED';
        item.completedAt = Date.now();
        changed = true;
      }
    }
    if (changed) saveTransfers(store);
    return store;
  }

  function setTextIfDifferent(el, text) {
    if (!el) return;
    const value = String(text);
    if (el.textContent !== value) el.textContent = value;
  }

  function setCard(cardValueId, label, value, hint) {
    const valueEl = document.getElementById(cardValueId);
    const card = valueEl?.closest?.('.summaryMini');
    if (!card) return;
    const labelEl = card.querySelector('span');
    const hintEl = card.querySelector('small');
    setTextIfDifferent(labelEl, label);
    setTextIfDifferent(valueEl, Number(value || 0).toLocaleString('ro-RO'));
    setTextIfDifferent(hintEl, hint);
    if (cardValueId === 'sumReview') {
      const progress = card.querySelector('.reviewProgress');
      if (progress) progress.style.display = 'none';
    }
  }

  function renderSummary(rows, store) {
    const counts = {HAVE:0, REVIEW:0, DOWNLOAD:0, IN_JD:0};
    for (const row of rows || []) counts[finalState(row, store)]++;
    setCard('sumTotal', 'TOTAL', rows.length, 'sesiunea curentă');
    setCard('sumReview', 'AI DEJA', counts.HAVE, 'confirmate de starea actuală');
    setCard('sumManual', 'DE VERIFICAT', counts.REVIEW, 'necesită atenție');
    setCard('sumSmart95', 'LIPSESC', counts.DOWNLOAD, 'gata de descărcat');
    setCard('sumMissing', 'ÎN JD', counts.IN_JD, 'trimise • aștept confirmare locală');
  }

  function renderTable(rows, store) {
    const byID = new Map((rows || []).map(row => [Number(row.id), row]));
    for (const tr of document.querySelectorAll('#tbody tr[data-rid]')) {
      const row = byID.get(Number(tr.dataset.rid));
      if (!row) continue;
      const cell = tr.children?.[1];
      if (!cell) continue;
      const state = finalState(row, store);
      const reason = stateReason(row, state);
      const manualSuffix = row.manual && !shouldReleaseManual(row) ? '<span class="ddgSmartMeta">manual</span>' : '';
      const fingerprint = `${state}|${reason}|${manualSuffix}`;
      if (cell.dataset.ddgSmartFingerprint === fingerprint) continue;
      cell.dataset.ddgSmartFingerprint = fingerprint;
      cell.innerHTML = `<span class="badge ${stateClass(state)}">${stateLabel(state)}</span><span class="ddgSmartReason">${reason}</span>${manualSuffix}`;
      cell.title = reason;
    }
  }

  function installStyles() {
    if (document.getElementById('ddgSmartStateV8568Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgSmartStateV8568Style';
    style.textContent = `
      .DDG_IN_JD{background:#173b5f;color:#9fd2ff;border:1px solid #285b87}
      .ddgSmartReason{display:block;margin-top:5px;color:#8ea3b8;font-size:10px;line-height:1.25;max-width:190px}
      .ddgSmartMeta{display:inline-block;margin-top:4px;margin-right:4px;border:1px solid #344b61;border-radius:999px;padding:1px 5px;color:#8ea3b8;font-size:9px;text-transform:uppercase;letter-spacing:.04em}
      .summaryMini[data-ddg-smart="1"] .reviewProgress{display:none!important}
    `;
    document.head.appendChild(style);
  }

  function markSummaryCards() {
    for (const id of ['sumTotal','sumReview','sumManual','sumSmart95','sumMissing']) {
      document.getElementById(id)?.closest?.('.summaryMini')?.setAttribute('data-ddg-smart','1');
    }
  }

  function parseFormField(form, name) {
    const input = form?.querySelector?.(`[name="${name}"]`);
    return String(input?.value || '');
  }

  function splitLines(value) {
    return String(value || '').split(/\r?\n/).map(x => x.trim()).filter(Boolean);
  }

  async function rememberJDSubmission(form) {
    const action = String(form?.action || '').toLowerCase();
    if (!action.includes('127.0.0.1:9666/flashgot')) return;
    const urls = splitLines(parseFormField(form, 'urls'));
    const names = splitLines(parseFormField(form, 'fnames'));
    if (!urls.length) return;
    const packageName = parseFormField(form, 'package');
    try {
      const rows = await fetchAllRows(true);
      const used = new Set();
      const store = loadTransfers();
      const now = Date.now();
      for (let i = 0; i < urls.length; i++) {
        const url = urls[i];
        const name = norm(names[i] || '');
        let matchIndex = rows.findIndex((row, idx) => !used.has(idx) && jdURL(row) === url && (!name || remoteFilename(row) === name));
        if (matchIndex < 0) matchIndex = rows.findIndex((row, idx) => !used.has(idx) && jdURL(row) === url);
        if (matchIndex < 0 && name) matchIndex = rows.findIndex((row, idx) => !used.has(idx) && remoteFilename(row) === name);
        if (matchIndex < 0) continue;
        used.add(matchIndex);
        const row = rows[matchIndex];
        store.transfers[rowKey(row)] = {
          state: 'PENDING', sentAt: now, completedAt: 0,
          packageName, url, name: row?.remote?.name || row?.remote?.path || names[i] || ''
        };
      }
      saveTransfers(store);
      scheduleCycle(80);
    } catch (_) {}
  }

  function installJDSubmitTracker() {
    const proto = window.HTMLFormElement?.prototype;
    if (!proto || proto.__ddgSmartStateSubmitV8568) return;
    const originalSubmit = proto.submit;
    proto.submit = function() {
      try { void rememberJDSubmission(this); } catch (_) {}
      return originalSubmit.apply(this, arguments);
    };
    proto.__ddgSmartStateSubmitV8568 = true;

    if (typeof proto.requestSubmit === 'function' && !proto.__ddgSmartStateRequestSubmitV8568) {
      const originalRequestSubmit = proto.requestSubmit;
      proto.requestSubmit = function() {
        try { void rememberJDSubmission(this); } catch (_) {}
        return originalRequestSubmit.apply(this, arguments);
      };
      proto.__ddgSmartStateRequestSubmitV8568 = true;
    }
  }

  async function runCycle() {
    if (cycleBusy) { rerun = true; return; }
    cycleBusy = true;
    try {
      let rows = await fetchAllRows();
      const released = await reconcileManual(rows);
      if (released) rows = await fetchAllRows(true);
      const store = reconcileTransfers(rows);
      markSummaryCards();
      renderSummary(rows, store);
      renderTable(rows, store);
      window.dispatchEvent(new CustomEvent('ddg:smart-state-updated', {detail:{rows:rows.length}}));
    } catch (_) {
      // The normal DDG UI remains fully usable if this presentation layer cannot refresh.
    } finally {
      cycleBusy = false;
      if (rerun) { rerun = false; scheduleCycle(60); }
    }
  }

  function scheduleCycle(delay = 120) {
    clearTimeout(cycleTimer);
    cycleTimer = setTimeout(runCycle, delay);
  }

  function wrapRefreshFunctions() {
    const load = window.loadResults;
    if (typeof load === 'function' && !load.__ddgSmartStateV8568) {
      const wrapped = async function() {
        const out = await load.apply(this, arguments);
        invalidateCache();
        scheduleCycle(50);
        return out;
      };
      wrapped.__ddgSmartStateV8568 = true;
      window.loadResults = wrapped;
    }

    const summary = window.applySummary;
    if (typeof summary === 'function' && !summary.__ddgSmartStateV8568) {
      const wrapped = function() {
        const out = summary.apply(this, arguments);
        scheduleCycle(80);
        return out;
      };
      wrapped.__ddgSmartStateV8568 = true;
      window.applySummary = wrapped;
    }
  }

  function installObservers() {
    const tbody = document.getElementById('tbody');
    if (tbody && !tbody.dataset.ddgSmartObserved) {
      tbody.dataset.ddgSmartObserved = '1';
      new MutationObserver(() => scheduleCycle(90)).observe(tbody, {childList:true, subtree:true});
    }
  }

  function install() {
    if (installed) return;
    installed = true;
    installStyles();
    installJDSubmitTracker();
    wrapRefreshFunctions();
    installObservers();
    markSummaryCards();
    scheduleCycle(150);
    setTimeout(() => { wrapRefreshFunctions(); installObservers(); scheduleCycle(80); }, 700);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();

  window.ddgSmartStateEngineV8568 = {
    finalState, shouldReleaseManual, strongLocalEvidence, exactCurrentEvidence,
    rowKey, pendingTransfer, fetchAllRows, scheduleCycle
  };
})();
