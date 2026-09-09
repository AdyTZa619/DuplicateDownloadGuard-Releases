// Generic-page discovery. It only discovers here; comparison starts after the
// user chooses items in Media Picker.
(() => {
  'use strict';

  const SERVICE = 'ddg-generic-media-v1';
  const PORT_BASE = 38950;
  const PORT_SPAN = 200;
  const PORT_ATTEMPTS = 8;
  let serviceURL = '';
  let servicePromise = null;
  let inFlight = false;
  let originalScanUniversal = null;

  const $ = id => document.getElementById(id);

  function fnv1a32(value) {
    let hash = 0x811c9dc5;
    const bytes = new TextEncoder().encode(String(value || '').toLowerCase());
    for (const byte of bytes) { hash ^= byte; hash = Math.imul(hash, 0x01000193) >>> 0; }
    return hash >>> 0;
  }

  function seedVariants(appDir) {
    const raw = String(appDir || '').trim();
    if (!raw) return [];
    const clean = raw.replace(/[\\/]+$/, '');
    const out = /[\\/]data$/i.test(clean) ? [clean] : [clean + (clean.includes('\\') ? '\\data' : '/data')];
    out.push(clean);
    return [...new Set(out.map(value => value.toLowerCase()))];
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
        for (let index = 0; index < PORT_ATTEMPTS; index++) {
          const candidate = `http://127.0.0.1:${base + index}`;
          try {
            const controller = new AbortController();
            const timer = setTimeout(() => controller.abort(), 450);
            const response = await fetch(candidate + '/health', {cache:'no-store', signal:controller.signal});
            clearTimeout(timer);
            if (!response.ok) continue;
            const data = await response.json();
            if (data?.ok && data?.service === SERVICE) { serviceURL = candidate; return serviceURL; }
          } catch (_) {}
        }
      }
      return '';
    })().finally(() => { servicePromise = null; });
    return servicePromise;
  }

  function currentURL() { return String($('directUrl')?.value || '').trim(); }
  function adapterIsAuto() { return String($('sourceAdapter')?.value || 'auto').trim().toLowerCase() === 'auto'; }
  function isGeneric(raw = currentURL()) { return !!window.ddgGenericMediaPickerV85114?.isGenericHTTP?.(raw); }
  function shouldIntercept() { return !inFlight && adapterIsAuto() && isGeneric(); }

  function setBusy(busy, detail = '') {
    const button = $('universalScanButton');
    if (!button) return;
    button.disabled = !!busy;
    button.textContent = busy ? (detail || 'Caut videoclipurile din pagină…') : 'Analizează fără download';
  }

  function normalizedDiscovery(raw, data, available = true) {
    return {
      available,
      url: raw,
      candidates: Array.isArray(data?.candidates) ? data.candidates : [],
      counts: data?.counts || {},
      warnings: Array.isArray(data?.warnings) ? data.warnings : []
    };
  }

  async function advancedDiscovery(raw) {
    try {
      const data = await api('/api/generic-media/discover', {
        method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({url:raw})
      });
      return normalizedDiscovery(raw, data, true);
    } catch (directError) {
      const base = await discoverService();
      if (!base) return normalizedDiscovery(raw, {warnings:[String(directError?.message || directError)]}, false);
      try {
        const data = await api(base + '/scan', {
          method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({url:raw})
        });
        const reply = normalizedDiscovery(raw, data, true);
        reply.warnings.push('preview proxy indisponibil; folosesc linkurile directe');
        return reply;
      } catch (fallbackError) {
        serviceURL = '';
        return normalizedDiscovery(raw, {warnings:[String(directError?.message || directError), String(fallbackError?.message || fallbackError)]}, false);
      }
    }
  }

  function discoverySummary(discovery) {
    const counts = discovery?.counts || {};
    const labels = [['html','HTML'], ['js','JS'], ['hls','HLS'], ['dash','DASH'], ['yt-dlp','yt-dlp'], ['gallery-dl','gallery-dl'], ['http','HTTP']];
    return labels.map(([key, label]) => Number(counts[key] || 0) > 0 ? `${label} ${Number(counts[key])}` : '').filter(Boolean).join(' • ');
  }

  async function runAdvancedGenericScan(force = false) {
    if (inFlight || !isGeneric() || (!force && !adapterIsAuto())) return false;
    const raw = currentURL();
    if (!raw) return false;
    inFlight = true;
    setBusy(true);
    const top = $('topStatus');
    if (top) top.textContent = 'Caut video, surse, iframe, HLS/DASH și calități…';
    window.dispatchEvent(new CustomEvent('ddg:source-scan-start', {detail:{url:raw, provider:'web', discoveryOnly:true}}));
    try {
      const discovery = await advancedDiscovery(raw);
      const found = discovery.candidates.length;
      const summary = discoverySummary(discovery);
      if (top) top.textContent = found
        ? `Media Picker: ${found.toLocaleString('ro-RO')} rezultate găsite${summary ? ` • ${summary}` : ''}`
        : 'Media Picker: extractor automat disponibil la comparare';
      window.dispatchEvent(new CustomEvent('ddg:generic-media-discovered', {detail:{url:raw, provider:'web', discovery}}));
      window.toast?.(found ? `${found.toLocaleString('ro-RO')} rezultate găsite. Alege ce compari.` : 'Nu am primit preview-uri; poți compara pagina prin fallback automat.');
      return true;
    } catch (error) {
      const message = String(error?.message || error || 'Eroare generică necunoscută');
      if (top) top.textContent = `Eroare Media Picker: ${message}`;
      window.dispatchEvent(new CustomEvent('ddg:source-scan-error', {detail:{url:raw, provider:'web', message, discoveryOnly:true}}));
      window.toast?.(message);
      return true;
    } finally {
      inFlight = false;
      setBusy(false);
    }
  }

  function interceptClick(event) {
    const button = event.target?.closest?.('#universalScanButton');
    if (!button || !shouldIntercept()) return;
    event.preventDefault(); event.stopImmediatePropagation(); void runAdvancedGenericScan();
  }

  function interceptEnter(event) {
    if (event.key !== 'Enter' || event.target?.id !== 'directUrl' || !shouldIntercept()) return;
    event.preventDefault(); event.stopImmediatePropagation(); void runAdvancedGenericScan();
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
    if (document.documentElement.dataset.ddgGenericAdvancedV85127 !== '1') {
      document.documentElement.dataset.ddgGenericAdvancedV85127 = '1';
      document.addEventListener('click', interceptClick, true);
      document.addEventListener('keydown', interceptEnter, true);
    }
    wrapProgrammaticScan();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true}); else bind();
  setTimeout(bind, 600);
  window.ddgGenericMediaDiscoveryV85127 = {discoverService, scan:advancedDiscovery, run:runAdvancedGenericScan, shouldIntercept, isGeneric};
})();
