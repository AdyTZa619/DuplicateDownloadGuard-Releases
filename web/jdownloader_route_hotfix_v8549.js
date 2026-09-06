// TEST hotfix: ExactGuard installs its own downloadSelected() after the base UI.
// When JDownloader is selected, intercept the primary Download button before
// ExactGuard can enqueue the file in DDG's internal queue.
(() => {
  'use strict';

  function selectedEngineIsJD() {
    const select = document.getElementById('downloadMethod');
    const value = String(select?.value || (typeof cfg !== 'undefined' ? cfg?.downloadMethod : '') || '').toLowerCase();
    return value === 'jdownloader';
  }

  function sendThroughJD() {
    const bridge = window.ddgDownloadActionsV8545;
    const send = bridge?.sendSelectedJDownloaderV8546 || bridge?.sendSelectedJDownloaderV8545;
    if (typeof send !== 'function') {
      if (typeof window.toast === 'function') window.toast('Integrarea JDownloader nu este încă pregătită. Reîncearcă într-o secundă.');
      return;
    }
    return send({ autoStart: true });
  }

  // Capture-phase interception is intentional: the legacy primary button has
  // inline onclick="downloadSelected()" and ExactGuard can replace that global
  // function later. In JD mode we stop that path before it can call /api/queue/add.
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
})();
