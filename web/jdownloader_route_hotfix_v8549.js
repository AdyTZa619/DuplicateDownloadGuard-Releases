// TEST hard route: JDownloader is an exclusive external engine.
// When selected, no frontend path is allowed to enqueue the same files in DDG.
(() => {
  'use strict';

  function selectedEngineIsJD() {
    const select = document.getElementById('downloadMethod');
    const value = String(select?.value || (typeof cfg !== 'undefined' ? cfg?.downloadMethod : '') || '').trim().toLowerCase();
    return value === 'jdownloader';
  }

  function idsSelected() {
    try {
      return typeof idsForAction === 'function' ? idsForAction().map(Number).filter(Number.isFinite) : [];
    } catch (_) {
      return [];
    }
  }

  function destinationSelected() {
    return String(document.getElementById('downloadDir')?.value || (typeof cfg !== 'undefined' ? cfg?.downloadDir : '') || '').trim();
  }

  function guardModeSelected() {
    return String(document.getElementById('downloadGuardMode')?.value || (typeof cfg !== 'undefined' ? cfg?.downloadGuardMode : '') || 'smart').trim() || 'smart';
  }

  // Final safety net. ExactGuard and legacy inline handlers can both call
  // /api/queue/add. If the visible engine is JDownloader, force the request to
  // carry engine=jdownloader so the backend router handles it externally.
  // This prevents stale cfg.downloadMethod='auto' from silently starting the
  // DDG downloader even if another JS module wins a function-override race.
  const nativeFetch = window.fetch.bind(window);
  window.fetch = function ddgJDFailClosedFetch(input, init) {
    try {
      const requestURL = typeof input === 'string' ? input : String(input?.url || '');
      if (selectedEngineIsJD() && /(^|\/)api\/queue\/add(?:\?|$)/i.test(requestURL)) {
        const next = {...(init || {})};
        if (typeof next.body === 'string') {
          const payload = JSON.parse(next.body);
          payload.engine = 'jdownloader';
          payload.destination = String(payload.destination || destinationSelected()).trim();
          payload.guardMode = String(payload.guardMode || guardModeSelected()).trim();
          next.body = JSON.stringify(payload);
        }
        return nativeFetch(input, next);
      }
    } catch (_) {
      // If a non-JSON request ever reaches this guard, leave it untouched.
      // The primary click route below still remains fail-closed.
    }
    return nativeFetch(input, init);
  };

  async function sendThroughJD() {
    const ids = idsSelected();
    if (!ids.length) return window.toast?.('Selectează fișiere');

    const destination = destinationSelected();
    if (!destination) return window.toast?.('Alege folderul de download în DDG.');

    const button = document.getElementById('downloadGuardBtn');
    const jdButton = document.getElementById('jdSelectedBtn');
    for (const b of [button, jdButton]) {
      if (!b) continue;
      b.disabled = true;
      b.dataset.ddgOldText = b.textContent || '';
      b.textContent = '⏳ Trimit în JDownloader…';
    }

    try {
      const data = await api('/api/queue/add', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          ids,
          engine: 'jdownloader',
          destination,
          guardMode: guardModeSelected()
        })
      });

      const sent = Number(data?.externalAdded || 0);
      const guard = data?.guard;
      const duplicate = Number(guard?.counts?.DUPLICATE || 0);
      const review = Number(guard?.counts?.REVIEW || 0);
      if (sent <= 0 && (duplicate > 0 || review > 0)) {
        window.ddgShowGuardReportV8545?.(guard, {
          ids,
          destination,
          guardMode: guardModeSelected(),
          engine: 'jdownloader'
        }, 0);
      }
      window.toast?.(data?.message || `JDownloader: ${sent} fișier(e) trimise`);
    } catch (error) {
      // Deliberately no fallback. If JDownloader is unavailable, nothing is
      // downloaded by DDG and the user gets the real connection error.
      window.toast?.(error?.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '⬇ Descarcă prin JDownloader';
        button.title = 'JDownloader este motor exclusiv. Dacă nu răspunde, DDG nu descarcă în locul lui.';
      }
      if (jdButton) {
        jdButton.disabled = false;
        jdButton.textContent = '↗ JDownloader';
      }
    }
  }

  // Capture both the primary download action and the explicit JD button before
  // inline/legacy handlers. In JD mode there is exactly one route: backend ->
  // JDownloader. No browser FlashGot form and no DDG queue fallback remain.
  document.addEventListener('click', event => {
    if (!selectedEngineIsJD()) return;
    const target = event.target?.closest?.('#downloadGuardBtn, #jdSelectedBtn, button[onclick="downloadSelected()"], button[onclick="sendSelectedJD2()"]');
    if (!target) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendThroughJD();
  }, true);

  function refreshLabel() {
    const select = document.getElementById('downloadMethod');
    if (!select) return;
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (selectedEngineIsJD()) {
      if (typeof cfg !== 'undefined' && cfg) cfg.downloadMethod = 'jdownloader';
      if (button && !button.disabled) {
        button.textContent = '⬇ Descarcă prin JDownloader';
        button.title = 'JDownloader este motor exclusiv. Coada DDG nu este folosită.';
      }
    }
  }

  function boot() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      const select = document.getElementById('downloadMethod');
      if (!select) {
        if (tries >= 100) clearInterval(timer);
        return;
      }
      if (!select.dataset.ddgJdHardBound) {
        select.dataset.ddgJdHardBound = '1';
        select.addEventListener('change', () => setTimeout(refreshLabel, 0));
      }
      refreshLabel();
      if (tries >= 100) clearInterval(timer);
    }, 100);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once: true});
  else boot();

  window.ddgJDownloaderRouteHotfixV8549 = {sendThroughJD};
})();
