(() => {
  'use strict';

  const TEST_MANIFEST = 'https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/testing/update-test.json';
  const POLL_MS = 20 * 1000;
  let installedVersion = '';
  let lastTriggeredVersion = '';
  let running = false;

  function parseVersion(value) {
    const m = String(value || '').match(/(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?/i);
    if (!m) return null;
    return {core: [+m[1], +m[2], +m[3]], pre: m[4] ? m[4].toLowerCase().split('.') : []};
  }

  function comparePre(a, b) {
    if (!a.length && !b.length) return 0;
    if (!a.length) return 1;
    if (!b.length) return -1;
    for (let i = 0; i < Math.min(a.length, b.length); i++) {
      if (a[i] === b[i]) continue;
      const an = /^\d+$/.test(a[i]);
      const bn = /^\d+$/.test(b[i]);
      if (an && bn) return Number(a[i]) > Number(b[i]) ? 1 : -1;
      if (an !== bn) return an ? -1 : 1;
      return a[i] > b[i] ? 1 : -1;
    }
    return a.length === b.length ? 0 : (a.length > b.length ? 1 : -1);
  }

  function isNewer(remote, local) {
    const a = parseVersion(remote);
    const b = parseVersion(local);
    if (!a || !b) return false;
    for (let i = 0; i < 3; i++) {
      if (a.core[i] !== b.core[i]) return a.core[i] > b.core[i];
    }
    return comparePre(a.pre, b.pre) > 0;
  }

  async function localVersion() {
    if (installedVersion) return installedVersion;
    if (typeof window.api !== 'function') return '';
    try {
      const info = await window.api('/api/about');
      installedVersion = String(info?.version || '').trim();
    } catch (_) {}
    return installedVersion;
  }

  async function checkOnce() {
    if (running) return;
    running = true;
    try {
      const local = await localVersion();
      if (!local) return;
      const response = await fetch(`${TEST_MANIFEST}?ddg=${Date.now()}`, {
        cache: 'no-store',
        headers: {'Accept':'application/json'}
      });
      if (!response.ok) return;
      const manifest = await response.json();
      const remote = String(manifest?.version || '').trim();
      if (!remote || !isNewer(remote, local)) return;
      if (remote === lastTriggeredVersion) return;
      lastTriggeredVersion = remote;

      // Lightweight raw polling only detects the change. The existing channel
      // updater then performs its pinned-commit GitHub API check before showing
      // and installing the update, preserving the SHA-safe transport.
      if (typeof window.ddgCheckAllUpdateChannels === 'function') {
        await window.ddgCheckAllUpdateChannels();
      }
      setTimeout(() => window.ddgUpdateSoundV8552?.scan?.(), 250);
    } catch (_) {
      // Network hiccups are silent; next 20-second poll retries automatically.
    } finally {
      running = false;
    }
  }

  function boot() {
    setTimeout(checkOnce, 2500);
    setInterval(checkOnce, POLL_MS);
    window.addEventListener('online', checkOnce);
    window.addEventListener('focus', checkOnce);
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) checkOnce();
    });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();

  window.ddgFastUpdateWatchV8562 = {check: checkOnce};
})();
