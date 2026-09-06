// Audible notification when a new Stable/TEST update appears in the DDG top bar.
// Uses a native Windows chime through the DDG backend, so it does not depend on
// browser/WebView autoplay permission or on the DDG window having focus.
(() => {
  'use strict';

  const SEEN_COOKIE = 'ddgUpdateSoundSeenV2';
  const MAX_AGE = 365 * 24 * 60 * 60;
  let observer = null;
  let inFlightKey = '';

  function updateEntries() {
    const corner = document.getElementById('ddgUpdateCorner');
    if (!corner) return [];
    return [...corner.querySelectorAll('.ddgUpdateChip[data-channel]')]
      .map(btn => ({
        channel: String(btn.dataset.channel || '').trim().toLowerCase(),
        label: String(btn.textContent || '').trim()
      }))
      .filter(x => x.channel && x.label);
  }

  function makeKey(entries) {
    return entries
      .map(x => `${x.channel}:${x.label}`)
      .sort()
      .join('|');
  }

  function lastSeen() {
    try {
      const prefix = SEEN_COOKIE + '=';
      for (const part of String(document.cookie || '').split(';')) {
        const item = part.trim();
        if (item.startsWith(prefix)) return decodeURIComponent(item.slice(prefix.length));
      }
    } catch (_) {}
    return '';
  }

  function remember(key) {
    try {
      document.cookie = `${SEEN_COOKIE}=${encodeURIComponent(key)}; Max-Age=${MAX_AGE}; Path=/; SameSite=Lax`;
    } catch (_) {}
  }

  async function notifyNative(key, label) {
    if (!key || key === lastSeen() || inFlightKey === key) return;
    inFlightKey = key;
    try {
      const response = await fetch('/api/update/native-notify', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({key, label}),
        cache: 'no-store'
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      remember(key);
      if (typeof window.toast === 'function') {
        window.toast(`Update nou disponibil: ${label}`);
      }
    } catch (_) {
      // Do not loop a failed notification every 1.5 seconds. A later update gets
      // its own key and will be attempted again.
      remember(key);
    } finally {
      inFlightKey = '';
    }
  }

  function scan() {
    const entries = updateEntries();
    if (!entries.length) return;
    const key = makeKey(entries);
    if (!key || key === lastSeen() || inFlightKey === key) return;
    notifyNative(key, entries.map(x => x.label).join(' + '));
  }

  function boot() {
    if (!observer && document.body) {
      observer = new MutationObserver(scan);
      observer.observe(document.body, {childList: true, subtree: true, characterData: true});
    }
    setInterval(scan, 1500);
    setTimeout(scan, 1800);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, {once: true});
  } else {
    boot();
  }

  window.ddgUpdateSoundV8552 = {scan};
})();
