// Final JDownloader routing guard for TEST builds.
// It runs in capture phase and calls a dedicated backend endpoint, so no
// legacy onclick/downloadSelected override can ever enqueue the same click in DDG.
(() => {
  'use strict';

  let busy = false;

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

  function updateLabel() {
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (!button || busy) return;
    if (liveEngine() === 'jdownloader') {
      button.textContent = '⬇ Descarcă prin JDownloader';
      button.title = 'Trimite selecția exclusiv către JDownloader. Coada internă DDG nu este folosită.';
    }
  }

  function selectedIDs() {
    try {
      return typeof window.idsForAction === 'function' ? window.idsForAction().map(Number) : [];
    } catch (_) {
      return [];
    }
  }

  async function sendExclusiveToJDownloader() {
    if (busy) return;
    const ids = selectedIDs();
    if (!ids.length) {
      window.toast?.('Selectează fișiere');
      return;
    }

    const destination = String(document.getElementById('downloadDir')?.value || window.cfg?.downloadDir || '').trim();
    const guardMode = document.getElementById('downloadGuardMode')?.value || window.cfg?.downloadGuardMode || 'smart';
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');

    busy = true;
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Trimit exclusiv în JDownloader…';
    }

    try {
      const data = await window.api('/api/download/jdownloader-direct', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, destination, guardMode })
      });

      if (data?.jdownloader !== true) {
        throw new Error('Protecție DDG: backendul nu a confirmat ruta JDownloader. Nu s-a pornit download intern.');
      }
      if (Number(data?.added || 0) !== 0) {
        throw new Error('Protecție DDG: ruta JDownloader a raportat joburi interne. Operația a fost oprită.');
      }

      const counts = data?.guard?.counts || {};
      if ((Number(counts.DUPLICATE || 0) > 0 || Number(counts.REVIEW || 0) > 0) && window.ddgShowGuardReportV8545) {
        window.ddgShowGuardReportV8545(data.guard, { ids, destination, guardMode }, Number(data.externalAdded || 0));
      }
      window.toast?.(data?.message || `Trimis exclusiv în JDownloader: ${Number(data?.externalAdded || 0)} fișier(e)`);
    } catch (error) {
      // Dedicated endpoint is fail-closed: on any error there is deliberately
      // no fallback to /api/queue/add and therefore no integrated DDG download.
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
      select.addEventListener('change', () => setTimeout(updateLabel, 0));
    }
    updateLabel();
  }

  // Capture phase is intentional. This executes before inline onclick and
  // before ExactGuard/legacy handlers. In JDownloader mode the click can only
  // reach the dedicated backend endpoint below.
  document.addEventListener('click', event => {
    if (liveEngine() !== 'jdownloader') return;
    const button = event.target?.closest?.('#downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!button) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendExclusiveToJDownloader();
  }, true);

  document.addEventListener('DOMContentLoaded', () => setTimeout(install, 0), { once: true });
  setTimeout(install, 600);

  window.ddgJDownloaderFinalV8551 = { sendExclusiveToJDownloader, install };
})();
