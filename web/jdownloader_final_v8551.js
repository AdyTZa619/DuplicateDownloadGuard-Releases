// Final JDownloader router for TEST builds.
// IMPORTANT: this file is injected directly into the compiled UI by build-test.yml,
// so it must not contain a second/legacy preflight implementation. The actual JD
// flow is completed by the DDG backend. The browser never posts unchecked
// current rows directly to JDownloader.
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
      button.title = 'Folosește rezultatele curente fără rescanarea HDD-urilor și trimite numai lipsurile, într-un singur pachet.';
    } else if (button.textContent.includes('JDownloader')) {
      button.textContent = '⬇ Descarcă';
    }
  }

  function selectedIDs() {
    return typeof window.idsForAction === 'function'
      ? window.idsForAction().map(Number).filter(Number.isFinite)
      : [];
  }

  async function guardedBackendHandoff() {
    const guard = window.ddgJDownloaderGuardV901;
    if (!guard?.sendSelected) throw new Error('Filtrul JDownloader nu este disponibil. Nu s-a trimis nimic.');
    return guard.sendSelected();
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
      await guardedBackendHandoff();
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
    guardedBackendHandoff,
    install
  };
})();
