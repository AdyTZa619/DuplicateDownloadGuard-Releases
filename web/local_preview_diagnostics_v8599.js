// TEST .102 LOCAL preview: BUFFER is primary for images and uses a dedicated
// loopback media origin when available. DIRECT stays as an explicit A/B method.
// No automatic timeout fallback, HDD scan, MEGA scan or JDownloader work.
(() => {
  'use strict';

  const TRACE_URL = '/api/local-preview/trace';
  const BASE_URL = '/api/local-preview/base';
  const PENDING_MS = 1500;
  const watched = new WeakSet();
  const state = new WeakMap();
  let dedicatedBase = '';

  function round(v) {
    return Number.isFinite(Number(v)) ? Math.max(0, Math.round(Number(v))) : 0;
  }

  function imageKind(path) {
    const e = String(path || '').split('.').pop().toLowerCase();
    return ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'avif'].includes(e);
  }

  function safeDedicatedBase(raw) {
    try {
      const u = new URL(String(raw || ''));
      if (u.protocol !== 'http:' || !['127.0.0.1', 'localhost'].includes(u.hostname)) return '';
      return u.origin;
    } catch (_) {
      return '';
    }
  }

  function mediaURL(path, method) {
    const endpoint = method === 'DIRECT' ? '/api/local-preview' : '/api/local-preview-buffered';
    return `${dedicatedBase}${endpoint}?path=${encodeURIComponent(path)}&_ddg=${Date.now()}`;
  }

  function installPrimaryRenderer() {
    const original = window.localPreviewHTML;
    if (typeof original !== 'function' || original.__ddgLocalPreview102) return;
    const wrapped = function(path) {
      if (!path || !imageKind(path)) return original(path);
      const ext = String(path).split('.').pop().toUpperCase();
      const src = mediaURL(path, 'BUFFER');
      return `<img id="localImage" src="${src}" alt="Preview local"><span class="miniInfo">${ext} • LOCAL BUFFER</span>`;
    };
    wrapped.__ddgLocalPreview102 = true;
    wrapped.__ddgOriginal = original;
    window.localPreviewHTML = wrapped;
  }

  async function resolveDedicatedBase() {
    try {
      const resp = await fetch(BASE_URL, {cache: 'no-store'});
      if (!resp.ok) return;
      const data = await resp.json();
      const base = safeDedicatedBase(data?.base);
      if (!base) return;
      dedicatedBase = base;
      installPrimaryRenderer();
    } catch (_) {}
  }

  function sourceInfo(el) {
    const raw = String(el?.currentSrc || el?.getAttribute?.('src') || '').trim();
    try {
      const u = new URL(raw, location.href);
      return {
        path: u.searchParams.get('path') || '',
        method: u.pathname.includes('/api/local-preview-buffered') ? 'BUFFER' : 'DIRECT',
        origin: u.origin === location.origin ? 'MAIN' : 'DEDICATED',
        url: u.href
      };
    } catch (_) {
      return {path: '', method: 'DIRECT', origin: 'UNKNOWN', url: raw};
    }
  }

  function resourceTiming(url) {
    if (!url || !window.performance?.getEntriesByName) return 'resource=n/a';
    const entries = performance.getEntriesByName(url);
    const e = entries?.[entries.length - 1];
    if (!e) return 'resource=n/a';
    const queued = e.requestStart > e.startTime ? e.requestStart - e.startTime : 0;
    const ttfb = e.responseStart > 0 && e.requestStart > 0 ? e.responseStart - e.requestStart : 0;
    const transfer = e.responseEnd > 0 && e.responseStart > 0 ? e.responseEnd - e.responseStart : 0;
    return `resource=${round(e.duration)}ms queue=${round(queued)}ms ttfb=${round(ttfb)}ms transfer=${round(transfer)}ms encoded=${round(e.encodedBodySize)} decoded=${round(e.decodedBodySize)}`;
  }

  function postTrace(event, el, elapsed, detail = '') {
    const info = sourceInfo(el);
    if (!info.path) return;
    const width = Number(el?.naturalWidth || el?.videoWidth || 0);
    const height = Number(el?.naturalHeight || el?.videoHeight || 0);
    fetch(TRACE_URL, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        event,
        path: info.path,
        method: info.method,
        elapsedMs: round(elapsed),
        naturalWidth: width,
        naturalHeight: height,
        detail: `${info.origin} ${String(detail || '')}`.slice(0, 180)
      }),
      keepalive: true
    }).catch(() => {});
  }

  function removeAlt(root) {
    root?.querySelector?.('.ddgLocalPreviewAltV8599')?.remove();
  }

  function ensureStyle() {
    if (document.getElementById('ddgLocalPreviewDiagStyleV8599')) return;
    const style = document.createElement('style');
    style.id = 'ddgLocalPreviewDiagStyleV8599';
    style.textContent = `
      .ddgLocalPreviewAltV8599{position:absolute;right:10px;top:10px;z-index:8;border:1px solid #4b6279;background:rgba(12,20,29,.94);color:#dbe9f8;border-radius:7px;padding:6px 9px;cursor:pointer;font-size:11px;font-weight:750}
      .ddgLocalPreviewAltV8599:hover{border-color:#78aee4}.ddgLocalPreviewAltV8599:disabled{opacity:.65;cursor:default}
    `;
    document.head.appendChild(style);
  }

  function offerAlternate(img, armPending) {
    const root = img?.closest?.('#localPreview');
    const s = state.get(img);
    const info = sourceInfo(img);
    if (!root || !s || !info.path || root.querySelector('.ddgLocalPreviewAltV8599')) return;
    ensureStyle();
    const target = info.method === 'BUFFER' ? 'DIRECT' : 'BUFFER';
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'ddgLocalPreviewAltV8599';
    button.textContent = target === 'BUFFER' ? 'ALT • BUFFER memorie' : 'ALT • DIRECT vechi';
    button.title = target === 'BUFFER'
      ? 'Compară cu citirea completă în memorie.'
      : 'Compară cu vechea metodă ServeFile. Ambele folosesc listenerul LOCAL dedicat când este disponibil.';
    button.addEventListener('click', event => {
      event.preventDefault();
      event.stopPropagation();
      const current = sourceInfo(img);
      if (!current.path || !img.isConnected) return;
      postTrace('ALT', img, performance.now() - s.started, `${current.method} -> ${target}; înlocuire unică, fără două citiri paralele`);
      clearTimeout(s.pendingTimer);
      s.started = performance.now();
      s.settled = false;
      button.remove();
      img.src = mediaURL(current.path, target);
      armPending();
    });
    root.appendChild(button);
  }

  function watchImage(img) {
    if (!img || watched.has(img)) return;
    watched.add(img);
    const s = {started: performance.now(), settled: false, pendingTimer: 0};
    state.set(img, s);

    const armPending = () => {
      clearTimeout(s.pendingTimer);
      s.pendingTimer = setTimeout(() => {
        if (!img.isConnected || s.settled) return;
        const info = sourceInfo(img);
        postTrace('PENDING', img, performance.now() - s.started, `${resourceTiming(info.url)} complete=${Boolean(img.complete)}`);
        offerAlternate(img, armPending);
      }, PENDING_MS);
    };

    const loaded = () => {
      if (s.settled) return;
      s.settled = true;
      clearTimeout(s.pendingTimer);
      const info = sourceInfo(img);
      postTrace('LOAD', img, performance.now() - s.started, `${resourceTiming(info.url)} complete=${Boolean(img.complete)}`);
      removeAlt(img.closest('#localPreview'));
    };

    const failed = () => {
      if (s.settled) return;
      s.settled = true;
      clearTimeout(s.pendingTimer);
      const info = sourceInfo(img);
      postTrace('ERROR', img, performance.now() - s.started, `${resourceTiming(info.url)} complete=${Boolean(img.complete)}`);
      s.settled = false;
      offerAlternate(img, armPending);
    };

    img.addEventListener('load', loaded);
    img.addEventListener('error', failed);
    armPending();

    // Very small buffered images can finish before MutationObserver attaches.
    if (img.complete) queueMicrotask(() => img.naturalWidth > 0 ? loaded() : failed());
  }

  function watchAV(el) {
    if (!el || watched.has(el)) return;
    watched.add(el);
    const s = {started: performance.now(), settled: false, pendingTimer: 0};
    state.set(el, s);
    s.pendingTimer = setTimeout(() => {
      if (!el.isConnected || s.settled) return;
      const info = sourceInfo(el);
      postTrace('PENDING', el, performance.now() - s.started, `${resourceTiming(info.url)} readyState=${el.readyState} networkState=${el.networkState}`);
    }, PENDING_MS);
    const done = eventName => {
      if (s.settled) return;
      s.settled = true;
      clearTimeout(s.pendingTimer);
      const info = sourceInfo(el);
      postTrace(eventName, el, performance.now() - s.started, `${resourceTiming(info.url)} readyState=${el.readyState} networkState=${el.networkState}`);
    };
    el.addEventListener('loadedmetadata', () => done('METADATA'));
    el.addEventListener('error', () => done('ERROR'));
    el.addEventListener('stalled', () => postTrace('STALLED', el, performance.now() - s.started, `readyState=${el.readyState} networkState=${el.networkState}`));
    el.addEventListener('waiting', () => postTrace('WAITING', el, performance.now() - s.started, `readyState=${el.readyState} networkState=${el.networkState}`));
  }

  function scan() {
    const root = document.getElementById('localPreview');
    if (!root) return;
    const image = root.querySelector('#localImage');
    if (image) watchImage(image);
    const video = root.querySelector('#localVideo, video');
    if (video) watchAV(video);
    const audio = root.querySelector('audio');
    if (audio) watchAV(audio);
  }

  function boot() {
    ensureStyle();
    installPrimaryRenderer();
    resolveDedicatedBase();
    scan();
    const root = document.getElementById('localPreview');
    if (!root || root.dataset.ddgLocalDiagObserverV8599 === '1') return;
    root.dataset.ddgLocalDiagObserverV8599 = '1';
    new MutationObserver(scan).observe(root, {childList: true, subtree: true});
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once: true});
  else boot();

  window.ddgLocalPreviewDiagnosticsV8599 = {scan, resolveDedicatedBase};
})();
