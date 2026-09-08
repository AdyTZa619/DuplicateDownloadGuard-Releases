// TEST127 — advanced discovery for GENERIC web pages only.
// Dedicated providers keep their own flows. This module intercepts only the
// generic + AUTO scan and falls back to the existing /api/source/batch path if
// advanced discovery is unavailable or cannot produce usable direct URLs.
(() => {
  'use strict';

  const SERVICE = 'ddg-generic-media-v1';
  const PORT_BASE = 38950;
  const PORT_SPAN = 200;
  const PORT_ATTEMPTS = 8;
  const MAX_BATCH_URLS = 400;

  let serviceURL = '';
  let servicePromise = null;
  let inFlight = false;
  let originalScanUniversal = null;

  const $ = id => document.getElementById(id);

  function fnv1a32(value) {
    let hash = 0x811c9dc5;
    const bytes = new TextEncoder().encode(String(value || '').toLowerCase());
    for (const b of bytes) {
      hash ^= b;
      hash = Math.imul(hash, 0x01000193) >>> 0;
    }
    return hash >>> 0;
  }

  function seedVariants(appDir) {
    const raw = String(appDir || '').trim();
    if (!raw) return [];
    const clean = raw.replace(/[\\/]+$/, '');
    const out = [];
    if (/[\\/]data$/i.test(clean)) out.push(clean);
    else out.push(clean + (clean.includes('\\') ? '\\data' : '/data'));
    out.push(clean);
    return [...new Set(out.map(x => x.toLowerCase()))];
  }

  async function api(url, options) {
    const response = await fetch(url, {...options, cache:'no-store'});
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : response.text();
  }

  async function discoverService() {
    if (serviceURL) return serviceURL;
    if (servicePromise) return servicePromise;
    servicePromise = (async () => {
      let about = null;
      try { about = await api('/api/about'); } catch (_) {}
      const bases = [];
      for (const seed of seedVariants(about?.appDir)) {
        const base = PORT_BASE + (fnv1a32(seed) % PORT_SPAN);
        if (!bases.includes(base)) bases.push(base);
      }
      if (!bases.length) bases.push(PORT_BASE);

      for (const base of bases) {
        for (let i = 0; i < PORT_ATTEMPTS; i++) {
          const candidate = `http://127.0.0.1:${base + i}`;
          try {
            const ctl = new AbortController();
            const timer = setTimeout(() => ctl.abort(), 450);
            const response = await fetch(candidate + '/health', {cache:'no-store', signal:ctl.signal});
            clearTimeout(timer);
            if (!response.ok) continue;
            const data = await response.json();
            if (data?.ok && data?.service === SERVICE) {
              serviceURL = candidate;
              return serviceURL;
            }
          } catch (_) {}
        }
      }
      return '';
    })().finally(() => { servicePromise = null; });
    return servicePromise;
  }

  function currentURL() {
    return String($('directUrl')?.value || '').trim();
  }

  function adapterIsAuto() {
    return String($('sourceAdapter')?.value || 'auto').trim().toLowerCase() === 'auto';
  }

  function isGeneric(raw = currentURL()) {
    return !!window.ddgGenericMediaPickerV85114?.isGenericHTTP?.(raw);
  }

  function shouldIntercept() {
    return !inFlight && adapterIsAuto() && isGeneric();
  }

  function setBusy(busy, detail = '') {
    const button = $('universalScanButton');
    if (button) {
      button.disabled = !!busy;
      button.textContent = busy ? (detail || 'Analizez pagina generică…') : 'Analizează fără download';
    }
  }

  async function advancedDiscovery(raw) {
    const base = await discoverService();
    if (!base) return {available:false, candidates:[], counts:{}, warnings:['motorul generic avansat nu a răspuns']};
    try {
      const data = await api(base + '/scan', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({url:raw})
      });
      return {
        available:true,
        candidates:Array.isArray(data?.candidates) ? data.candidates : [],
        counts:data?.counts || {},
        warnings:Array.isArray(data?.warnings) ? data.warnings : []
      };
    } catch (error) {
      serviceURL = '';
      return {available:false, candidates:[], counts:{}, warnings:[String(error?.message || error)]};
    }
  }

  function uniqueHTTPURLs(candidates) {
    const seen = new Set();
    const out = [];
    for (const item of candidates || []) {
      const raw = String(item?.url || '').trim();
      if (!/^https?:\/\//i.test(raw) || seen.has(raw)) continue;
      seen.add(raw);
      out.push(raw);
      if (out.length >= MAX_BATCH_URLS) break;
    }
    return out;
  }

  function discoverySummary(discovery) {
    const counts = discovery?.counts || {};
    const parts = [];
    const labels = [
      ['html','HTML'], ['js','JS'], ['hls','HLS'], ['dash','DASH'],
      ['yt-dlp','yt-dlp'], ['gallery-dl','gallery-dl'], ['http','HTTP']
    ];
    for (const [key, label] of labels) {
      const value = Number(counts[key] || 0);
      if (value > 0) parts.push(`${label} ${value}`);
    }
    return parts.join(' • ');
  }

  async function runAdvancedGenericScan() {
    if (!shouldIntercept()) return false;
    const raw = currentURL();
    if (!raw) return false;

    inFlight = true;
    setBusy(true);
    const top = $('topStatus');
    if (top) top.textContent = 'Analizez HTML / iframe / HLS / DASH + extractoare…';
    window.dispatchEvent(new CustomEvent('ddg:source-scan-start', {detail:{url:raw, provider:'web', advanced:true}}));

    try {
      const mode = $('mode')?.value || 'balanced';
      const discovery = await advancedDiscovery(raw);
      const discoveredURLs = uniqueHTTPURLs(discovery.candidates);
      let data = null;
      let usedAdvanced = false;
      let directError = null;

      if (discoveredURLs.length) {
        setBusy(true, `Compar ${discoveredURLs.length} media…`);
        try {
          data = await api('/api/source/batch', {
            method:'POST',
            headers:{'Content-Type':'application/json'},
            body:JSON.stringify({urls:discoveredURLs, mode, adapter:'http'})
          });
          usedAdvanced = true;
        } catch (error) {
          directError = error;
        }
      }

      if (!data) {
        // Safe fallback: preserve the proven TEST126 generic chain instead of
        // turning an extractor/CDN failure into a broken scan.
        setBusy(true, 'Fallback yt-dlp / gallery-dl…');
        data = await api('/api/source/batch', {
          method:'POST',
          headers:{'Content-Type':'application/json'},
          body:JSON.stringify({urls:[raw], mode, adapter:'auto'})
        });
      }

      const items = Number(data?.items || 0);
      const summary = discoverySummary(discovery);
      const suffix = usedAdvanced && summary ? ` • ${summary}` : '';
      window.toast?.(`Web: ${items.toLocaleString('ro-RO')} fișier(e) comparate${suffix}`);
      if (top) top.textContent = usedAdvanced
        ? `Web generic: ${items.toLocaleString('ro-RO')} media • ${summary || 'detecție avansată'}`
        : 'Web generic: fallback terminat';

      window.dispatchEvent(new CustomEvent('ddg:source-scan-complete', {detail:{
        url:raw,
        provider:'web',
        adapter:usedAdvanced ? 'generic-advanced' : 'auto',
        items,
        advanced:usedAdvanced,
        discovered:discoveredURLs.length,
        counts:discovery.counts || {},
        warnings:discovery.warnings || [],
        fallbackError:directError ? String(directError?.message || directError) : ''
      }}));
      if (typeof window.goTab === 'function') window.goTab('results');
      return true;
    } catch (error) {
      const message = String(error?.message || error || 'Eroare generică necunoscută');
      window.toast?.(message);
      if (top) top.textContent = `Eroare sursă generică: ${message}`;
      window.dispatchEvent(new CustomEvent('ddg:source-scan-error', {detail:{url:raw, provider:'web', message, advanced:true}}));
      return true;
    } finally {
      inFlight = false;
      setBusy(false);
    }
  }

  function interceptClick(event) {
    const button = event.target?.closest?.('#universalScanButton');
    if (!button || !shouldIntercept()) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    void runAdvancedGenericScan();
  }

  function interceptEnter(event) {
    if (event.key !== 'Enter' || event.target?.id !== 'directUrl' || !shouldIntercept()) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    void runAdvancedGenericScan();
  }

  function wrapProgrammaticScan() {
    if (window.scanUniversal?.__ddgGeneric127Wrapped) return;
    originalScanUniversal = window.scanUniversal;
    const wrapped = function(...args) {
      if (shouldIntercept()) return runAdvancedGenericScan();
      return typeof originalScanUniversal === 'function' ? originalScanUniversal.apply(this, args) : undefined;
    };
    wrapped.__ddgGeneric127Wrapped = true;
    window.scanUniversal = wrapped;
  }

  function bind() {
    if (document.documentElement.dataset.ddgGenericAdvancedV85127 === '1') return;
    document.documentElement.dataset.ddgGenericAdvancedV85127 = '1';
    document.addEventListener('click', interceptClick, true);
    document.addEventListener('keydown', interceptEnter, true);
    wrapProgrammaticScan();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true});
  else bind();
  setTimeout(() => { bind(); wrapProgrammaticScan(); }, 600);

  window.ddgGenericMediaDiscoveryV85127 = {
    discoverService,
    scan:advancedDiscovery,
    run:runAdvancedGenericScan,
    shouldIntercept,
    isGeneric
  };
})();
