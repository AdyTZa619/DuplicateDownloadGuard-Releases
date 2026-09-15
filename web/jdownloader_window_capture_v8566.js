// TEST v9.0.1 — canonical, fail-closed JDownloader boundary.
// This script is loaded before every legacy JD module. It owns the earliest
// window-capture handler and is the only frontend entry allowed to hand IDs to
// JDownloader. The backend reruns Download Guard and exports only final LIPSĂ.
(() => {
  'use strict';

  let busy = false;

  const liveEngine = () => String(document.getElementById('downloadMethod')?.value || window.cfg?.downloadMethod || '').trim().toLowerCase();
  const normalizeIDs = ids => [...new Set((ids || []).map(Number).filter(Number.isFinite))];

  function selectedIDs() {
    try {
      return typeof window.idsForAction === 'function' ? normalizeIDs(window.idsForAction()) : [];
    } catch (_) {
      return [];
    }
  }

  function destination() {
    return String(document.getElementById('downloadDir')?.value || window.cfg?.downloadDir || '').trim();
  }

  function guardMode() {
    return document.getElementById('downloadGuardMode')?.value || window.cfg?.downloadGuardMode || 'smart';
  }

  async function sendIDs(rawIDs) {
    const ids = normalizeIDs(rawIDs);
    if (!ids.length) throw new Error('Selectează fișiere');
    if (busy) throw new Error('Verificarea JDownloader este deja în curs.');
    if (typeof window.api !== 'function') throw new Error('Filtrul JDownloader nu este disponibil. Nu s-a trimis nimic.');

    busy = true;
    const request = {ids, destination: destination(), guardMode: guardMode()};
    try {
      const result = await window.api('/api/download/jdownloader-direct', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(request)
      });
      await window.loadResults?.();
      if (result?.guard && window.ddgShowGuardReportV8545) {
        window.ddgShowGuardReportV8545(result.guard, request, result.externalAdded || 0);
      }
      window.toast?.(result?.message || `JDownloader: ${result?.externalAdded || 0} fișier(e) confirmate LIPSĂ`);
      return result;
    } finally {
      busy = false;
    }
  }

  const sendSelected = () => sendIDs(selectedIDs());
  const guardedAPI = Object.freeze({sendIDs, sendSelected, selectedIDs});
  Object.defineProperty(window, 'ddgJDownloaderGuardV901', {
    value: guardedAPI,
    configurable: false,
    enumerable: true,
    writable: false
  });

  window.addEventListener('click', event => {
    const target = event.target?.closest?.('#jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;
    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;

    event.preventDefault();
    event.stopImmediatePropagation();
    sendSelected().catch(error => window.toast?.(error?.message || String(error)));
  }, true);
})();
