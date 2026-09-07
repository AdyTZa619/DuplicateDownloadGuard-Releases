// TEST v8.5.78 — MEGA scan result visibility + duplicate-click guard.
// UI-only wrapper. It does not modify the MEGA backend, preview listeners, indexer or JD.
(() => {
  'use strict';

  let installed = false;
  let inFlight = false;

  async function resultRevision() {
    try {
      const r = await fetch('/api/results/summary', {cache: 'no-store'});
      if (!r.ok) return null;
      const data = await r.json();
      return Number(data?.revision ?? 0);
    } catch (_) {
      return null;
    }
  }

  function megaButtons() {
    return [...document.querySelectorAll('button[onclick="scanMega()"]')];
  }

  function setButtonsBusy(busy) {
    for (const button of megaButtons()) {
      if (!button.dataset.ddgMegaOriginalText) button.dataset.ddgMegaOriginalText = button.textContent || 'Scanează MEGA';
      button.disabled = !!busy;
      button.textContent = busy ? 'Scanez MEGA…' : button.dataset.ddgMegaOriginalText;
    }
  }

  async function showFreshResults() {
    try {
      if (typeof window.goTab === 'function') window.goTab('results');
      if (typeof window.loadResults === 'function') await window.loadResults();
      if (typeof window.refreshStats === 'function') await window.refreshStats();
      const top = document.getElementById('topStatus');
      if (top) top.textContent = 'MEGA: comparație terminată • rezultate afișate';
    } catch (_) {}
  }

  function install() {
    if (installed) return;
    const original = window.scanMega;
    if (typeof original !== 'function') {
      setTimeout(install, 50);
      return;
    }
    if (original.__ddgMegaResultAwareV8578) {
      installed = true;
      return;
    }

    async function wrappedScanMega(...args) {
      if (inFlight) {
        if (typeof window.toast === 'function') window.toast('Scanarea MEGA este deja în curs.');
        return;
      }
      inFlight = true;
      setButtonsBusy(true);
      const before = await resultRevision();
      try {
        await original.apply(this, args);
        const after = await resultRevision();
        // The legacy scan catches its own HTTP errors. Only switch to Results
        // when DDG actually published a newer result revision.
        if (after !== null && before !== null && after > before) {
          await showFreshResults();
        } else if (after !== null && before === null) {
          await showFreshResults();
        }
      } finally {
        inFlight = false;
        setButtonsBusy(false);
      }
    }

    wrappedScanMega.__ddgMegaResultAwareV8578 = true;
    wrappedScanMega.__ddgMegaOriginal = original;
    window.scanMega = wrappedScanMega;
    installed = true;
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once: true});
  else install();

  window.ddgMegaScanResultUIV8578 = {install, showFreshResults};
})();
