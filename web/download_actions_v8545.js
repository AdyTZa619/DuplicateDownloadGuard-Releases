(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';

  function counts(report) {
    const c = report?.counts || {};
    return {
      duplicate: Number(c.DUPLICATE || 0),
      review: Number(c.REVIEW || 0),
      download: Number(c.DOWNLOAD || 0)
    };
  }

  function queueNeedsAttention(data) {
    const c = counts(data?.guard);
    return c.duplicate > 0 || c.review > 0 || (Array.isArray(data?.rejected) && data.rejected.length > 0);
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

  function gofileJDURL(row) {
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

  function jdURLForRow(row) {
    const r = row?.remote || {};
    return gofileJDURL(row) || String(r.directUrl || r.url || '').trim();
  }

  async function fetchWithTimeout(url, options = {}, timeoutMs = 1800) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      return await fetch(url, { ...options, signal: controller.signal, cache: 'no-store' });
    } finally {
      clearTimeout(timer);
    }
  }

  async function checkJDownloader() {
    // Current JDownloader exposes this JSON endpoint with permissive CORS/PNA.
    // It gives us a real request/response handshake instead of merely assuming
    // that a hidden form reached port 9666.
    try {
      const response = await fetchWithTimeout(`${JD_BASE}/jdcheck.json?_ddg=${Date.now()}`, {}, 1800);
      if (response.ok) {
        const text = await response.text();
        try {
          const data = JSON.parse(text);
          return {
            running: true,
            name: String(data?.name || 'JDownloader 2'),
            version: String(data?.version || '')
          };
        } catch (_) {
          return { running: true, name: 'JDownloader 2', version: '' };
        }
      }
    } catch (_) {}

    // Compatibility fallback for older JD builds: jdcheck.js is executable
    // cross-origin and sets window.jdownloader=true.
    return await new Promise(resolve => {
      document.getElementById('ddgJDCheckScript')?.remove();
      window.jdownloader = false;
      const script = document.createElement('script');
      script.id = 'ddgJDCheckScript';
      script.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let finished = false;
      const finish = value => {
        if (finished) return;
        finished = true;
        script.remove();
        resolve({ running: Boolean(value), name: 'JDownloader 2', version: '' });
      };
      script.onload = () => finish(window.jdownloader === true);
      script.onerror = () => finish(false);
      document.head.appendChild(script);
      setTimeout(() => finish(window.jdownloader === true), 1800);
    });
  }

  function buildJDSubmission(rows, destination, autoStart) {
    const urls = [];
    const descriptions = [];
    const seen = new Set();
    for (const row of rows) {
      const url = jdURLForRow(row);
      if (!url || seen.has(url)) continue;
      seen.add(url);
      urls.push(url);
      descriptions.push(row?.remote?.name || row?.remote?.path || 'DDG');
    }
    if (!urls.length) throw new Error('Selecția nu conține linkuri compatibile cu JDownloader.');

    const dir = String(destination || '').trim();
    if (!dir) {
      throw new Error('Alege folderul de download în DDG. JDownloader nu va primi o destinație implicită sau aproximată.');
    }

    const body = new URLSearchParams();
    body.set('urls', urls.join('\n'));
    body.set('description', descriptions.join('\n'));
    body.set('dir', dir);
    // JDownloader ExternInterfaceImpl interprets autostart=1 as both
    // auto-confirm and auto-start. This turns JD into the actual download
    // engine instead of leaving links in LinkGrabber and relying on JD rules.
    body.set('autostart', autoStart ? '1' : '0');
    return { body, count: urls.length, destination: dir };
  }

  async function submitJDownloaderDirect(rows, destination, autoStart = true) {
    const submission = buildJDSubmission(rows, destination, autoStart);
    let response;
    try {
      response = await fetchWithTimeout(`${JD_BASE}/flashgot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
        body: submission.body.toString()
      }, 15000);
    } catch (error) {
      throw new Error(`JDownloader nu a confirmat primirea linkurilor: ${error?.name === 'AbortError' ? 'timeout' : (error?.message || error)}`);
    }

    const reply = (await response.text()).trim();
    if (!response.ok || /(^|\s)failed(\s|$)/i.test(reply)) {
      throw new Error(`JDownloader a refuzat cererea${reply ? `: ${reply}` : ''}`);
    }
    return { ...submission, reply };
  }

  async function preflightAllowed(ids, destination, mode) {
    const guard = await api('/api/download/preflight', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids, destination, mode })
    });
    const allowed = (guard.decisions || []).filter(x => x.verdict === 'DOWNLOAD').map(x => Number(x.resultId));
    return { guard, allowed };
  }

  async function sendSelectedJDownloaderV8546(options = {}) {
    const ids = Array.isArray(options.ids) && options.ids.length
      ? options.ids.map(Number)
      : (typeof idsForAction === 'function' ? idsForAction() : []);
    if (!ids.length) return toast('Selectează fișiere');

    const button = document.getElementById('jdSelectedBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Conectez JD…';
    }

    try {
      const jd = await checkJDownloader();
      if (!jd.running) {
        throw new Error('JDownloader nu este conectat pe 127.0.0.1:9666. Pornește JDownloader 2 și încearcă din nou. DDG nu mai folosește automat FolderWatch ca substitut pentru conexiunea directă.');
      }

      const destination = String(document.getElementById('downloadDir')?.value || cfg?.downloadDir || '').trim();
      if (!destination) throw new Error('Alege folderul de download înainte de trimiterea în JDownloader.');
      const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';
      const { guard, allowed } = await preflightAllowed(ids, destination, mode);
      if (!allowed.length) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, 0);
        return toast('Nimic de trimis: selecția este duplicat sau necesită verificare');
      }

      const rows = await rowsForIDs(allowed);
      const result = await submitJDownloaderDirect(rows, destination, options.autoStart !== false);
      const c = counts(guard);
      if (c.duplicate > 0 || c.review > 0) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, result.count);
      }
      const jdLabel = jd.version ? `${jd.name} ${jd.version}` : jd.name;
      toast(`JDownloader confirmat • ${result.count} fișier(e) • ${result.destination} • ${jdLabel}`);
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '↗ JDownloader';
      }
    }
  }

  async function downloadSelectedV8546() {
    const ids = typeof idsForAction === 'function' ? idsForAction() : [];
    if (!ids.length) return toast('Selectează fișiere');
    const destination = String(document.getElementById('downloadDir')?.value || cfg?.downloadDir || '').trim();
    if (!destination) return toast('Setează folderul de download');
    const engine = document.getElementById('downloadMethod')?.value || cfg?.downloadMethod || 'auto';
    const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';

    // JDownloader is now a real engine choice. In this mode the normal DDG
    // download queue is not touched at all: ExactGuard filters the selection,
    // then JDownloader receives the links, exact destination and autostart=1.
    if (String(engine).toLowerCase() === 'jdownloader') {
      return sendSelectedJDownloaderV8546({ ids, autoStart: true });
    }

    const request = { ids, engine, destination, guardMode: mode };
    const button = document.getElementById('downloadGuardBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Pregătesc…';
    }
    try {
      const data = await api('/api/queue/add', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)
      });
      await loadResults();
      if (queueNeedsAttention(data)) {
        window.ddgShowGuardReportV8545?.(data.guard, request, data.added);
      } else {
        const modal = document.getElementById('guardModal');
        if (modal && !modal.classList.contains('hidden')) window.closeGuardReport?.();
      }
      await loadQueue();
      if (data.added > 0) {
        const suffix = data.guard?.reusedFreshIndex ? ' • fără a rescana HDD-ul' : '';
        toast(`${data.added} fișier(e) în coadă${suffix}`);
        if (!queueNeedsAttention(data)) goTab('downloads');
      } else if (!queueNeedsAttention(data)) {
        toast('Niciun fișier nou nu a fost adăugat în coadă');
      }
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        updateDownloadButtonLabel();
      }
    }
  }

  function ensureJDownloaderEngineOption() {
    const select = document.getElementById('downloadMethod');
    if (!select) return;
    if (!select.querySelector('option[value="jdownloader"]')) {
      const option = document.createElement('option');
      option.value = 'jdownloader';
      option.textContent = 'JDownloader 2 — direct';
      select.appendChild(option);
    }
    if (String(cfg?.downloadMethod || '').toLowerCase() === 'jdownloader') {
      select.value = 'jdownloader';
    }
    if (!select.dataset.ddgJdBound) {
      select.dataset.ddgJdBound = '1';
      select.addEventListener('change', updateDownloadButtonLabel);
    }
  }

  function updateDownloadButtonLabel() {
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (!button) return;
    const engine = document.getElementById('downloadMethod')?.value || cfg?.downloadMethod || 'auto';
    if (String(engine).toLowerCase() === 'jdownloader') {
      button.textContent = '⬇ Descarcă prin JDownloader';
      button.title = 'ExactGuard verifică selecția, apoi JDownloader primește direct linkurile, folderul ales în DDG și comanda de pornire.';
    } else {
      button.textContent = '⬇ Descarcă';
      button.title = 'Un click: DDG verifică în fundal și pornește coada. Indexul tocmai actualizat este reutilizat, nu scanat încă o dată.';
    }
  }

  function installButtons() {
    const download = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (!download) return false;

    // Preserve the existing id expected by guard/report code even when the
    // original button only had inline onclick.
    if (!download.id) download.id = 'downloadGuardBtn';
    ensureJDownloaderEngineOption();
    updateDownloadButtonLabel();

    if (!document.getElementById('jdSelectedBtn')) {
      const jd = document.createElement('button');
      jd.className = 'btn';
      jd.id = 'jdSelectedBtn';
      jd.type = 'button';
      jd.textContent = '↗ JDownloader';
      jd.title = 'Conexiune directă cu JDownloader 2: verifică selecția, trimite folderul exact și pornește downloadul în JD.';
      jd.addEventListener('click', () => sendSelectedJDownloaderV8546({ autoStart: true }));
      download.insertAdjacentElement('afterend', jd);
    }

    window.downloadSelected = downloadSelectedV8546;
    window.sendSelectedJD2 = () => sendSelectedJDownloaderV8546({ autoStart: true });
    return true;
  }

  function install() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      if (installButtons() || tries >= 80) clearInterval(timer);
    }, 100);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setTimeout(install, 0), { once: true });
  } else {
    install();
  }

  window.ddgDownloadActionsV8545 = {
    install,
    sendSelectedJDownloaderV8545: sendSelectedJDownloaderV8546,
    sendSelectedJDownloaderV8546,
    checkJDownloader
  };
})();
