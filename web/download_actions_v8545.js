(() => {
  'use strict';

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

  function checkJDownloader() {
    return new Promise(resolve => {
      document.getElementById('ddgJDCheckScript')?.remove();
      window.jdownloader = false;
      const script = document.createElement('script');
      script.id = 'ddgJDCheckScript';
      script.src = `http://127.0.0.1:9666/jdcheck.js?_ddg=${Date.now()}`;
      let finished = false;
      const finish = value => {
        if (finished) return;
        finished = true;
        script.remove();
        resolve(Boolean(value));
      };
      script.onload = () => finish(window.jdownloader === true);
      script.onerror = () => finish(false);
      document.head.appendChild(script);
      setTimeout(() => finish(window.jdownloader === true), 1200);
    });
  }

  function submitFlashGot(rows, destination) {
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

    let iframe = document.getElementById('ddgJDTarget');
    if (!iframe) {
      iframe = document.createElement('iframe');
      iframe.id = 'ddgJDTarget';
      iframe.name = 'ddgJDTarget';
      iframe.style.display = 'none';
      document.body.appendChild(iframe);
    }
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = 'http://127.0.0.1:9666/flashgot';
    form.target = 'ddgJDTarget';
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
    if (destination) add('dir', destination);
    document.body.appendChild(form);
    form.submit();
    setTimeout(() => form.remove(), 1500);
    return urls.length;
  }

  async function folderWatchFallback(ids) {
    const folder = String(cfg?.jdFolder || '').trim();
    if (!folder) return false;
    const data = await api('/api/download/jd2', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids, folder })
    });
    if (window.ddgShowGuardReportV8545 && counts(data.guard).duplicate + counts(data.guard).review > 0) {
      window.ddgShowGuardReportV8545(data.guard, { ids, destination: cfg?.downloadDir || '', guardMode: cfg?.downloadGuardMode || 'smart' }, data.count || 0);
    }
    toast(`JDownloader FolderWatch: ${data.count || 0} fișier(e) selectate pregătite`);
    return true;
  }

  async function sendSelectedJDownloaderV8545() {
    const ids = typeof idsForAction === 'function' ? idsForAction() : [];
    if (!ids.length) return toast('Selectează fișiere');
    const button = document.getElementById('jdSelectedBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ JDownloader…';
    }
    try {
      const running = await checkJDownloader();
      if (!running) {
        if (await folderWatchFallback(ids)) return;
        throw new Error('JDownloader nu răspunde pe 127.0.0.1:9666. Pornește JDownloader sau setează FolderWatch în Reguli.');
      }

      const destination = document.getElementById('downloadDir')?.value?.trim() || cfg?.downloadDir || '';
      const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';
      const guard = await api('/api/download/preflight', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, destination, mode })
      });
      const allowed = (guard.decisions || []).filter(x => x.verdict === 'DOWNLOAD').map(x => Number(x.resultId));
      if (!allowed.length) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, 0);
        return toast('Nimic de trimis: selecția este duplicat sau necesită verificare');
      }
      const rows = await rowsForIDs(allowed);
      const count = submitFlashGot(rows, destination);
      const c = counts(guard);
      if (c.duplicate > 0 || c.review > 0) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, count);
      }
      toast(`Trimis în JDownloader LinkGrabber: ${count} fișier(e)`);
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '↗ JDownloader';
      }
    }
  }

  async function downloadSelectedV8545() {
    const ids = typeof idsForAction === 'function' ? idsForAction() : [];
    if (!ids.length) return toast('Selectează fișiere');
    const destination = document.getElementById('downloadDir')?.value?.trim() || cfg?.downloadDir || '';
    const engine = document.getElementById('downloadMethod')?.value || cfg?.downloadMethod || 'auto';
    const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';
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
        button.textContent = '⬇ Descarcă';
      }
    }
  }

  function installButtons() {
    const download = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (!download) return false;
    download.textContent = '⬇ Descarcă';
    download.title = 'Un click: DDG verifică în fundal și pornește coada. Indexul tocmai actualizat este reutilizat, nu scanat încă o dată.';

    if (!document.getElementById('jdSelectedBtn')) {
      const jd = document.createElement('button');
      jd.className = 'btn';
      jd.id = 'jdSelectedBtn';
      jd.type = 'button';
      jd.textContent = '↗ JDownloader';
      jd.title = 'Trimite numai selecția confirmată ca lipsă în JDownloader LinkGrabber.';
      jd.addEventListener('click', sendSelectedJDownloaderV8545);
      download.insertAdjacentElement('afterend', jd);
    }
    window.downloadSelected = downloadSelectedV8545;
    window.sendSelectedJD2 = sendSelectedJDownloaderV8545;
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
  window.ddgDownloadActionsV8545 = { install, sendSelectedJDownloaderV8545 };
})();
