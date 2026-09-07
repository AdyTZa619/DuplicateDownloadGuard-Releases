// TEST v8.5.69 — evidence-first Smart State Engine + best-candidate reconciliation.
// Design borrows proven patterns from sync/duplicate tools: current filesystem evidence
// is separate from remembered history, candidate pairs can be rejected independently,
// and verification escalates from cheap metadata to stronger checks only when needed.
(() => {
  'use strict';

  const STORAGE_KEY = 'ddg.smartState.transfers.v2';
  const LEGACY_STORAGE_KEY = 'ddg.smartState.transfers.v1';
  const MAX_TRANSFER_AGE_MS = 14 * 24 * 60 * 60 * 1000;
  const MAX_HISTORY_AGE_MS = 45 * 24 * 60 * 60 * 1000;
  const CANDIDATE_RETRY_MS = 12000;
  const PAGE_LIMIT = 1000;

  let cache = {at: 0, rows: []};
  let cycleTimer = 0;
  let cycleBusy = false;
  let candidateBusy = false;
  let rerun = false;
  let installed = false;
  const candidateAttempts = new Map();

  function upper(value) { return String(value || '').trim().toUpperCase(); }
  function norm(value) { return String(value || '').trim().replace(/\\/g, '/').toLowerCase(); }
  function basename(value) {
    const parts = norm(value).split('/').filter(Boolean);
    return parts.at(-1) || '';
  }
  function remoteFilename(row) { return basename(row?.remote?.name || row?.remote?.path || ''); }
  function candidateFilename(candidate) { return basename(candidate?.name || candidate?.path || ''); }
  function exactName(row) {
    if (!row?.localPath) return false;
    const remote = remoteFilename(row), local = basename(row.localPath);
    return Boolean(remote && local && remote === local);
  }
  function exactCandidateName(row, candidate) {
    const remote = remoteFilename(row), local = candidateFilename(candidate);
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
    const remote = row?.remote || {}, source = upper(remote.source);
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

  function emptyStore() { return {version: 2, transfers: {}, rejectedPairs: {}, resolutions: {}}; }
  function loadTransfers() {
    try {
      let raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null');
      if (!raw) raw = JSON.parse(localStorage.getItem(LEGACY_STORAGE_KEY) || 'null');
      const store = emptyStore();
      if (raw && typeof raw === 'object') {
        if (raw.transfers && typeof raw.transfers === 'object') store.transfers = raw.transfers;
        if (raw.rejectedPairs && typeof raw.rejectedPairs === 'object') store.rejectedPairs = raw.rejectedPairs;
        if (raw.resolutions && typeof raw.resolutions === 'object') store.resolutions = raw.resolutions;
      }
      return store;
    } catch (_) { return emptyStore(); }
  }
  function saveTransfers(store) {
    try {
      const now = Date.now();
      store.version = 2;
      store.transfers ||= {}; store.rejectedPairs ||= {}; store.resolutions ||= {};
      for (const [key, item] of Object.entries(store.transfers)) {
        const anchor = Number(item?.completedAt || item?.sentAt || 0);
        if (!anchor || now - anchor > MAX_HISTORY_AGE_MS) delete store.transfers[key];
      }
      for (const [key, item] of Object.entries(store.resolutions)) {
        const at = Number(item?.at || 0);
        if (!at || now - at > MAX_HISTORY_AGE_MS) delete store.resolutions[key];
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
      localStorage.removeItem(LEGACY_STORAGE_KEY);
    } catch (_) {}
  }
  function transferRecord(row, store = loadTransfers()) { return store.transfers?.[rowKey(row)] || null; }
  function pendingTransfer(row, store = loadTransfers()) {
    const item = transferRecord(row, store);
    if (!item || upper(item.state) !== 'PENDING') return null;
    const sentAt = Number(item.sentAt || 0);
    if (!sentAt || Date.now() - sentAt > MAX_TRANSFER_AGE_MS) return null;
    return item;
  }
  function resolutionRecord(row, store = loadTransfers()) { return store.resolutions?.[rowKey(row)] || null; }

  function rejectedPaths(row, store) {
    const values = store.rejectedPairs?.[rowKey(row)];
    return new Set(Array.isArray(values) ? values.map(norm).filter(Boolean) : []);
  }
  function rejectPair(row, path, store) {
    const p = norm(path);
    if (!p) return false;
    const key = rowKey(row);
    const values = Array.isArray(store.rejectedPairs?.[key]) ? store.rejectedPairs[key].slice() : [];
    if (values.some(x => norm(x) === p)) return false;
    values.push(path);
    store.rejectedPairs[key] = values.slice(-12);
    return true;
  }

  function currentAuto(row) { return upper(row?.autoStatus || row?.status); }
  function strongLocalEvidence(row) {
    if (!row?.localPath) return false;
    const guard = upper(row.guardVerdict), auto = currentAuto(row);
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
    const guard = upper(row.guardVerdict), auto = currentAuto(row);
    if (guard === 'DUPLICATE' || auto === 'VERIFIED' || upper(row.status) === 'VERIFIED') return true;
    return auto === 'HAVE' && Boolean(row.sameSize) && exactName(row);
  }
  function manualStatus(row) { return upper(row?.manualStatus || (row?.manual ? row?.status : '')); }
  function shouldReleaseManual(row) {
    if (!row?.manual) return false;
    const manual = manualStatus(row);
    if (manual === 'MISSING') return strongLocalEvidence(row);
    if (manual === 'DIFFERENT') return exactCurrentEvidence(row);
    return false;
  }

  function finalState(row, store = loadTransfers()) {
    if (strongLocalEvidence(row)) return 'HAVE';
    if (pendingTransfer(row, store)) return 'IN_JD';
    const guard = upper(row?.guardVerdict);
    if (guard === 'DUPLICATE') return 'HAVE';
    if (guard === 'DOWNLOAD') return 'DOWNLOAD';
    if (guard === 'REVIEW') return 'REVIEW';
    const status = upper(row?.status), auto = currentAuto(row), manual = Boolean(row?.manual), ms = manualStatus(row);
    if (manual) {
      if (ms === 'HAVE') return 'HAVE';
      if (ms === 'MISSING' || ms === 'DIFFERENT') return 'DOWNLOAD';
    }
    if (status === 'VERIFIED' || auto === 'VERIFIED') return 'HAVE';
    if (status === 'MISSING' || status === 'DIFFERENT' || auto === 'MISSING' || auto === 'DIFFERENT') return 'DOWNLOAD';
    if (status === 'HAVE' || auto === 'HAVE') return row?.localPath ? 'HAVE' : 'REVIEW';
    if (['POSSIBLE','SAMPLED','REVIEW','UNKNOWN',''].includes(status) || ['POSSIBLE','SAMPLED'].includes(auto)) return 'REVIEW';
    return row?.localPath ? 'REVIEW' : 'DOWNLOAD';
  }

  function stateReason(row, state, store = loadTransfers()) {
    const transfer = transferRecord(row, store), resolution = resolutionRecord(row, store);
    if (state === 'IN_JD') return 'trimis în JDownloader • aștept confirmarea locală';
    if (state === 'HAVE') {
      if (resolution?.reason === 'jd-rebind') return 'găsit după JD • candidat nou confirmat';
      if (resolution?.reason === 'candidate-rebind') return 'candidat local nou confirmat';
      if (upper(row?.status) === 'VERIFIED' || currentAuto(row) === 'VERIFIED') return 'verificat exact';
      if (upper(row?.guardVerdict) === 'DUPLICATE') return 'Smart Guard confirmă duplicatul';
      if (row?.localPath) {
        const score = Number(row.matchScore || 0);
        return score ? `găsit local • potrivire ${score}%` : 'găsit local';
      }
      return row?.manual ? 'confirmat de tine' : 'copie locală identificată';
    }
    if (state === 'DOWNLOAD') {
      const ms = manualStatus(row);
      if (ms === 'DIFFERENT') return 'copia locală asociată este diferită';
      if (row?.localPath && upper(row?.status) === 'DIFFERENT') return 'există candidat, dar conținutul diferă';
      if (transfer && upper(transfer.state) === 'COMPLETED') return 'download anterior înregistrat • fișierul nu mai este confirmat local';
      return row?.manual ? 'marcat pentru descărcare' : 'nu există copie locală confirmată';
    }
    if (resolution?.reason === 'candidate-review') return 'candidat nou mai bun • necesită verificare';
    if (row?.localPath) {
      const score = Number(row.matchScore || 0);
      return score ? `candidat local • scor ${score}%` : 'există candidat local • verifică';
    }
    return 'nu există suficientă dovadă pentru un verdict sigur';
  }
  function stateLabel(state) { return {HAVE:'AI DEJA', REVIEW:'DE VERIFICAT', DOWNLOAD:'LIPSEȘTE', IN_JD:'ÎN JD'}[state] || state; }
  function stateClass(state) { return {HAVE:'HAVE', REVIEW:'POSSIBLE', DOWNLOAD:'MISSING', IN_JD:'DDG_IN_JD'}[state] || 'POSSIBLE'; }

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
        method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({ids:chunk,status:'AUTO',note:''})
      });
    }
  }
  async function selectCandidate(id, path) {
    return window.api('/api/results/candidate', {
      method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:Number(id),path})
    });
  }
  async function smartVerifyCandidate(id, path) {
    return window.api('/api/results/smart-verify', {
      method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({id:Number(id),path})
    });
  }
  async function candidateRows(row) {
    const data = await window.api(`/api/results/candidates?id=${encodeURIComponent(Number(row.id))}&limit=25`);
    return Array.isArray(data?.rows) ? data.rows : [];
  }

  function candidatePriority(row, candidate, store) {
    if (!candidate?.path || !candidate.sameSize) return -1e9;
    const transfer = pendingTransfer(row, store);
    const rejected = rejectedPaths(row, store);
    const path = norm(candidate.path);
    if (rejected.has(path)) return -1e8;
    const exact = exactCandidateName(row, candidate);
    const nameScore = Number(candidate.nameScore || 0), matchScore = Number(candidate.matchScore || 0);
    let score = matchScore * 3 + nameScore * 2;
    if (candidate.sameExt) score += 35;
    if (exact) score += 180;
    if (transfer) {
      score += 20;
      if (exact) score += 100;
    }
    if (path === norm(row?.localPath)) score -= 120;
    return score;
  }

  function candidateIsStrong(row, candidate, store) {
    if (!candidate?.sameSize) return false;
    if (!candidate.sameExt && basename(row?.remote?.name || row?.remote?.path).includes('.')) return false;
    if (exactCandidateName(row, candidate)) return true;
    const nameScore = Number(candidate.nameScore || 0), matchScore = Number(candidate.matchScore || 0);
    if (nameScore >= 97 && matchScore >= 95) return true;
    return Boolean(pendingTransfer(row, store)) && nameScore >= 92 && matchScore >= 94;
  }

  function needsCandidateRebind(row, store) {
    if (!row || strongLocalEvidence(row)) return false;
    const ms = manualStatus(row);
    if (row.manual && (ms === 'MISSING' || ms === 'DIFFERENT')) return true;
    if (pendingTransfer(row, store)) return true;
    return false;
  }

  function attemptFingerprint(row) {
    return [rowKey(row), norm(row?.localPath), upper(row?.status), currentAuto(row), manualStatus(row), Number(row?.matchScore || 0)].join('|');
  }

  async function reconcileOneCandidate(row, store, force = false) {
    const fingerprint = attemptFingerprint(row);
    const prev = candidateAttempts.get(fingerprint) || 0;
    if (!force && Date.now() - prev < CANDIDATE_RETRY_MS) return {changed:false, skipped:true};
    candidateAttempts.set(fingerprint, Date.now());

    const ms = manualStatus(row);
    let storeChanged = false;
    // A manual MISSING/DIFFERENT verdict means the old pair itself must not keep
    // winning every future scan. This is pair-level rejection, not a global ignore.
    if (row.manual && (ms === 'MISSING' || ms === 'DIFFERENT') && row.localPath) {
      storeChanged = rejectPair(row, row.localPath, store) || storeChanged;
    }

    let candidates = await candidateRows(row);
    candidates = candidates
      .filter(c => c?.path && norm(c.path) !== norm(row.localPath))
      .sort((a,b) => candidatePriority(row,b,store) - candidatePriority(row,a,store));
    const best = candidates.find(c => candidateIsStrong(row,c,store));
    if (!best) {
      if (storeChanged) saveTransfers(store);
      return {changed:false, noCandidate:true};
    }

    let updated = await selectCandidate(row.id, best.path);
    let verifyResult = null;
    const beforeManual = Boolean(row.manual);
    const beforeManualStatus = ms;

    // Progressive verification, rclone-style: metadata chooses the candidate;
    // stronger remote verification is attempted only when the current verdict
    // still cannot safely retire a DIFFERENT decision or complete a JD transfer.
    const needVerify = !strongLocalEvidence(updated) && (beforeManualStatus === 'DIFFERENT' || pendingTransfer(updated,store));
    if (needVerify) {
      try {
        verifyResult = await smartVerifyCandidate(row.id, best.path);
        if (verifyResult?.result) updated = verifyResult.result;
      } catch (_) {
        // A provider may not support the stronger check. Keep the conservative state.
      }
    }

    let released = false;
    if (beforeManual) {
      if (beforeManualStatus === 'MISSING' && candidateIsStrong(row,best,store)) {
        // The stale "missing" decision no longer describes the current disk.
        // AUTO may become HAVE or REVIEW; both are safer than pretending it is absent.
        await restoreAuto([row.id]);
        released = true;
      } else if (beforeManualStatus === 'DIFFERENT' && exactCurrentEvidence(updated)) {
        await restoreAuto([row.id]);
        released = true;
      }
    }

    const transfer = transferRecord(row, store);
    const reason = transfer ? 'jd-rebind' : (released || strongLocalEvidence(updated) ? 'candidate-rebind' : 'candidate-review');
    store.resolutions[rowKey(row)] = {at:Date.now(), path:best.path, reason, nameScore:Number(best.nameScore||0), matchScore:Number(best.matchScore||0)};
    if (transfer && (released || strongLocalEvidence(updated) || exactCurrentEvidence(updated))) {
      transfer.state = 'COMPLETED'; transfer.completedAt = Date.now(); transfer.localPath = best.path;
    }
    saveTransfers(store);
    invalidateCache();
    return {changed:true,released,best,verified:Boolean(verifyResult)};
  }

  async function reconcileBestCandidates(rows, options = {}) {
    if (candidateBusy) return {changed:0,released:0,missing:0,targets:0};
    candidateBusy = true;
    try {
      const store = loadTransfers();
      const scope = options.ids ? new Set(options.ids.map(Number).filter(Number.isFinite)) : null;
      let targets = (rows || []).filter(row => (!scope || scope.has(Number(row.id))) && needsCandidateRebind(row,store));
      const max = options.force ? 250 : (Number(options.max || 24));
      targets = targets.slice(0,max);
      let changed = 0, released = 0, missing = 0;
      for (const row of targets) {
        try {
          const result = await reconcileOneCandidate(row,store,Boolean(options.force));
          if (result.changed) changed++;
          if (result.released) released++;
          if (result.noCandidate) missing++;
        } catch (_) { missing++; }
      }
      return {changed,released,missing,targets:targets.length};
    } finally { candidateBusy = false; }
  }

  async function reconcileManual(rows) {
    const stale = (rows || []).filter(shouldReleaseManual).map(row => Number(row.id)).filter(Number.isFinite);
    if (!stale.length) return 0;
    await restoreAuto(stale);
    invalidateCache();
    return stale.length;
  }

  function reconcileTransfers(rows) {
    const store = loadTransfers();
    let changed = false;
    const byKey = new Map((rows || []).map(row => [rowKey(row),row]));
    for (const [key,item] of Object.entries(store.transfers || {})) {
      if (upper(item?.state) !== 'PENDING') continue;
      const row = byKey.get(key);
      if (row && strongLocalEvidence(row)) {
        item.state = 'COMPLETED'; item.completedAt = Date.now(); item.localPath = row.localPath || '';
        store.resolutions[key] = {at:Date.now(),path:row.localPath||'',reason:'jd-rebind'};
        changed = true;
      }
    }
    if (changed) saveTransfers(store);
    return store;
  }

  function setTextIfDifferent(el,text) { if (el && el.textContent !== String(text)) el.textContent = String(text); }
  function setCard(cardValueId,label,value,hint) {
    const valueEl = document.getElementById(cardValueId), card = valueEl?.closest?.('.summaryMini');
    if (!card) return;
    setTextIfDifferent(card.querySelector('span'),label);
    setTextIfDifferent(valueEl,Number(value||0).toLocaleString('ro-RO'));
    setTextIfDifferent(card.querySelector('small'),hint);
    if (cardValueId === 'sumReview') { const p = card.querySelector('.reviewProgress'); if (p) p.style.display='none'; }
  }
  function renderSummary(rows,store) {
    const counts = {HAVE:0,REVIEW:0,DOWNLOAD:0,IN_JD:0};
    for (const row of rows || []) counts[finalState(row,store)]++;
    setCard('sumTotal','TOTAL',rows.length,'sesiunea curentă');
    setCard('sumReview','AI DEJA',counts.HAVE,'confirmate de starea actuală');
    setCard('sumManual','DE VERIFICAT',counts.REVIEW,'necesită atenție');
    setCard('sumSmart95','LIPSESC',counts.DOWNLOAD,'gata de descărcat');
    setCard('sumMissing','ÎN JD',counts.IN_JD,'trimise • aștept confirmare locală');
  }
  function renderTable(rows,store) {
    const byID = new Map((rows||[]).map(row => [Number(row.id),row]));
    for (const tr of document.querySelectorAll('#tbody tr[data-rid]')) {
      const row = byID.get(Number(tr.dataset.rid)); if (!row) continue;
      const cell = tr.children?.[1]; if (!cell) continue;
      const state = finalState(row,store), reason = stateReason(row,state,store);
      const manualSuffix = row.manual && !shouldReleaseManual(row) ? '<span class="ddgSmartMeta">manual</span>' : '';
      const fingerprint = `${state}|${reason}|${manualSuffix}`;
      if (cell.dataset.ddgSmartFingerprint === fingerprint) continue;
      cell.dataset.ddgSmartFingerprint = fingerprint;
      cell.innerHTML = `<span class="badge ${stateClass(state)}">${stateLabel(state)}</span><span class="ddgSmartReason">${reason}</span>${manualSuffix}`;
      cell.title = reason;
    }
  }

  function installStyles() {
    if (document.getElementById('ddgSmartStateV8569Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgSmartStateV8569Style';
    style.textContent = `
      .DDG_IN_JD{background:#173b5f;color:#9fd2ff;border:1px solid #285b87}
      .ddgSmartReason{display:block;margin-top:5px;color:#8ea3b8;font-size:10px;line-height:1.25;max-width:200px}
      .ddgSmartMeta{display:inline-block;margin-top:4px;margin-right:4px;border:1px solid #344b61;border-radius:999px;padding:1px 5px;color:#8ea3b8;font-size:9px;text-transform:uppercase;letter-spacing:.04em}
      .summaryMini[data-ddg-smart="1"] .reviewProgress{display:none!important}
      #ddgReconcileJDBtn{white-space:nowrap}
    `;
    document.head.appendChild(style);
  }
  function markSummaryCards() {
    for (const id of ['sumTotal','sumReview','sumManual','sumSmart95','sumMissing']) document.getElementById(id)?.closest?.('.summaryMini')?.setAttribute('data-ddg-smart','1');
  }

  function selectedIDs() {
    try {
      if (typeof window.idsForAction === 'function') return window.idsForAction().map(Number).filter(Number.isFinite);
    } catch (_) {}
    return [...document.querySelectorAll('#tbody .rowcheck:checked')].map(x=>Number(x.dataset.id)).filter(Number.isFinite);
  }
  async function manualReconcile() {
    const button = document.getElementById('ddgReconcileJDBtn');
    if (button?.disabled) return;
    if (button) { button.disabled=true; button.textContent='⏳ Reconciliez…'; }
    try {
      let rows = await fetchAllRows(true);
      const ids = selectedIDs();
      const result = await reconcileBestCandidates(rows,{force:true,ids:ids.length?ids:null});
      rows = await fetchAllRows(true);
      const released = await reconcileManual(rows);
      if (released) rows = await fetchAllRows(true);
      const store = reconcileTransfers(rows);
      renderSummary(rows,store); renderTable(rows,store);
      window.loadResults?.();
      if (!result.targets) window.toast?.(ids.length ? 'Selecția nu are fișiere care necesită reconciliere.' : 'Nu există fișiere în așteptare după JDownloader.');
      else if (result.changed) window.toast?.(`Reconciliere JD: ${result.changed} candidat(ți) actualizați${result.released+released ? ` • ${result.released+released} stări manuale înlocuite` : ''}.`);
      else window.toast?.(`Reconciliere JD: nu am găsit încă o copie locală mai bună pentru ${result.targets} fișier(e). Verifică dacă folderul JDownloader este inclus în index.`);
    } catch (error) { window.toast?.(error?.message || String(error)); }
    finally { if (button) { button.disabled=false; button.textContent='↻ Reconciliază după JD'; } }
  }
  function installReconcileButton() {
    if (document.getElementById('ddgReconcileJDBtn')) return;
    const anchor = document.querySelector('button[onclick="smartVerifySelected()"]') || document.getElementById('smartVerifySelectionBtn');
    const parent = anchor?.parentElement || document.querySelector('.selectionToolbar .group');
    if (!parent) return;
    const button = document.createElement('button');
    button.id='ddgReconcileJDBtn'; button.type='button'; button.className='btn';
    button.textContent='↻ Reconciliază după JD';
    button.title='Caută din nou cel mai bun candidat local pentru fișierele trimise în JD sau marcate Lipsește/Diferit. Nu pornește o rescanare HDD.';
    button.addEventListener('click',manualReconcile);
    parent.appendChild(button);
  }

  function parseFormField(form,name) { return String(form?.querySelector?.(`[name="${name}"]`)?.value || ''); }
  function splitLines(value) { return String(value||'').split(/\r?\n/).map(x=>x.trim()).filter(Boolean); }
  async function rememberJDSubmission(form) {
    const action = String(form?.action||'').toLowerCase();
    if (!action.includes('127.0.0.1:9666/flashgot')) return;
    const urls=splitLines(parseFormField(form,'urls')), names=splitLines(parseFormField(form,'fnames'));
    if (!urls.length) return;
    const packageName=parseFormField(form,'package');
    try {
      const rows=await fetchAllRows(true), used=new Set(), store=loadTransfers(), now=Date.now();
      for (let i=0;i<urls.length;i++) {
        const url=urls[i], name=norm(names[i]||'');
        let idx=rows.findIndex((row,j)=>!used.has(j)&&jdURL(row)===url&&(!name||remoteFilename(row)===name));
        if(idx<0) idx=rows.findIndex((row,j)=>!used.has(j)&&jdURL(row)===url);
        if(idx<0&&name) idx=rows.findIndex((row,j)=>!used.has(j)&&remoteFilename(row)===name);
        if(idx<0) continue;
        used.add(idx); const row=rows[idx];
        store.transfers[rowKey(row)]={state:'PENDING',sentAt:now,completedAt:0,packageName,url,name:row?.remote?.name||row?.remote?.path||names[i]||''};
      }
      saveTransfers(store); scheduleCycle(80);
    } catch (_) {}
  }
  function installJDSubmitTracker() {
    const proto=window.HTMLFormElement?.prototype;
    if(!proto||proto.__ddgSmartStateSubmitV8569) return;
    const originalSubmit=proto.submit;
    proto.submit=function(){ try{void rememberJDSubmission(this);}catch(_){} return originalSubmit.apply(this,arguments); };
    proto.__ddgSmartStateSubmitV8569=true;
    if(typeof proto.requestSubmit==='function'&&!proto.__ddgSmartStateRequestSubmitV8569){
      const originalRequestSubmit=proto.requestSubmit;
      proto.requestSubmit=function(){ try{void rememberJDSubmission(this);}catch(_){} return originalRequestSubmit.apply(this,arguments); };
      proto.__ddgSmartStateRequestSubmitV8569=true;
    }
  }

  async function runCycle() {
    if(cycleBusy){rerun=true;return;} cycleBusy=true;
    try {
      let rows=await fetchAllRows();
      const candidateResult=await reconcileBestCandidates(rows,{force:false,max:24});
      if(candidateResult.changed) rows=await fetchAllRows(true);
      const released=await reconcileManual(rows);
      if(released) rows=await fetchAllRows(true);
      const store=reconcileTransfers(rows);
      markSummaryCards(); renderSummary(rows,store); renderTable(rows,store); installReconcileButton();
      window.dispatchEvent(new CustomEvent('ddg:smart-state-updated',{detail:{rows:rows.length,reconciled:candidateResult.changed}}));
    } catch (_) {} finally {
      cycleBusy=false; if(rerun){rerun=false;scheduleCycle(60);}
    }
  }
  function scheduleCycle(delay=120){clearTimeout(cycleTimer);cycleTimer=setTimeout(runCycle,delay);}
  function wrapRefreshFunctions(){
    const load=window.loadResults;
    if(typeof load==='function'&&!load.__ddgSmartStateV8569){
      const wrapped=async function(){const out=await load.apply(this,arguments);invalidateCache();scheduleCycle(50);return out;};
      wrapped.__ddgSmartStateV8569=true; window.loadResults=wrapped;
    }
    const summary=window.applySummary;
    if(typeof summary==='function'&&!summary.__ddgSmartStateV8569){
      const wrapped=function(){const out=summary.apply(this,arguments);scheduleCycle(80);return out;};
      wrapped.__ddgSmartStateV8569=true; window.applySummary=wrapped;
    }
  }
  function installObservers(){
    const tbody=document.getElementById('tbody');
    if(tbody&&!tbody.dataset.ddgSmartObservedV8569){tbody.dataset.ddgSmartObservedV8569='1';new MutationObserver(()=>scheduleCycle(90)).observe(tbody,{childList:true,subtree:true});}
  }
  function install(){
    if(installed)return;installed=true;installStyles();installJDSubmitTracker();wrapRefreshFunctions();installObservers();markSummaryCards();installReconcileButton();scheduleCycle(150);
    setTimeout(()=>{wrapRefreshFunctions();installObservers();installReconcileButton();scheduleCycle(80);},700);
  }
  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',install,{once:true});else install();

  window.ddgSmartStateEngineV8569={
    finalState,shouldReleaseManual,strongLocalEvidence,exactCurrentEvidence,rowKey,pendingTransfer,
    fetchAllRows,scheduleCycle,reconcileBestCandidates,candidateIsStrong,candidatePriority,manualReconcile
  };
})();
