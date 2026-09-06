// TEST hotfix: route the primary Download action exclusively to JDownloader
// when that engine is selected. ExactGuard otherwise replaces downloadSelected
// and can enqueue the file in DDG's own queue before the JD bridge gets a chance.
(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';

  function selectedEngineIsJD() {
    const select = document.getElementById('downloadMethod');
    const value = String(select?.value || (typeof cfg !== 'undefined' ? cfg?.downloadMethod : '') || '').toLowerCase();
    return value === 'jdownloader';
  }

  function idsSelected() {
    try {
      return typeof idsForAction === 'function' ? idsForAction().map(Number) : [];
    } catch (_) {
      return [];
    }
  }

  async function rowsForIDs(ids) {
    const wanted = new Set(ids.map(Number));
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 100 && wanted.size; page++) {
      const data = await api(`/api/results?offset=${offset}&limit=1000&status=ALL`);
      const batch = Array.isArray(data.rows) ? data.rows : [];
      for (const row of batch) {
        if (wanted.has(Number(row.id))) {
          rows.push(row);
          wanted.delete(Number(row.id));
        }
      }
      if (!batch.length || offset + batch.length >= Number(data.total || 0)) break;
      offset += batch.length;
    }
    return rows;
  }

  function gofileStableURL(row) {
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

  function jdURL(row) {
    const r = row?.remote || {};
    return gofileStableURL(row) || String(r.directUrl || r.url || '').trim();
  }

  async function jdownloaderRunning() {
    const check = window.ddgDownloadActionsV8545?.checkJDownloader;
    if (typeof check === 'function') {
      const state = await check();
      return Boolean(state?.running);
    }
    // Old-JD compatibility: jdcheck.js sets window.jdownloader=true and is not
    // blocked by fetch CORS because it is loaded as a script resource.
    return await new Promise(resolve => {
      const old = document.getElementById('ddgJDRouteCheck');
      old?.remove();
      window.jdownloader = false;
      const script = document.createElement('script');
      script.id = 'ddgJDRouteCheck';
      script.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let done = false;
      const finish = value => {
        if (done) return;
        done = true;
        script.remove();
        resolve(Boolean(value));
      };
      script.onload = () => finish(window.jdownloader === true);
      script.onerror = () => finish(false);
      document.head.appendChild(script);
      setTimeout(() => finish(window.jdownloader === true), 1800);
    });
  }

  function submitFlashGotForm(rows, destination) {
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

    let frame = document.getElementById('ddgJDFlashGotTarget');
    if (!frame) {
      frame = document.createElement('iframe');
      frame.id = 'ddgJDFlashGotTarget';
      frame.name = 'ddgJDFlashGotTarget';
      frame.style.display = 'none';
      document.body.appendChild(frame);
    }

    const form = document.createElement('form');
    form.method = 'POST';
    form.action = `${JD_BASE}/flashgot`;
    form.target = frame.name;
    form.style.display = 'none';

    const add = (name, value) => {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    };

    add('urls', urls.join('\n'));
    add('description', descriptions.join('\n'));
    add('package', 'Duplicate Download Guard');
    add('dir', destination);
    add('autostart', '1');

    document.body.appendChild(form);
    form.submit();
    setTimeout(() => form.remove(), 3000);
    return urls.length;
  }

  async function sendThroughJD() {
    const ids = idsSelected();
    if (!ids.length) return window.toast?.('Selectează fișiere');

    const destination = String(document.getElementById('downloadDir')?.value || (typeof cfg !== 'undefined' ? cfg?.downloadDir : '') || '').trim();
    if (!destination) return window.toast?.('Alege folderul de download în DDG.');

    const button = document.getElementById('downloadGuardBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Trimit în JDownloader…';
    }

    try {
      if (!(await jdownloaderRunning())) {
        throw new Error('JDownloader 2 nu răspunde pe 127.0.0.1:9666. Pornește JDownloader și încearcă din nou.');
      }

      const mode = document.getElementById('downloadGuardMode')?.value || (typeof cfg !== 'undefined' ? cfg?.downloadGuardMode : '') || 'smart';
      const guard = await api('/api/download/preflight', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, destination, mode })
      });
      const allowed = (guard.decisions || []).filter(x => x.verdict === 'DOWNLOAD').map(x => Number(x.resultId));
      if (!allowed.length) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, 0);
        return window.toast?.('Nimic de trimis în JDownloader: selecția este duplicat sau necesită verificare.');
      }

      const rows = await rowsForIDs(allowed);
      const sent = submitFlashGotForm(rows, destination);
      const c = guard?.counts || {};
      if (Number(c.DUPLICATE || 0) > 0 || Number(c.REVIEW || 0) > 0) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, sent);
      }
      window.toast?.(`Trimis exclusiv în JDownloader: ${sent} fișier(e) • ${destination}`);
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '⬇ Descarcă prin JDownloader';
      }
    }
  }

  // Capture-phase interception is intentional. The legacy button still has an
  // inline onclick="downloadSelected()" and ExactGuard replaces that function.
  // In JD mode this stops that handler before /api/queue/add can ever run.
  document.addEventListener('click', event => {
    if (!selectedEngineIsJD()) return;
    const button = event.target?.closest?.('#downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!button) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendThroughJD();
  }, true);

  function refreshLabel() {
    if (!selectedEngineIsJD()) return;
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (button && !button.disabled) {
      button.textContent = '⬇ Descarcă prin JDownloader';
      button.title = 'Trimite selecția exclusiv către JDownloader 2. Coada internă DDG nu este folosită.';
    }
  }

  function boot() {
    const timer = setInterval(() => {
      const select = document.getElementById('downloadMethod');
      if (!select) return;
      clearInterval(timer);
      select.addEventListener('change', () => setTimeout(refreshLabel, 0));
      refreshLabel();
    }, 100);
    setTimeout(() => clearInterval(timer), 10000);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, { once: true });
  else boot();

  window.ddgJDownloaderRouteHotfixV8549 = { sendThroughJD };
})();
