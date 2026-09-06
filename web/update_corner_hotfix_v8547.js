// TEST hotfix: keep Stable/TEST update actions as a real header sibling.
// The operation HUD rebuilds the former topStatus parent with innerHTML, so an
// updater control nested there can be deleted or become an invalid nested button.
(() => {
  'use strict';

  const CORNER_ID = 'ddgUpdateCorner';
  let observer = null;

  function placeCornerBesideHud() {
    const top = document.querySelector('.top');
    const corner = document.getElementById(CORNER_ID);
    if (!top || !corner) return false;

    const hud = document.getElementById('operationHud');
    if (corner.parentElement !== top) {
      if (hud && hud.parentElement === top) top.insertBefore(corner, hud);
      else top.appendChild(corner);
    } else if (hud && hud.parentElement === top && corner.nextElementSibling !== hud) {
      top.insertBefore(corner, hud);
    }

    corner.style.marginLeft = 'auto';
    corner.style.marginRight = '8px';
    corner.style.flexShrink = '0';
    return true;
  }

  function boot() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      placeCornerBesideHud();
      if (tries >= 80) clearInterval(timer);
    }, 100);

    if (!observer && document.body) {
      observer = new MutationObserver(() => placeCornerBesideHud());
      observer.observe(document.body, { childList: true, subtree: true });
    }

    // Re-check after the channel module performs its delayed manifest check.
    setTimeout(placeCornerBesideHud, 1800);
    setTimeout(placeCornerBesideHud, 3500);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, { once: true });
  } else {
    boot();
  }

  window.ddgPlaceUpdateCornerV8547 = placeCornerBesideHud;
})();
