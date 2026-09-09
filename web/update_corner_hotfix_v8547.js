// Persistent Stable/TEST update alerts in the DDG top bar.
// The operation HUD can rebuild the header and delete sibling controls; this
// module recreates the updater slot, re-runs the manifest checks automatically,
// and keeps the user from having to open the updater page manually.
(() => {
  'use strict';

  const CORNER_ID = 'ddgUpdateCorner';
  const AUTO_CHECK_MS = 5 * 60 * 1000;
  let observer = null;
  let checkTimer = null;
  let lastCheckRequestedAt = 0;
  let repairQueued = false;

  function ensureCornerNode() {
    let corner = document.getElementById(CORNER_ID);
    if (!corner) {
      corner = document.createElement('span');
      corner.id = CORNER_ID;
      corner.setAttribute('aria-live', 'polite');
    }
    return corner;
  }

  function placeCornerBesideHud() {
    const top = document.querySelector('.top');
    if (!top) return false;
    const corner = ensureCornerNode();
    const hud = document.getElementById('operationHud');
    const status = document.getElementById('topStatus');
    const statusHost = status?.parentElement;

    if (hud && hud.parentElement === top) {
      if (corner.parentElement !== top || corner.nextElementSibling !== hud) {
        top.insertBefore(corner, hud);
      }
    } else if (statusHost && statusHost.parentElement === top) {
      if (corner.parentElement !== top || corner.nextElementSibling !== statusHost) {
        top.insertBefore(corner, statusHost);
      }
    } else if (corner.parentElement !== top) {
      top.appendChild(corner);
    }

    corner.style.display = 'inline-flex';
    corner.style.alignItems = 'center';
    corner.style.gap = '7px';
    corner.style.marginLeft = 'auto';
    corner.style.marginRight = '8px';
    corner.style.flexShrink = '0';
    return true;
  }

  function requestChannelCheck(delay = 0, force = false) {
    const now = Date.now();
    if (!force && now - lastCheckRequestedAt < 15000) return;
    lastCheckRequestedAt = now;
    clearTimeout(checkTimer);
    checkTimer = setTimeout(() => {
      if (typeof window.ddgCheckAllUpdateChannels === 'function') {
        window.ddgCheckAllUpdateChannels();
      } else {
        // The channel module is loaded just before this one. If WebView delayed
        // execution, retry until it becomes available.
        lastCheckRequestedAt = 0;
        requestChannelCheck(400, true);
      }
    }, delay);
  }

  function repairAfterHeaderMutation() {
    if (repairQueued) return;
    repairQueued = true;
    queueMicrotask(() => {
      repairQueued = false;
      const before = document.getElementById(CORNER_ID);
      const wasMissing = !before || before.parentElement !== document.querySelector('.top');
      if (!placeCornerBesideHud()) return;
      // If the HUD deleted/replaced the update slot, current button DOM was lost.
      // Re-checking restores the Stable/TEST button immediately from fresh state.
      if (wasMissing) requestChannelCheck(120, true);
    });
  }

  function boot() {
    placeCornerBesideHud();

    if (!observer && document.body) {
      observer = new MutationObserver(repairAfterHeaderMutation);
      observer.observe(document.body, { childList: true, subtree: true });
    }

    // Automatic discovery: startup, periodic checks, returning to the app and
    // network reconnection all refresh the top-bar notification.
    requestChannelCheck(900, true);
    setInterval(() => requestChannelCheck(0, true), AUTO_CHECK_MS);
    window.addEventListener('focus', () => requestChannelCheck(150, false));
    window.addEventListener('online', () => requestChannelCheck(300, true));
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) requestChannelCheck(150, false);
    });

    // The operation HUD initializes shortly after DOMContentLoaded.
    setTimeout(repairAfterHeaderMutation, 1600);
    setTimeout(repairAfterHeaderMutation, 3500);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, { once: true });
  } else {
    boot();
  }

  window.ddgPlaceUpdateCornerV8547 = placeCornerBesideHud;
  window.ddgRefreshUpdateAlerts = () => requestChannelCheck(0, true);
})();
