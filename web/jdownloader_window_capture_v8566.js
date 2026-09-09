// TEST v8.5.66 — top-level JD click guard.
// Window capture runs before every document-level legacy capture handler.
(() => {
  'use strict';

  const liveEngine = () => String(document.getElementById('downloadMethod')?.value || window.cfg?.downloadMethod || '').trim().toLowerCase();

  window.addEventListener('click', event => {
    const target = event.target?.closest?.('#jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;
    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;

    event.preventDefault();
    event.stopImmediatePropagation();

    const send = () => {
      const fast = window.ddgJDownloaderFastV8566;
      if (fast?.sendBatchAware) return fast.sendBatchAware();
      window.toast?.('Modulul JDownloader se inițializează. Apasă din nou peste o secundă.');
    };
    send();
  }, true);
})();
