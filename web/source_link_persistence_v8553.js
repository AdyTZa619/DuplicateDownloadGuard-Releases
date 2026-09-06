// Persist the last universal source link across DDG restarts.
// DDG serves its UI from a random 127.0.0.1 port on every launch. localStorage
// is origin+port scoped, so it cannot survive those port changes reliably.
// A host cookie is shared across ports on 127.0.0.1 and is therefore suitable.
(() => {
  'use strict';

  const COOKIE = 'ddgLastUniversalSourceV1';
  const MAX_AGE = 365 * 24 * 60 * 60;

  function readCookie() {
    try {
      const prefix = COOKIE + '=';
      for (const part of String(document.cookie || '').split(';')) {
        const item = part.trim();
        if (!item.startsWith(prefix)) continue;
        return decodeURIComponent(item.slice(prefix.length));
      }
    } catch (_) {}
    return '';
  }

  function writeCookie(value) {
    const raw = String(value || '').trim();
    if (!raw) return;
    try {
      document.cookie = `${COOKIE}=${encodeURIComponent(raw)}; Max-Age=${MAX_AGE}; Path=/; SameSite=Lax`;
    } catch (_) {}
    // Same-port fallback; harmless when the cookie is available.
    try { localStorage.setItem('ddg.lastUniversalSourceUrl', raw); } catch (_) {}
  }

  function savedValue() {
    const cookie = String(readCookie() || '').trim();
    if (cookie) return cookie;
    try { return String(localStorage.getItem('ddg.lastUniversalSourceUrl') || '').trim(); } catch (_) { return ''; }
  }

  function input() {
    return document.getElementById('directUrl');
  }

  function restore() {
    const el = input();
    if (!el) return false;
    if (!String(el.value || '').trim()) {
      const saved = savedValue();
      if (saved) {
        el.value = saved;
        el.dispatchEvent(new Event('input', {bubbles: true}));
      }
    }
    return true;
  }

  function rememberCurrent() {
    const el = input();
    if (!el) return;
    writeCookie(el.value);
  }

  function boot() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      if (restore() || tries >= 50) clearInterval(timer);
    }, 100);

    document.addEventListener('click', event => {
      const target = event.target?.closest?.('#universalScanButton, button[onclick="scanUniversal()"]');
      if (target) rememberCurrent();
    }, true);

    document.addEventListener('keydown', event => {
      if (event.key === 'Enter' && event.target?.id === 'directUrl') rememberCurrent();
    }, true);

    document.addEventListener('change', event => {
      if (event.target?.id === 'directUrl') rememberCurrent();
    }, true);

    window.addEventListener('beforeunload', rememberCurrent);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, {once: true});
  } else {
    boot();
  }

  window.ddgSourceLinkPersistenceV8553 = {restore, rememberCurrent};
})();
