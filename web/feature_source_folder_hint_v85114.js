// TEST128 — Folder Advisor v2 + compact source decision summary.
// Uses current DDG results first, then cautiously blends persistent source-folder
// observations. It does not rescan HDD and does not patch Media Picker/JD/preview.
(() => {
  'use strict';

  const SOURCE_PANEL_ID = 'ddgSourceFolderHintV85114';
  const LEARNING_SERVICE = 'ddg-source-folder-learning-v1';
  const LEARNING_PORT_BASE = 39250;
  const LEARNING_PORT_SPAN = 200;
  const LEARNING_PORT_ATTEMPTS = 8;

  let rowsCache = [];
  let rowsAt = 0;
  let sourceStamp = '';
  let computeSeq = 0;
  let decisionTimer = 0;
  let sourceTimer = 0;
  let lastReport = null;
  let learningURL = '';
  let learningPromise = null;
  const candidateCache = new Map();

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

  async function api(url, options) {
    if (!options && typeof window.api === 'function') return window.api(url);
    const response = await fetch(url, {...options, cache:'no-store'});
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : response.text();
  }

  function currentSource() {
    return String(document.getElementById('directUrl')?.value || document.getElementById('megaUrl')?.value || '').trim();
  }

  function normalizePath(value) {
    return String(value || '').trim().replace(/\//g, '\\').replace(/\\+$/g, '');
  }

  function dirname(value) {
    const p = normalizePath(value);
    const i = p.lastIndexOf('\\');
    if (i <= 2) return p;
    return p.slice(0, i);
  }

  function roots() {
    const list = Array.isArray(window.cfg?.localPaths) ? window.cfg.localPaths : [];
    return list.map(normalizePath).filter(Boolean).sort((a,b) => b.length - a.length);
  }

  function isUnder(path, root) {
    const p = normalizePath(path).toLowerCase();
    const r = normalizePath(root).toLowerCase();
    return p === r || p.startsWith(r + '\\');
  }

  // v2: always recommend a meaningful child folder. The old implementation
  // collapsed to the configured root whenever more than one HDD root existed,
  // which produced results such as H:\ instead of H:\trans\Ana.
  function bucketFolder(filePath) {
    const parent = dirname(filePath);
    if (!parent) return '';
    const localRoots = roots();
    for (const root of localRoots) {
      if (!isUnder(parent, root)) continue;
      const cleanRoot = normalizePath(root);
      const rel = normalizePath(parent).slice(cleanRoot.length).replace(/^\\+/, '');
      const parts = rel.split('\\').filter(Boolean);
      if (!parts.length) return cleanRoot;
      const driveRoot = /^[a-z]:$/i.test(cleanRoot) || /^[a-z]:\\?$/i.test(cleanRoot);
      const take = driveRoot && parts.length > 1 ? 2 : 1;
      return `${cleanRoot}\\${parts.slice(0, take).join('\\')}`;
    }
    return parent;
  }

  function statusOf(row) {
    return String(row?.status || row?.autoStatus || '').trim().toUpperCase();
  }

  function strongLocalEvidence(row) {
    const status = statusOf(row);
    const guard = String(row?.guardVerdict || '').trim().toUpperCase();
    return Boolean(row?.manual) || guard === 'DUPLICATE' || ['VERIFIED','HAVE','SAMPLED'].includes(status);
  }

  async function readAllRows(force = false) {
    const stamp = currentSource();
    if (stamp !== sourceStamp) {
      sourceStamp = stamp;
      rowsCache = [];
      rowsAt = 0;
      candidateCache.clear();
      lastReport = null;
    }
    if (!force && rowsCache.length && Date.now() - rowsAt < 15000) return rowsCache;
    const out = [];
    let offset = 0;
    for (let page = 0; page < 500; page++) {
      const data = await api(`/api/results?status=ALL&q=&offset=${offset}&limit=1000&sort=path&order=asc`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      out.push(...batch);
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    rowsCache = out;
    rowsAt = Date.now();
    return out;
  }

  async function candidatesFor(row) {
    const id = Number(row?.id);
    if (!Number.isFinite(id)) return [];
    if (candidateCache.has(id)) return candidateCache.get(id);
    try {
      const data = await api(`/api/results/candidates?id=${id}&limit=5`);
      const list = Array.isArray(data?.rows) ? data.rows : [];
      candidateCache.set(id, list);
      return list;
    } catch (_) {
      candidateCache.set(id, []);
      return [];
    }
  }

  function scoreRow(map, folder) {
    const clean = normalizePath(folder);
    if (!clean) return null;
    const key = clean.toLowerCase();
    let row = map.get(key);
    if (!row) {
      row = {folder:clean, score:0, currentScore:0, strong:0, linked:0, candidates:0, learned:0, learnedScore:0, exactLearned:0, familyLearned:0, hostLearned:0, examples:[]};
      map.set(key, row);
    }
    return row;
  }

  function addCurrentScore(map, path, points, kind, fileName = '') {
    const row = scoreRow(map, bucketFolder(path));
    if (!row) return;
    row.score += points;
    row.currentScore += points;
    if (kind === 'strong') row.strong++;
    else if (kind === 'linked') row.linked++;
    else row.candidates++;
    if (fileName && row.examples.length < 3 && !row.examples.includes(fileName)) row.examples.push(fileName);
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

  async function discoverLearningService() {
    if (learningURL) return learningURL;
    if (learningPromise) return learningPromise;
    learningPromise = (async () => {
      let about = null;
      try { about = await api('/api/about'); } catch (_) {}
      const bases = [];
      for (const seed of seedVariants(about?.appDir)) {
        const base = LEARNING_PORT_BASE + (fnv1a32(seed) % LEARNING_PORT_SPAN);
        if (!bases.includes(base)) bases.push(base);
      }
      if (!bases.length) bases.push(LEARNING_PORT_BASE);
      for (const base of bases) {
        for (let i = 0; i < LEARNING_PORT_ATTEMPTS; i++) {
          const candidate = `http://127.0.0.1:${base + i}`;
          try {
            const ctl = new AbortController();
            const timer = setTimeout(() => ctl.abort(), 450);
            const response = await fetch(candidate + '/health', {cache:'no-store', signal:ctl.signal});
            clearTimeout(timer);
            if (!response.ok) continue;
            const data = await response.json();
            if (data?.ok && data?.service === LEARNING_SERVICE) {
              learningURL = candidate;
              return learningURL;
            }
          } catch (_) {}
        }
      }
      return '';
    })().finally(() => { learningPromise = null; });
    return learningPromise;
  }

  async function learnedSuggestions(raw) {
    if (!raw) return [];
    const base = await discoverLearningService();
    if (!base) return [];
    try {
      const data = await api(`${base}/suggest?url=${encodeURIComponent(raw)}`);
      return Array.isArray(data?.suggestions) ? data.suggestions : [];
    } catch (_) {
      learningURL = '';
      return [];
    }
  }

  async function rememberFolder(raw, currentTop) {
    if (!raw || !currentTop || currentTop.strong < 1) return;
    const base = await discoverLearningService();
    if (!base) return;
    const confidence = currentTop.strong >= 3 ? 'ridicata' : 'medie';
    const evidence = Math.max(1, currentTop.strong * 3 + currentTop.linked + currentTop.candidates);
    try {
      await api(base + '/observe', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({url:raw, folder:currentTop.folder, confidence, evidence})
      });
    } catch (_) {
      learningURL = '';
    }
  }

  function applyLearned(scores, suggestions) {
    for (const item of suggestions || []) {
      const row = scoreRow(scores, item?.folder);
      if (!row) continue;
      const learnedScore = Math.min(18, Math.max(0, Number(item?.score || 0)));
      row.score += learnedScore;
      row.learnedScore += learnedScore;
      row.learned += Math.max(0, Number(item?.observations || 0));
      row.exactLearned += Math.max(0, Number(item?.exact || 0));
      row.familyLearned += Math.max(0, Number(item?.family || 0));
      row.hostLearned += Math.max(0, Number(item?.host || 0));
    }
  }

  function reportConfidence(items) {
    if (!items.length) return 'fără dovezi';
    const top = items[0];
    const second = items[1]?.score || 0;
    if (top.strong >= 3 && top.score >= second * 1.35) return 'ridicată';
    if (top.strong >= 1 || top.exactLearned >= 1) return 'medie';
    if (top.learned >= 2 && top.score >= second * 1.25) return 'medie';
    return 'orientativă';
  }

  async function compute(ids = [], deep = false) {
    const seq = ++computeSeq;
    const all = await readAllRows();
    if (seq !== computeSeq) return null;
    const wanted = new Set((ids || []).map(Number).filter(Number.isFinite));
    const rows = wanted.size ? all.filter(row => wanted.has(Number(row?.id))) : all;
    const scores = new Map();

    for (const row of rows) {
      const local = String(row?.localPath || '').trim();
      if (!local) continue;
      const strong = strongLocalEvidence(row);
      addCurrentScore(scores, local, strong ? 6 : 2.2, strong ? 'strong' : 'linked', row?.remote?.name || row?.remote?.path || '');
    }

    if (deep) {
      const unresolved = rows.filter(row => !row?.localPath && Number(row?.candidates || 0) > 0).slice(0, 24);
      for (const row of unresolved) {
        if (seq !== computeSeq) return null;
        const list = await candidatesFor(row);
        list.slice(0, 3).forEach((candidate, rank) => {
          const quality = Math.max(0, Math.min(99, Number(candidate?.matchScore || 0)));
          if (quality < 42) return;
          const weight = (1.5 - rank * 0.28) * (0.55 + quality / 200);
          addCurrentScore(scores, candidate.path, weight, 'candidate', row?.remote?.name || row?.remote?.path || '');
        });
      }
    }

    const currentItems = [...scores.values()].sort((a,b) => b.currentScore - a.currentScore || b.strong - a.strong || a.folder.localeCompare(b.folder));
    const currentTop = currentItems[0] ? {...currentItems[0]} : null;
    const source = currentSource();
    const learned = await learnedSuggestions(source);
    if (seq !== computeSeq) return null;
    applyLearned(scores, learned);

    const items = [...scores.values()].sort((a,b) => b.score - a.score || b.strong - a.strong || b.exactLearned - a.exactLearned || a.folder.localeCompare(b.folder));
    const report = {source, rows:rows.length, deep, confidence:reportConfidence(items), items:items.slice(0, 3)};
    if (currentTop?.strong >= 1) rememberFolder(source, currentTop).catch(() => {});
    return report;
  }

  function fallbackCopy(value) {
    const ta = document.createElement('textarea');
    ta.value = value;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); window.toast?.('Calea folderului a fost copiată'); } catch (_) {}
    ta.remove();
  }

  function copyText(text) {
    const value = String(text || '');
    if (!value) return;
    if (navigator.clipboard?.writeText) navigator.clipboard.writeText(value).then(() => window.toast?.('Calea folderului a fost copiată')).catch(() => fallbackCopy(value));
    else fallbackCopy(value);
  }

  function resultHTML(report) {
    if (!report) return '<span class="muted">Analiza folderului a fost întreruptă.</span>';
    if (!report.items.length) {
      return `<div class="folderHintTitle"><b>Folder recomandat</b><span class="sourcePill">${report.rows} rezultate</span></div><div class="muted small">Nu există încă suficiente legături locale. „Analiză extinsă” folosește și candidații locali.</div><div class="ddgFolderHintToolsV85114"><button class="btn" id="ddgFolderHintDeepV85114" type="button">Analiză extinsă</button></div>`;
    }
    const top = report.items[0];
    const history = top.learned ? ` • ${top.learned} observații din istoric` : '';
    const detail = `${top.strong} potriviri confirmate • ${top.linked} asocieri • ${top.candidates} indicii${history}`;
    const alternatives = report.items.slice(1).map(x => `<div class="folderHintAlt"><code>${esc(x.folder)}</code><span>${x.strong + x.linked + x.candidates + x.learned} indicii</span></div>`).join('');
    return `<div class="folderHintTitle"><b>Folder recomandat</b><span class="badge ${report.confidence === 'ridicată' ? 'VERIFIED' : 'POSSIBLE'}">${esc(report.confidence.toUpperCase())}</span>${top.learned ? '<span class="sourcePill">învățat din istoric</span>' : ''}</div><div class="folderHintTop"><code title="${esc(top.folder)}">${esc(top.folder)}</code><button class="btn folderHintCopyV85114" type="button" data-folder="${esc(top.folder)}">Copiază</button></div><div class="muted small">${esc(detail)}${report.deep ? ' • analiză extinsă' : ''}</div>${alternatives}<div class="ddgFolderHintToolsV85114"><button class="btn" id="ddgFolderHintDeepV85114" type="button">Analiză extinsă</button></div>`;
  }

  function ensureStyles() {
    if (document.getElementById('ddgSourceFolderHintStyleV85114')) return;
    const style = document.createElement('style');
    style.id = 'ddgSourceFolderHintStyleV85114';
    style.textContent = `
      .ddgFolderHintV85114{margin:0;padding:0;border:0;background:transparent;min-width:0}
      .ddgFolderHintV85114 .folderHintTitle{display:flex;gap:7px;align-items:center;flex-wrap:wrap;margin-bottom:7px}
      .ddgFolderHintV85114 .folderHintTop{display:flex;gap:7px;align-items:center;min-width:0}.ddgFolderHintV85114 code{font-family:Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#c7d9ea;min-width:0;flex:1}
      .ddgFolderHintV85114 .folderHintAlt{display:flex;gap:7px;align-items:center;margin-top:6px;padding-top:6px;border-top:1px solid #1e2b38}.ddgFolderHintV85114 .folderHintAlt span{font-size:10px;color:#8fa5ba;white-space:nowrap}
      .ddgFolderHintToolsV85114{display:flex;gap:7px;margin-top:7px;flex-wrap:wrap}
      .ddgSourceDecisionV85128{display:grid;grid-template-columns:auto repeat(5,minmax(90px,1fr));gap:1px;background:#223141;border-top:1px solid #223141}
      .ddgSourceDecisionV85128 .decisionLabel{display:flex;align-items:center;padding:9px 11px;background:#0d151e;font-size:9px;font-weight:900;letter-spacing:.08em;color:#7691ab}
      .ddgSourceDecisionV85128 .decisionCell{padding:8px 10px;background:#0b131c;min-width:0}.ddgSourceDecisionV85128 .decisionCell span{display:block;font-size:8px;color:#6f8ba6;font-weight:800;letter-spacing:.06em}.ddgSourceDecisionV85128 .decisionCell b{display:block;margin-top:3px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:12px;color:#d3dfeb}
      @media(max-width:980px){.ddgSourceDecisionV85128{grid-template-columns:repeat(3,1fr)}.ddgSourceDecisionV85128 .decisionLabel{grid-column:1/-1}}
    `;
    document.head.appendChild(style);
  }

  function ensureSourcePanel() {
    ensureStyles();
    let panel = document.getElementById(SOURCE_PANEL_ID);
    if (panel) return panel;
    const slot = document.getElementById('ddgSourceFolderSlotV85124');
    const anchor = document.getElementById('ddgMediaPickerLaunchV8566') || document.getElementById('universalProviderState');
    if (!slot && !anchor?.parentElement) return null;
    panel = document.createElement('div');
    panel.id = SOURCE_PANEL_ID;
    panel.className = 'ddgFolderHintV85114';
    panel.innerHTML = '<div class="folderHintTitle"><b>Folder recomandat</b></div><div class="muted small">După scanare folosesc potrivirile locale și istoricul sursei, fără rescanare HDD.</div>';
    if (slot) slot.replaceChildren(panel);
    else anchor.insertAdjacentElement('afterend', panel);
    return panel;
  }

  function wirePanel(panel) {
    panel.querySelectorAll('.folderHintCopyV85114').forEach(button => button.addEventListener('click', event => {
      event.preventDefault();
      event.stopPropagation();
      copyText(button.dataset.folder || '');
    }));
    panel.querySelector('#ddgFolderHintDeepV85114')?.addEventListener('click', () => refreshPanel(panel, [], true));
  }

  function setPanel(panel, report) {
    if (!panel || !report) return;
    lastReport = report;
    panel.innerHTML = resultHTML(report);
    wirePanel(panel);
    window.dispatchEvent(new CustomEvent('ddg:folder-advisor-updated', {detail:{report}}));
    scheduleDecisionRefresh();
  }

  async function refreshPanel(panel = ensureSourcePanel(), ids = [], deep = false) {
    if (!panel) return null;
    panel.innerHTML = `<span class="muted small">${deep ? 'Analizez potrivirile, candidații și istoricul…' : 'Calculez folderul cel mai legat de sursă…'}</span>`;
    try {
      const report = await compute(ids, deep);
      if (report) setPanel(panel, report);
      return report;
    } catch (error) {
      panel.innerHTML = `<span class="muted small">Folder Advisor: ${esc(error?.message || String(error))}</span>`;
      return null;
    }
  }

  function selectedCount() {
    const text = String(document.getElementById('ddgMediaPickerCountV8566')?.textContent || '');
    const match = text.match(/(\d[\d\s.,]*)\s+selectat/i);
    if (match) {
      const n = Number(match[1].replace(/[^0-9]/g, ''));
      if (Number.isFinite(n)) return n;
    }
    try {
      const ids = typeof window.idsForAction === 'function' ? window.idsForAction() : [];
      if (Array.isArray(ids)) return ids.length;
    } catch (_) {}
    return 0;
  }

  function ensureDecisionBar() {
    const card = document.getElementById('ddgSourceIntelligenceV85124');
    if (!card) return null;
    let bar = document.getElementById('ddgSourceDecisionV85128');
    if (bar) return bar;
    bar = document.createElement('div');
    bar.id = 'ddgSourceDecisionV85128';
    bar.className = 'ddgSourceDecisionV85128';
    bar.innerHTML = '<div class="decisionLabel">SITUAȚIE CURENTĂ</div><div class="decisionCell"><span>TOTAL</span><b id="ddgDecisionTotalV85128">—</b></div><div class="decisionCell"><span>PE PC</span><b id="ddgDecisionLocalV85128">—</b></div><div class="decisionCell"><span>LIPSĂ</span><b id="ddgDecisionMissingV85128">—</b></div><div class="decisionCell"><span>SELECTATE</span><b id="ddgDecisionSelectedV85128">0</b></div><div class="decisionCell"><span>FOLDER</span><b id="ddgDecisionFolderV85128">—</b></div>';
    const hostList = card.querySelector('.providerHostList');
    if (hostList) hostList.insertAdjacentElement('beforebegin', bar);
    else card.appendChild(bar);
    return bar;
  }

  async function refreshDecision() {
    const bar = ensureDecisionBar();
    if (!bar) return;
    let data = null;
    try { data = await api('/api/results/summary'); } catch (_) {}
    const summary = data?.summary || data || {};
    const effective = summary?.effective || {};
    const total = Number(summary?.total || 0);
    const local = Number(effective.HAVE || 0) + Number(effective.VERIFIED || 0) + Number(effective.SAMPLED || 0);
    const missing = Number(effective.MISSING || 0);
    const folder = lastReport?.items?.[0]?.folder || '—';
    const set = (id, value, title = '') => {
      const el = document.getElementById(id);
      if (!el) return;
      el.textContent = value;
      if (title) el.title = title;
    };
    set('ddgDecisionTotalV85128', total ? total.toLocaleString('ro-RO') : '—');
    set('ddgDecisionLocalV85128', total ? local.toLocaleString('ro-RO') : '—');
    set('ddgDecisionMissingV85128', total ? missing.toLocaleString('ro-RO') : '—');
    set('ddgDecisionSelectedV85128', selectedCount().toLocaleString('ro-RO'));
    set('ddgDecisionFolderV85128', folder, folder === '—' ? '' : folder);
  }

  function scheduleDecisionRefresh() {
    clearTimeout(decisionTimer);
    decisionTimer = setTimeout(() => refreshDecision().catch(() => {}), 120);
  }

  function scheduleSourceRefresh(deep = false) {
    clearTimeout(sourceTimer);
    sourceTimer = setTimeout(() => {
      rowsCache = [];
      rowsAt = 0;
      candidateCache.clear();
      refreshPanel(ensureSourcePanel(), [], deep);
    }, 170);
  }

  function bindEvents() {
    if (document.documentElement.dataset.ddgFolderAdvisorV85128 === '1') return;
    document.documentElement.dataset.ddgFolderAdvisorV85128 = '1';

    window.addEventListener('ddg:source-scan-complete', () => scheduleSourceRefresh(false));
    window.addEventListener('ddg:source-history-updated', scheduleDecisionRefresh);
    window.addEventListener('ddg:folder-advisor-updated', scheduleDecisionRefresh);
    window.addEventListener('ddg:source-scan-error', scheduleDecisionRefresh);

    const input = document.getElementById('directUrl');
    input?.addEventListener('input', () => {
      sourceStamp = '';
      lastReport = null;
      const panel = ensureSourcePanel();
      if (panel) panel.innerHTML = '<div class="folderHintTitle"><b>Folder recomandat</b></div><div class="muted small">Scanează sursa pentru o recomandare bazată pe fișierele locale și istoric.</div>';
      scheduleDecisionRefresh();
    });

    document.addEventListener('click', event => {
      if (event.target?.closest?.('#ddgMediaPickerV8566 .mediaCard,#ddgMediaPickerClearV8566,#ddgMediaPickerRecommendedV8566,#ddgMediaPickerAllVisibleV8566,#ddgMediaPickerSendSelectedV8566,#ddgMediaPickerSendAllV8566')) scheduleDecisionRefresh();
      if (event.target?.closest?.('#tab-results,input[type="checkbox"],[data-row-id]')) scheduleDecisionRefresh();
    }, true);

    // Fallback for legacy/MEGA flows which do not always emit the universal event.
    const top = document.getElementById('topStatus');
    if (top && top.dataset.ddgFolderAdvisorTopV85128 !== '1') {
      top.dataset.ddgFolderAdvisorTopV85128 = '1';
      let lastDone = false;
      new MutationObserver(() => {
        const text = String(top.textContent || '').toLowerCase();
        const done = text.includes('analiză terminată') || text.includes('ultima operație: reușită') || text.includes('fișiere comparate');
        if (done && !lastDone) scheduleSourceRefresh(false);
        lastDone = done;
      }).observe(top, {childList:true, characterData:true, subtree:true});
    }
  }

  function boot() {
    ensureStyles();
    ensureSourcePanel();
    ensureDecisionBar();
    bindEvents();
    discoverLearningService().catch(() => {});
    scheduleDecisionRefresh();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
  setTimeout(boot, 650);

  window.ddgSourceFolderHintV85114 = {
    compute,
    refresh:() => refreshPanel(ensureSourcePanel(), [], false),
    deep:() => refreshPanel(ensureSourcePanel(), [], true),
    lastReport:() => lastReport,
    learningService:discoverLearningService,
    refreshDecision:scheduleDecisionRefresh
  };
})();
