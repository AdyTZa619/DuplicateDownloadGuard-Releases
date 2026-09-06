// Final JDownloader routing for TEST builds.
// This path never calls the DDG download queue. It verifies the selection,
// then hands approved links to JDownloader and lets JDownloader decide package,
// destination folder and start behavior from its own settings/rules.
(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';
  let busy = false;
  let pendingReview = null;

  function liveEngine() {
    const select = document.getElementById('downloadMethod');
    return String(select?.value || '').trim().toLowerCase();
  }

  function ensureEngineOption() {
    const select = document.getElementById('downloadMethod');
    if (!select) return false;
    if (!select.querySelector('option[value="jdownloader"]')) {
      const option = document.createElement('option');
      option.value = 'jdownloader';
      option.textContent = 'JDownloader 2 — direct';
      select.appendChild(option);
    }
    if (String(window.cfg?.downloadMethod || '').toLowerCase() === 'jdownloader') {
      select.value = 'jdownloader';
    }
    return true;
  }

  function primaryButton() {
    return document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
  }

  function updateLabel() {
    const button = primaryButton();
    if (!button || busy) return;
    if (liveEngine() === 'jdownloader') {
      button.textContent = '⬇ Trimite în JDownloader';
      button.title = 'DDG trimite doar linkurile. JDownloader decide colecția/pachetul, folderul și pornirea după propriile reguli.';
    } else if (button.textContent.includes('JDownloader')) {
      button.textContent = '⬇ Descarcă';
    }
  }

  function selectedIDs() {
    try {
      return typeof window.idsForAction === 'function'
        ? window.idsForAction().map(Number).filter(Number.isFinite)
        : [];
    } catch (_) {
      return [];
    }
  }

  // Used only by ExactGuard to know which local folders to scan. This value is
  // deliberately NOT sent to JDownloader as a destination.
  function guardDestination() {
    return String(document.getElementById('downloadDir')?.value || window.cfg?.downloadDir || '').trim();
  }

  function guardMode() {
    return document.getElementById('downloadGuardMode')?.value || window.cfg?.downloadGuardMode || 'smart';
  }

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
      if (parts.length >= 2 && parts[0].toLowerCase() === 'd' && parts[1]) {
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
      // gallery-dl exposes Bunkr's public media slug. Hand the public /f/ page
      // to JDownloader so its Bunkr plugin resolves fresh CDN URL + Referer on
      // its own. Sending DDG's temporary CDN URL directly loses that context.
      return `${album.origin}/f/${encodeURIComponent(String(r.handle))}`;
    } catch (_) {
      return '';
    }
  }

  function cyberdropURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'CYBERDROP' || !r.providerId) return '';
    try {
      const album = new URL(r.url || '');
      // The maintained gallery-dl Cyberdrop extractor resolves files through
      // the public /f/<id> page, then obtains a fresh auth/CDN URL from the
      // Cyberdrop API. Hand the stable public media page to JDownloader instead
      // of DDG's expiring signed CDN URL.
      return `${album.origin}/f/${encodeURIComponent(String(r.providerId))}`;
    } catch (_) {
      return '';
    }
  }

  function jdURL(row) {
    const r = row?.remote || {};
    return gofileURL(row) || bunkrURL(row) || cyberdropURL(row) || String(r.directUrl || r.url || '').trim();
  }

  async function checkJD() {
    if (window.ddgDownloadActionsV8545?.checkJDownloader) {
      return window.ddgDownloadActionsV8545.checkJDownloader();
    }
    return await new Promise(resolve => {
      document.getElementById('ddgFinalJDCheck')?.remove();
      window.jdownloader = false;
      const s = document.createElement('script');
      s.id = 'ddgFinalJDCheck';
      s.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let done = false;
      const finish = running => {
        if (done) return;
        done = true;
        s.remove();
        resolve({running: Boolean(running), name: 'JDownloader 2'});
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

    const params = new URLSearchParams();
    params.set('urls', urls.join('\n'));
    params.set('description', descriptions.join('\n'));

    // IMPORTANT: do not send package, dir or autostart. Those fields were the
    // reason JD inherited DDG's default folder and started immediately. With
    // only the links supplied, JDownloader/LinkGrabber is free to apply its own
    // Packagizer rules, package/collection naming, download directory and start
    // policy exactly as if the user had added the links in JDownloader itself.
    return {params, count: urls.length};
  }

  function submitHiddenForm(params) {
    const frameName = `ddgJDFrame_${Date.now()}`;
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
    setTimeout(() => {
      form.remove();
      iframe.remove();
    }, 4000);
  }

  async function submitRowsToJD(rows) {
    const jd = await checkJD();
    if (!jd?.running) {
      throw new Error('JDownloader 2 nu răspunde pe 127.0.0.1:9666. DDG nu a pornit niciun download intern.');
    }

    const submission = buildSubmission(rows);
    try {
      const response = await fetch(`${JD_BASE}/flashgot`, {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8'},
        body: submission.params.toString(),
        cache: 'no-store'
      });
      const reply = (await response.text()).trim();
      if (!response.ok || /(^|\s)failed(\s|$)/i.test(reply)) {
        throw new Error(reply || `HTTP ${response.status}`);
      }
      return {count: submission.count, confirmed: true};
    } catch (_) {
      // Unele WebView/Windows builds blochează răspunsul cross-origin către
      // portul 9666. JD a fost verificat înainte, deci folosim POST de formular,
      // care nu poate cădea în downloaderul intern DDG.
      submitHiddenForm(submission.params);
      return {count: submission.count, confirmed: false};
    }
  }

  async function preflight(ids, dest, mode) {
    return window.api('/api/download/preflight', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({ids, destination: dest, mode})
    });
  }

  function IDsByVerdict(report, verdict) {
    return (report?.decisions || [])
      .filter(x => String(x?.verdict || '').toUpperCase() === verdict)
      .map(x => Number(x.resultId))
      .filter(Number.isFinite);
  }

  function showReview(report, ids, dest, mode, sent) {
    pendingReview = {
      ids: IDsByVerdict(report, 'REVIEW'),
      guardDestination: dest,
      guardMode: mode
    };
    window.ddgShowGuardReportV8545?.(report, {
      ids,
      destination: dest,
      guardMode: mode,
      engine: 'jdownloader'
    }, sent);
    const override = document.getElementById('guardReviewOverride');
    if (override && pendingReview.ids.length) {
      override.classList.remove('hidden');
      override.textContent = `Trimite oricum în JDownloader (${pendingReview.ids.length})`;
      override.title = 'Confirmare explicită: DDG trimite doar linkurile; JDownloader decide colecția, folderul și pornirea.';
    }
  }

  async function sendReviewOverride() {
    if (!pendingReview?.ids?.length || busy) return;
    busy = true;
    const override = document.getElementById('guardReviewOverride');
    if (override) {
      override.disabled = true;
      override.textContent = '⏳ Trimit în JDownloader…';
    }
    try {
      const rows = await rowsForIDs(pendingReview.ids);
      const result = await submitRowsToJD(rows);
      const suffix = result.confirmed ? 'confirmat de JD' : 'trimis prin interfața locală JD';
      window.closeGuardReport?.();
      window.toast?.(`JDownloader: ${result.count} fișier(e) • ${suffix} • folder/pachet gestionate de JD`);
      pendingReview = null;
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
      if (override) override.disabled = false;
      updateLabel();
    }
  }

  async function sendExclusiveToJDownloader() {
    if (busy) return;
    const ids = selectedIDs();
    if (!ids.length) return window.toast?.('Selectează fișiere');
    const dest = guardDestination();
    const mode = guardMode();
    const button = primaryButton();

    busy = true;
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Verific și trimit în JDownloader…';
    }
    try {
      const report = await preflight(ids, dest, mode);
      const safeIDs = IDsByVerdict(report, 'DOWNLOAD');
      const reviewIDs = IDsByVerdict(report, 'REVIEW');
      let sent = 0;
      let confirmed = true;

      if (safeIDs.length) {
        const rows = await rowsForIDs(safeIDs);
        const result = await submitRowsToJD(rows);
        sent = result.count;
        confirmed = result.confirmed;
      }

      if (reviewIDs.length || IDsByVerdict(report, 'DUPLICATE').length) {
        showReview(report, ids, dest, mode, sent);
      } else {
        pendingReview = null;
      }

      if (sent > 0) {
        window.toast?.(`JDownloader: ${sent} fișier(e) ${confirmed ? 'confirmate' : 'trimise'} • JD decide colecția/folderul/pornirea`);
      } else if (!reviewIDs.length) {
        window.toast?.('Nimic de trimis în JDownloader: selecția este deja locală/blocată.');
      }
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
      if (button) button.disabled = false;
      updateLabel();
    }
  }

  function install() {
    if (!ensureEngineOption()) {
      setTimeout(install, 100);
      return;
    }
    const select = document.getElementById('downloadMethod');
    if (select && !select.dataset.ddgFinalJdBound) {
      select.dataset.ddgFinalJdBound = '1';
      select.addEventListener('change', () => {
        if (window.cfg) window.cfg.downloadMethod = select.value;
        if (typeof window.saveCfg === 'function') window.saveCfg().catch(() => {});
        setTimeout(updateLabel, 0);
      });
    }
    updateLabel();
    window.sendSelectedJD2 = sendExclusiveToJDownloader;
  }

  // Capture phase: the JD action never reaches legacy onclick handlers or the
  // DDG queue. The explicit JD buttons work even if another engine is selected;
  // the primary Download button uses JD only when JD is the selected engine.
  document.addEventListener('click', event => {
    const target = event.target?.closest?.('#guardReviewOverride, #jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;

    if (target.id === 'guardReviewOverride' && pendingReview?.ids?.length) {
      event.preventDefault();
      event.stopImmediatePropagation();
      sendReviewOverride();
      return;
    }

    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;

    event.preventDefault();
    event.stopImmediatePropagation();
    sendExclusiveToJDownloader();
  }, true);

  document.addEventListener('DOMContentLoaded', () => setTimeout(install, 0), {once: true});
  setTimeout(install, 600);

  window.ddgJDownloaderFinalV8551 = {
    sendExclusiveToJDownloader,
    sendReviewOverride,
    install
  };
})();
