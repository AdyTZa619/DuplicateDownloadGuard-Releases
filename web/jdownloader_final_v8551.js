// Final JDownloader router for TEST builds.
// IMPORTANT: this file is injected directly into the compiled UI by build-test.yml,
// so it must not contain a second/legacy preflight implementation. The actual JD
// flow lives in jdownloader_batch_confirm_v8564.js: current results first, explicit
// confirmation, optional full HDD recheck, one FlashGot request / one package.
(() => {
  'use strict';

  let busy = false;

  function liveEngine() {
    const select = document.getElementById('downloadMethod');
    return String(select?.value || window.cfg?.downloadMethod || '').trim().toLowerCase();
  }

  function primaryButton() {
    return document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
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
    const button = primaryButton();
    if (!button || busy) return;
    if (liveEngine() === 'jdownloader') {
      button.textContent = '⬇ Verifică și trimite în JDownloader';
      button.title = 'Folosește rezultatele curente fără rescanarea HDD-urilor. Alegi recomandate, TOATE sau reverificare completă.';
    } else if (button.textContent.includes('JDownloader')) {
      button.textContent = '⬇ Descarcă';
    }
  }

  async function ensureBatchModule() {
    if (window.ddgJDownloaderBatchConfirmV8564?.sendBatchAware) {
      return window.ddgJDownloaderBatchConfirmV8564;
    }

    let script = document.getElementById('ddgJDownloaderBatchConfirmV8564Script');
    if (!script) {
      script = document.createElement('script');
      script.id = 'ddgJDownloaderBatchConfirmV8564Script';
      script.src = '/jdownloader_batch_confirm_v8564.js';
      script.async = false;
      document.head.appendChild(script);
    }

    const started = Date.now();
    while (Date.now() - started < 4000) {
      if (window.ddgJDownloaderBatchConfirmV8564?.sendBatchAware) {
        return window.ddgJDownloaderBatchConfirmV8564;
      }
      await new Promise(resolve => setTimeout(resolve, 50));
    }
    throw new Error('Modulul de confirmare JDownloader nu s-a încărcat. Repornește DDG și încearcă din nou.');
  }

  async function sendExclusiveToJDownloader() {
    if (busy) return;
    busy = true;
    const button = primaryButton();
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Pregătesc confirmarea JDownloader…';
    }
    try {
      const batch = await ensureBatchModule();
      // No /api/download/preflight here. The batch module uses current DDG
      // results and offers an explicit full HDD recheck only if the user asks.
      await batch.sendBatchAware();
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
    window.sendSelectedJD2 = sendExclusiveToJDownloader;
    updateLabel();
  }

  // This file is the last JD router injected into the compiled EXE. Intercept
  // JD actions in capture phase and route every one through the single batch
  // confirmation implementation. This prevents legacy handlers from starting
  // a 300k+ file preflight or sending only REVIEW items.
  document.addEventListener('click', event => {
    const target = event.target?.closest?.('#jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;

    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;

    event.preventDefault();
    event.stopImmediatePropagation();
    sendExclusiveToJDownloader();
  }, true);

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setTimeout(install, 0), {once:true});
  } else {
    setTimeout(install, 0);
  }
  setTimeout(install, 600);

  window.ddgJDownloaderFinalV8551 = {
    sendExclusiveToJDownloader,
    ensureBatchModule,
    install
  };
})();
