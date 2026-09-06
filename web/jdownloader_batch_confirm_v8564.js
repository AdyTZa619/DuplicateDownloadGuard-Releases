// Batch-aware JDownloader handoff for TEST builds.
// If ExactGuard finds REVIEW/DUPLICATE items, nothing is sent automatically:
// the report lets the user send only recommended files or explicitly confirm
// sending the entire original selection anyway. Every send is one FlashGot
// submission with one package; DDG still does not force dir or autostart.
(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';
  let busy = false;
  let pending = null;

  const liveEngine = () => String(document.getElementById('downloadMethod')?.value || '').trim().toLowerCase();
  const selectedIDs = () => {
    try {
      return typeof window.idsForAction === 'function'
        ? window.idsForAction().map(Number).filter(Number.isFinite)
        : [];
    } catch (_) { return []; }
  };
  const guardDestination = () => String(document.getElementById('downloadDir')?.value || window.cfg?.downloadDir || '').trim();
  const guardMode = () => document.getElementById('downloadGuardMode')?.value || window.cfg?.downloadGuardMode || 'smart';

  async function rowsForIDs(ids) {
    const wanted = new Set(ids.map(Number));
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 100 && wanted.size; page++) {
      const data = await window.api(`/api/results?offset=${offset}&limit=1000&status=ALL`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      for (const row of batch) {
        const id = Number(row?.id);
        if (wanted.has(id)) {
          rows.push(row);
          wanted.delete(id);
        }
      }
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    if (wanted.size) throw new Error(`Nu mai găsesc ${wanted.size} rezultat(e) selectate.`);
    return rows;
  }

  function gofileURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'GOFILE' || !r.providerId) return '';
    try {
      const u = new URL(r.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      if (parts.length >= 2 && parts[0].toLowerCase() === 'd') {
        return `https://gofile.io/?c=${encodeURIComponent(parts[1])}#file=${encodeURIComponent(r.providerId)}`;
      }
    } catch (_) {}
    return '';
  }

  function bunkrURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'BUNKR' || !r.handle) return '';
    try {
      const album = new URL(r.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(r.handle))}`;
    } catch (_) { return ''; }
  }

  function cyberdropURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'CYBERDROP' || !r.providerId) return '';
    try {
      const album = new URL(r.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(r.providerId))}`;
    } catch (_) { return ''; }
  }

  function jdURL(row) {
    const r = row?.remote || {};
    return gofileURL(row) || bunkrURL(row) || cyberdropURL(row) || String(r.directUrl || r.url || '').trim();
  }

  function cleanPackageName(value) {
    return String(value || '')
      .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, 120);
  }

  function onePackageName(rows) {
    const paths = rows.map(r => String(r?.remote?.path || '').replace(/\\/g, '/')).filter(Boolean);
    const parents = paths.map(p => p.split('/').filter(Boolean).slice(0, -1));
    if (parents.length && parents.every(p => p.length)) {
      const first = parents[0];
      const common = [];
      for (let i = 0; i < first.length; i++) {
        if (parents.every(p => p[i] === first[i])) common.push(first[i]);
        else break;
      }
      if (common.length) {
        const name = cleanPackageName(common.join(' - '));
        if (name) return name;
      }
    }

    const first = rows[0]?.remote || {};
    const provider = String(first.source || 'DDG').toUpperCase();
    try {
      const u = new URL(first.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      const id = parts.length ? parts[parts.length - 1] : u.hostname;
      const name = cleanPackageName(`${provider} - ${decodeURIComponent(id)}`);
      if (name) return name;
    } catch (_) {}
    return 'DDG - selecție';
  }

  async function checkJD() {
    if (window.ddgDownloadActionsV8545?.checkJDownloader) {
      return window.ddgDownloadActionsV8545.checkJDownloader();
    }
    return await new Promise(resolve => {
      document.getElementById('ddgBatchJDCheck')?.remove();
      window.jdownloader = false;
      const s = document.createElement('script');
      s.id = 'ddgBatchJDCheck';
      s.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let done = false;
      const finish = running => {
        if (done) return;
        done = true;
        s.remove();
        resolve({running: Boolean(running)});
      };
      s.onload = () => finish(window.jdownloader === true);
      s.onerror = () => finish(false);
      document.head.appendChild(s);
      setTimeout(() => finish(window.jdownloader === true), 2200);
    });
  }

  function buildSubmission(rows) {
    const urls = [];
    const descriptions = [];
    const seen = new Set();
    for (const row of rows) {
      const url = jdURL(row);
      if (!url || seen.has(url)) continue;
      seen.add(url);
      urls.push(url);
      descriptions.push(row?.remote?.name || row?.remote?.path || 'DDG');
    }
    if (!urls.length) throw new Error('Selecția nu conține linkuri compatibile cu JDownloader.');
    const packageName = onePackageName(rows);
    const params = new URLSearchParams();
    params.set('urls', urls.join('\n'));
    params.set('descriptions', descriptions.join('\n'));
    params.set('package', packageName);
    // Deliberately omit dir + autostart. Only the grouping package is forced.
    return {params, count: urls.length, packageName};
  }

  function submitHiddenForm(params) {
    const frameName = `ddgJDBatch_${Date.now()}`;
    const iframe = document.createElement('iframe');
    iframe.name = frameName;
    iframe.style.display = 'none';
    document.body.appendChild(iframe);
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = `${JD_BASE}/flashgot`;
    form.target = frameName;
    form.style.display = 'none';
    for (const [name, value] of params.entries()) {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    }
    document.body.appendChild(form);
    form.submit();
    setTimeout(() => { form.remove(); iframe.remove(); }, 4000);
  }

  async function submitRows(rows) {
    const jd = await checkJD();
    if (!jd?.running) throw new Error('JDownloader 2 nu răspunde pe 127.0.0.1:9666.');
    const submission = buildSubmission(rows);
    try {
      const response = await fetch(`${JD_BASE}/flashgot`, {
        method:'POST',
        headers:{'Content-Type':'application/x-www-form-urlencoded;charset=UTF-8'},
        body:submission.params.toString(),
        cache:'no-store'
      });
      const reply = (await response.text()).trim();
      if (!response.ok || /(^|\s)failed(\s|$)/i.test(reply)) throw new Error(reply || `HTTP ${response.status}`);
      return {...submission, confirmed:true};
    } catch (_) {
      submitHiddenForm(submission.params);
      return {...submission, confirmed:false};
    }
  }

  async function preflight(ids, dest, mode) {
    return window.api('/api/download/preflight', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ids, destination:dest, mode})
    });
  }

  function idsByVerdict(report, verdict) {
    return (report?.decisions || [])
      .filter(x => String(x?.verdict || '').toUpperCase() === verdict)
      .map(x => Number(x.resultId))
      .filter(Number.isFinite);
  }

  function ensureSafeOnlyButton() {
    let b = document.getElementById('guardJDSafeOnly');
    if (b) return b;
    const override = document.getElementById('guardReviewOverride');
    if (!override?.parentElement) return null;
    b = document.createElement('button');
    b.className = 'btn';
    b.id = 'guardJDSafeOnly';
    override.parentElement.insertBefore(b, override);
    return b;
  }

  function showDecision(report, ids) {
    const safe = idsByVerdict(report, 'DOWNLOAD');
    const review = idsByVerdict(report, 'REVIEW');
    const duplicates = idsByVerdict(report, 'DUPLICATE');
    pending = {all: ids.slice(), safe, review, duplicates};

    window.ddgShowGuardReportV8545?.(report, {
      ids,
      destination:guardDestination(),
      guardMode:guardMode(),
      engine:'jdownloader'
    }, 0);

    const info = document.getElementById('guardScanInfo');
    if (info) {
      info.innerHTML += `<br><b>JDownloader:</b> nimic nu a fost trimis încă. Poți respecta verdictul DDG sau poți confirma explicit trimiterea întregii selecții.`;
    }

    const allBtn = document.getElementById('guardReviewOverride');
    if (allBtn) {
      allBtn.classList.remove('hidden');
      allBtn.disabled = false;
      allBtn.textContent = `Trimite TOATE oricum în JDownloader (${ids.length})`;
      allBtn.title = `Include și ${duplicates.length} duplicate + ${review.length} de verificat. Va cere confirmare explicită.`;
    }

    const safeBtn = ensureSafeOnlyButton();
    if (safeBtn) {
      safeBtn.classList.toggle('hidden', safe.length === 0);
      safeBtn.disabled = false;
      safeBtn.textContent = `Trimite doar recomandate (${safe.length})`;
      safeBtn.title = 'Trimite numai fișierele pe care DDG le-a marcat DESCARCĂ.';
    }
  }

  async function sendIDs(ids, closeModal = true) {
    if (busy || !ids.length) return;
    busy = true;
    try {
      const rows = await rowsForIDs(ids);
      const result = await submitRows(rows);
      if (closeModal) window.closeGuardReport?.();
      window.toast?.(`JDownloader: ${result.count} fișier(e) într-un singur pachet „${result.packageName}”`);
      pending = null;
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
    }
  }

  async function confirmAll() {
    if (!pending?.all?.length || busy) return;
    const blocked = pending.duplicates.length;
    const review = pending.review.length;
    const ok = window.confirm(
      `DDG a găsit ${blocked} fișier(e) pe care spune că le ai deja și ${review} de verificat.\n\nConfirmi că vrei să trimiți TOATE cele ${pending.all.length} fișiere în JDownloader oricum?`
    );
    if (!ok) return;
    await sendIDs(pending.all);
  }

  async function sendSafeOnly() {
    if (!pending?.safe?.length || busy) return;
    await sendIDs(pending.safe);
  }

  async function sendBatchAware() {
    if (busy) return;
    const ids = selectedIDs();
    if (!ids.length) return window.toast?.('Selectează fișiere');
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    busy = true;
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Verific înainte de JDownloader…';
    }
    try {
      const report = await preflight(ids, guardDestination(), guardMode());
      const review = idsByVerdict(report, 'REVIEW');
      const duplicates = idsByVerdict(report, 'DUPLICATE');
      if (review.length || duplicates.length) {
        showDecision(report, ids);
      } else {
        const rows = await rowsForIDs(ids);
        const result = await submitRows(rows);
        window.toast?.(`JDownloader: ${result.count} fișier(e) într-un singur pachet „${result.packageName}”`);
      }
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
      if (button) {
        button.disabled = false;
        button.textContent = liveEngine() === 'jdownloader' ? '⬇ Trimite în JDownloader' : '⬇ Descarcă';
      }
    }
  }

  // Loaded before jdownloader_final_v8551.js. Capture first and stop the legacy
  // handler so a mixed batch cannot be split into multiple JD submissions.
  document.addEventListener('click', event => {
    const target = event.target?.closest?.('#guardReviewOverride, #guardJDSafeOnly, #jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;

    if (target.id === 'guardReviewOverride' && pending?.all?.length) {
      event.preventDefault();
      event.stopImmediatePropagation();
      confirmAll();
      return;
    }
    if (target.id === 'guardJDSafeOnly' && pending?.safe?.length) {
      event.preventDefault();
      event.stopImmediatePropagation();
      sendSafeOnly();
      return;
    }

    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendBatchAware();
  }, true);

  // Also win over programmatic calls after the older module finishes installing.
  setTimeout(() => { window.sendSelectedJD2 = sendBatchAware; }, 1000);

  window.ddgJDownloaderBatchConfirmV8564 = {sendBatchAware, confirmAll, sendSafeOnly, onePackageName};
})();
