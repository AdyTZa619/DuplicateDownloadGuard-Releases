// TEST v8.5.114 — source-to-local-folder advisor.
// Uses CURRENT DDG results/candidates only. No HDD rescan and no JDownloader monkey-patching.
(() => {
  'use strict';

  const SOURCE_PANEL_ID = 'ddgSourceFolderHintV85114';
  const PICKER_PANEL_ID = 'ddgPickerFolderHintV85114';
  const JD_PANEL_ID = 'ddgJDFolderHintV85114';
  let rowsCache = [];
  let rowsAt = 0;
  let sourceStamp = '';
  let computeSeq = 0;
  const candidateCache = new Map();

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

  async function api(url) {
    if (typeof window.api === 'function') return window.api(url);
    const response = await fetch(url, {cache:'no-store'});
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
    return response.json();
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

  function bucketFolder(filePath) {
    const parent = dirname(filePath);
    if (!parent) return '';
    const localRoots = roots();
    for (const root of localRoots) {
      if (!isUnder(parent, root)) continue;
      if (localRoots.length > 1) return root;
      const rel = normalizePath(parent).slice(normalizePath(root).length).replace(/^\\+/, '');
      if (!rel) return root;
      const first = rel.split('\\').filter(Boolean)[0];
      return first ? `${normalizePath(root)}\\${first}` : root;
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

  function addScore(map, path, points, kind, fileName = '') {
    const folder = bucketFolder(path);
    if (!folder) return;
    const key = folder.toLowerCase();
    let x = map.get(key);
    if (!x) {
      x = {folder, score:0, strong:0, linked:0, candidates:0, examples:[]};
      map.set(key, x);
    }
    x.score += points;
    if (kind === 'strong') x.strong++;
    else if (kind === 'linked') x.linked++;
    else x.candidates++;
    if (fileName && x.examples.length < 3 && !x.examples.includes(fileName)) x.examples.push(fileName);
  }

  function reportConfidence(items) {
    if (!items.length) return 'fără dovezi';
    const top = items[0];
    const second = items[1]?.score || 0;
    if (top.strong >= 3 && top.score >= second * 1.6) return 'ridicată';
    if (top.strong >= 1 || top.score >= second * 1.25) return 'medie';
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
      addScore(scores, local, strong ? 5 : 2, strong ? 'strong' : 'linked', row?.remote?.name || row?.remote?.path || '');
    }

    if (deep) {
      const unresolved = rows.filter(row => !row?.localPath && Number(row?.candidates || 0) > 0).slice(0, 24);
      for (const row of unresolved) {
        if (seq !== computeSeq) return null;
        const list = await candidatesFor(row);
        list.slice(0, 3).forEach((candidate, rank) => {
          const quality = Math.max(0, Math.min(99, Number(candidate?.matchScore || 0)));
          if (quality < 42) return;
          const weight = (1.4 - rank * 0.28) * (0.55 + quality / 200);
          addScore(scores, candidate.path, weight, 'candidate', row?.remote?.name || row?.remote?.path || '');
        });
      }
    }

    const items = [...scores.values()].sort((a,b) => b.score - a.score || b.strong - a.strong || a.folder.localeCompare(b.folder));
    return {
      source: currentSource(),
      rows: rows.length,
      deep,
      confidence: reportConfidence(items),
      items: items.slice(0, 3)
    };
  }

  function copyText(text) {
    const value = String(text || '');
    if (!value) return;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(value).then(() => window.toast?.('Calea folderului a fost copiată')).catch(() => fallbackCopy(value));
      return;
    }
    fallbackCopy(value);
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

  function resultHTML(report, compact = false) {
    if (!report) return '<span class="muted">Analiza folderului a fost întreruptă.</span>';
    if (!report.items.length) {
      return `<div class="folderHintTitle"><b>Folder legat de sursă</b><span class="sourcePill">${report.rows} rezultate</span></div><div class="muted small">Nu există încă suficiente legături locale. Folosește „Analizează candidații” pentru o estimare mai largă.</div>`;
    }
    const top = report.items[0];
    const detail = `${top.strong} potriviri puternice • ${top.linked} asocieri • ${top.candidates} indicii din candidați`;
    const alternatives = report.items.slice(1).map(x => `<div class="folderHintAlt"><code>${esc(x.folder)}</code><span>${x.strong + x.linked + x.candidates} legături</span></div>`).join('');
    return `<div class="folderHintTitle"><b>Folder recomandat pentru sursa asta</b><span class="badge ${report.confidence === 'ridicată' ? 'VERIFIED' : 'POSSIBLE'}">${esc(report.confidence.toUpperCase())}</span></div><div class="folderHintTop"><code title="${esc(top.folder)}">${esc(top.folder)}</code><button class="btn folderHintCopyV85114" type="button" data-folder="${esc(top.folder)}">Copiază calea</button></div><div class="muted small">${esc(detail)}${report.deep ? ' • analiză cu candidați' : ''}</div>${compact ? '' : alternatives}`;
  }

  function wirePanel(panel) {
    panel.querySelectorAll('.folderHintCopyV85114').forEach(button => {
      button.addEventListener('click', event => {
        event.preventDefault();
        event.stopPropagation();
        copyText(button.dataset.folder || '');
      });
    });
  }

  function setPanel(panel, report, compact = false) {
    if (!panel) return;
    panel.innerHTML = resultHTML(report, compact);
    wirePanel(panel);
  }

  async function refreshPanel(panel, ids = [], deep = false, compact = false) {
    if (!panel) return;
    panel.innerHTML = `<span class="muted small">${deep ? 'Analizez și candidații locali…' : 'Calculez folderul cel mai legat de sursă…'}</span>`;
    try {
      const report = await compute(ids, deep);
      if (report) setPanel(panel, report, compact);
    } catch (error) {
      panel.innerHTML = `<span class="muted small">Folder advisor: ${esc(error?.message || String(error))}</span>`;
    }
  }

  function ensureStyles() {
    if (document.getElementById('ddgSourceFolderHintStyleV85114')) return;
    const style = document.createElement('style');
    style.id = 'ddgSourceFolderHintStyleV85114';
    style.textContent = `
      .ddgFolderHintV85114{margin-top:9px;padding:10px 11px;border:1px solid #304256;border-radius:9px;background:#0b131c;min-width:0}
      .ddgFolderHintV85114 .folderHintTitle{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:7px}
      .ddgFolderHintV85114 .folderHintTop{display:flex;gap:8px;align-items:center;min-width:0}.ddgFolderHintV85114 code{font-family:Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#c7d9ea;min-width:0;flex:1}
      .ddgFolderHintV85114 .folderHintAlt{display:flex;gap:8px;align-items:center;margin-top:6px;padding-top:6px;border-top:1px solid #1e2b38}.ddgFolderHintV85114 .folderHintAlt span{font-size:10px;color:#8fa5ba;white-space:nowrap}
      .ddgFolderHintToolsV85114{display:flex;gap:7px;margin-top:7px;flex-wrap:wrap}
    `;
    document.head.appendChild(style);
  }

  function ensureSourcePanel() {
    ensureStyles();
    let panel = document.getElementById(SOURCE_PANEL_ID);
    if (panel) return panel;
    const anchor = document.getElementById('ddgMediaPickerLaunchV8566') || document.getElementById('universalProviderState');
    if (!anchor?.parentElement) return null;
    panel = document.createElement('div');
    panel.id = SOURCE_PANEL_ID;
    panel.className = 'ddgFolderHintV85114';
    panel.innerHTML = '<div class="folderHintTitle"><b>Folder legat de sursă</b></div><div class="muted small">După scanare pot arăta unde ai deja cel mai mult conținut legat de sursa curentă.</div><div class="ddgFolderHintToolsV85114"><button class="btn" id="ddgFolderHintQuickV85114" type="button">Găsește folderul</button><button class="btn" id="ddgFolderHintDeepV85114" type="button">Analizează candidații</button></div>';
    anchor.insertAdjacentElement('afterend', panel);
    panel.querySelector('#ddgFolderHintQuickV85114')?.addEventListener('click', () => refreshPanel(panel, [], false));
    panel.querySelector('#ddgFolderHintDeepV85114')?.addEventListener('click', () => refreshPanel(panel, [], true));
    return panel;
  }

  function pickerIDs() {
    return [...document.querySelectorAll('#ddgMediaPickerV8566 .mediaCard.on[data-picker-id]')]
      .map(card => Number(card.dataset.pickerId)).filter(Number.isFinite);
  }

  function ensurePickerPanel() {
    const modal = document.getElementById('ddgMediaPickerV8566');
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (!modal || !grid) return null;
    let panel = document.getElementById(PICKER_PANEL_ID);
    if (!panel) {
      panel = document.createElement('div');
      panel.id = PICKER_PANEL_ID;
      panel.className = 'ddgFolderHintV85114';
      panel.style.margin = '9px 14px 0';
      grid.insertAdjacentElement('beforebegin', panel);
    }
    return panel;
  }

  function mainIDs() {
    try {
      return typeof window.idsForAction === 'function' ? window.idsForAction().map(Number).filter(Number.isFinite) : [];
    } catch (_) { return []; }
  }

  function ensureJDPanel() {
    const body = document.querySelector('#ddgJDFastDecisionV8567 .body');
    if (!body) return null;
    let panel = document.getElementById(JD_PANEL_ID);
    if (!panel) {
      panel = document.createElement('div');
      panel.id = JD_PANEL_ID;
      panel.className = 'ddgFolderHintV85114';
      body.appendChild(panel);
    }
    return panel;
  }

  function installObservers() {
    const top = document.getElementById('topStatus');
    if (top && top.dataset.ddgFolderHintWatchV85114 !== '1') {
      top.dataset.ddgFolderHintWatchV85114 = '1';
      new MutationObserver(() => {
        const text = String(top.textContent || '').toLowerCase();
        if (!text.includes('analiză terminată') && !text.includes('gata')) return;
        rowsCache = [];
        rowsAt = 0;
        candidateCache.clear();
        setTimeout(() => refreshPanel(ensureSourcePanel(), [], false), 120);
      }).observe(top, {childList:true, characterData:true,subtree:true});
    }

    const bodyObserver = new MutationObserver(() => {
      const picker = document.getElementById('ddgMediaPickerV8566');
      if (picker && !picker.classList.contains('hidden')) {
        const ids = pickerIDs();
        refreshPanel(ensurePickerPanel(), ids, false, true);
      }
      const jd = document.getElementById('ddgJDFastDecisionV8567');
      if (jd && !jd.classList.contains('hidden')) refreshPanel(ensureJDPanel(), mainIDs(), false, true);
    });
    bodyObserver.observe(document.body, {childList:true, subtree:true, attributes:true, attributeFilter:['class']});

    document.addEventListener('click', event => {
      if (event.target?.closest?.('#ddgMediaPickerV8566 .mediaCard')) {
        setTimeout(() => refreshPanel(ensurePickerPanel(), pickerIDs(), false, true), 40);
      }
      if (event.target?.closest?.('#ddgMediaPickerSendSelectedV8566,#ddgMediaPickerSendAllV8566')) {
        const ids = event.target.closest('#ddgMediaPickerSendAllV8566') ? [] : pickerIDs();
        refreshPanel(ensurePickerPanel(), ids, false, true);
      }
    }, true);
  }

  function boot() {
    ensureSourcePanel();
    installObservers();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
  setTimeout(() => ensureSourcePanel(), 700);
  window.ddgSourceFolderHintV85114 = {compute, refresh:() => refreshPanel(ensureSourcePanel(), [], false), deep:() => refreshPanel(ensureSourcePanel(), [], true)};
})();
