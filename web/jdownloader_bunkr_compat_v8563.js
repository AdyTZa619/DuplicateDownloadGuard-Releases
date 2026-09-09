// JDownloader/Bunkr compatibility shim for TEST builds.
// Current JDownloader's Bunkr plugin accepts public /d/, /i/ and /v/ URLs,
// while DDG's earlier handoff used gallery-dl's /f/ form. Rewrite only Bunkr
// FlashGot submissions, leaving GoFile/Cyberdrop/MEGA and JD settings untouched.
(() => {
  'use strict';

  function isBunkrHost(host) {
    return /(^|\.)bunkr+[a-z0-9-]*\.[a-z]{2,}$/i.test(String(host || ''));
  }

  function rewriteBunkrURL(raw) {
    try {
      const u = new URL(String(raw || '').trim());
      if (!isBunkrHost(u.hostname)) return raw;
      if (!/^\/f\//i.test(u.pathname)) return raw;
      // /d/ is JDownloader's generic Bunkr single-file route and works for
      // arbitrary file types. JD then resolves/refreshes the real CDN URL and
      // Referer using its maintained Bunkr plugin.
      u.pathname = u.pathname.replace(/^\/f\//i, '/d/');
      return u.toString();
    } catch (_) {
      return raw;
    }
  }

  function rewriteURLBlock(value) {
    return String(value || '').split(/\r?\n/).map(rewriteBunkrURL).join('\n');
  }

  function rewriteEncodedBody(body) {
    if (typeof body !== 'string') return body;
    try {
      const params = new URLSearchParams(body);
      if (!params.has('urls')) return body;
      params.set('urls', rewriteURLBlock(params.get('urls')));
      return params.toString();
    } catch (_) {
      return body;
    }
  }

  function isJDFlashGot(raw) {
    try {
      const u = new URL(typeof raw === 'string' ? raw : raw?.url || '', location.href);
      return (u.hostname === '127.0.0.1' || u.hostname === 'localhost') && u.port === '9666' && u.pathname === '/flashgot';
    } catch (_) {
      return false;
    }
  }

  const previousFetch = window.fetch.bind(window);
  window.fetch = function ddgBunkrJDFetch(input, init) {
    if (!isJDFlashGot(input)) return previousFetch(input, init);
    const next = {...(init || {})};
    next.body = rewriteEncodedBody(next.body);
    return previousFetch(input, next);
  };

  const nativeSubmit = HTMLFormElement.prototype.submit;
  HTMLFormElement.prototype.submit = function ddgBunkrJDFormSubmit() {
    try {
      if (isJDFlashGot(this.action)) {
        const field = this.querySelector('input[name="urls"], textarea[name="urls"]');
        if (field) field.value = rewriteURLBlock(field.value);
      }
    } catch (_) {}
    return nativeSubmit.call(this);
  };

  window.ddgJDownloaderBunkrCompatV8563 = {rewriteBunkrURL};
})();