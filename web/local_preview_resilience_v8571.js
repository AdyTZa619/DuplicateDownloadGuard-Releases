// TEST v8.5.72 — resilient LOCAL preview, single-winner pipeline.
// Fast direct <img> is tried first. If it stalls, DDG cancels that path before
// starting the local Blob fallback, so the same file is never raced by two
// preview paths. Async callbacks are generation-guarded: stale requests cannot
// overwrite a newer/successful preview. No HDD rescan, MEGA traffic or JD changes.
(() => {
  'use strict';

  const DIRECT_WATCHDOG_MS = 1250;
  const FETCH_TIMEOUT_MS = 7000;
  const PROBE_BYTES = 64;

  let activeBlobURL = '';
  let previewGeneration = 0;
  const watchdogs = new WeakMap();
  const fetchControllers = new WeakMap();

  const MIME_BY_EXT = {
    jpg: 'image/jpeg', jpeg: 'image/jpeg', jpe: 'image/jpeg', jfif: 'image/jpeg',
    png: 'image/png', gif: 'image/gif', webp: 'image/webp',
    bmp: 'image/bmp', avif: 'image/avif', tif: 'image/tiff', tiff: 'image/tiff',
    heic: 'image/heic', heif: 'image/heif'
  };

  function esc(value) {
    if (typeof window.esc === 'function') return window.esc(value);
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot',"'":'&#39;'}[ch]));
  }

  function extOf(path) {
    const name = String(path || '').split(/[\\/]/).pop() || '';
    const dot = name.lastIndexOf('.');
    return dot >= 0 ? name.slice(dot + 1).toLowerCase() : '';
  }

  function kindOf(path) {
    const ext = extOf(path);
    if (Object.prototype.hasOwnProperty.call(MIME_BY_EXT, ext)) return 'image';
    if (typeof window.previewKind === 'function') return window.previewKind(path);
    if (['mp4','webm','ogv','mov','m4v','mkv','avi','flv','ts','mts','m2ts'].includes(ext)) return 'video';
    if (['mp3','wav','ogg','m4a','aac','flac','opus'].includes(ext)) return 'audio';
    return 'other';
  }

  function directURL(path) {
    return `/api/local-preview?path=${encodeURIComponent(path)}&_ddg=${Date.now()}`;
  }

  function releaseBlob() {
    if (!activeBlobURL) return;
    try { URL.revokeObjectURL(activeBlobURL); } catch (_) {}
    activeBlobURL = '';
  }

  function generationOf(img) {
    return Number(img?.dataset?.ddgGeneration || 0);
  }

  function isCurrent(img, generation = generationOf(img)) {
    if (!img || !img.isConnected) return false;
    if (generation !== previewGeneration || generationOf(img) !== previewGeneration) return false;
    return document.getElementById('localImage') === img;
  }

  function clearWatchdog(img) {
    const timer = watchdogs.get(img);
    if (timer) clearTimeout(timer);
    watchdogs.delete(img);
  }

  function cancelFallback(img) {
    const controller = fetchControllers.get(img);
    if (controller) {
      try { controller.abort(); } catch (_) {}
    }
    fetchControllers.delete(img);
  }

  function cancelPreviewWork(img) {
    if (!img) return;
    clearWatchdog(img);
    cancelFallback(img);
  }

  function setLocalType(text, title = '', img = null, generation = null) {
    if (img && !isCurrent(img, generation ?? generationOf(img))) return;
    const el = document.getElementById('localType');
    if (!el) return;
    el.textContent = text;
    el.title = title || '';
  }

  function errorButtons() {
    return '<button class="btn primary" onclick="playLocal()">▶ Local extern</button> <button class="btn" onclick="openLocal()">⌕ Explorer</button>';
  }

  function renderError(element, title, detail, generation = generationOf(element)) {
    if (!isCurrent(element, generation)) return;
    if (element?.dataset?.ddgSettled === 'success') return;
    cancelPreviewWork(element);
    const box = document.getElementById('localPreview') || element?.parentElement;
    if (!box || !isCurrent(element, generation)) return;
    const path = String(element?.dataset?.ddgLocalPath || '').trim();
    const mime = String(element?.dataset?.ddgDetectedMime || '').trim();
    const extra = mime ? `<br><br><span class="sourcePill">format detectat: ${esc(mime)}</span>` : '';
    box.innerHTML = `<div class="previewEmpty"><b>${esc(title)}</b><br><br>${esc(detail)}${extra}${path ? `<br><br><span class="miniPath">${esc(path)}</span>` : ''}<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', detail);
  }

  function ascii(bytes, start, length) {
    let out = '';
    const end = Math.min(bytes.length, start + length);
    for (let i = start; i < end; i++) out += String.fromCharCode(bytes[i]);
    return out;
  }

  function sniffImageMime(input, headerType = '', ext = '') {
    const bytes = input instanceof Uint8Array ? input : new Uint8Array(input || 0);
    const b = i => bytes[i] ?? -1;
    if (bytes.length >= 3 && b(0) === 0xff && b(1) === 0xd8 && b(2) === 0xff) return {mime:'image/jpeg', source:'magic'};
    if (bytes.length >= 8 && b(0) === 0x89 && ascii(bytes, 1, 3) === 'PNG' && b(4) === 0x0d && b(5) === 0x0a && b(6) === 0x1a && b(7) === 0x0a) return {mime:'image/png', source:'magic'};
    if (bytes.length >= 6 && (ascii(bytes, 0, 6) === 'GIF87a' || ascii(bytes, 0, 6) === 'GIF89a')) return {mime:'image/gif', source:'magic'};
    if (bytes.length >= 12 && ascii(bytes, 0, 4) === 'RIFF' && ascii(bytes, 8, 4) === 'WEBP') return {mime:'image/webp', source:'magic'};
    if (bytes.length >= 2 && ascii(bytes, 0, 2) === 'BM') return {mime:'image/bmp', source:'magic'};
    if (bytes.length >= 4 && ((b(0) === 0x49 && b(1) === 0x49 && b(2) === 0x2a && b(3) === 0x00) || (b(0) === 0x4d && b(1) === 0x4d && b(2) === 0x00 && b(3) === 0x2a))) return {mime:'image/tiff', source:'magic'};
    if (bytes.length >= 12 && ascii(bytes, 4, 4) === 'ftyp') {
      const brands = ascii(bytes, 8, Math.min(40, bytes.length - 8)).toLowerCase();
      if (/(avif|avis)/.test(brands)) return {mime:'image/avif', source:'magic'};
      if (/(heic|heix|hevc|hevx|heim|heis|mif1|msf1)/.test(brands)) return {mime:'image/heic', source:'magic'};
    }
    const header = String(headerType || '').split(';')[0].trim().toLowerCase();
    if (header.startsWith('image/')) return {mime:header, source:'header'};
    const byExt = MIME_BY_EXT[String(ext || '').toLowerCase()];
    if (byExt) return {mime:byExt, source:'extension'};
    return {mime:'application/octet-stream', source:'unknown'};
  }

  async function fetchWithTimeout(url, options, img) {
    const controller = new AbortController();
    fetchControllers.set(img, controller);
    const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
    try {
      return await fetch(url, {...options, signal: controller.signal});
    } finally {
      clearTimeout(timer);
      if (fetchControllers.get(img) === controller) fetchControllers.delete(img);
    }
  }

  function stopDirectRequest(img, generation) {
    if (!isCurrent(img, generation)) return false;
    img.dataset.ddgLocalStage = 'blob-fetch';
    // Changing/removing src asks WebView to cancel the old direct image request.
    // Any resulting load/error event is ignored while stage === blob-fetch.
    try { img.removeAttribute('src'); } catch (_) {}
    return true;
  }

  async function blobRetry(img, reason = 'error') {
    if (!img || img.dataset.ddgLocalRetry === '1') return;
    const generation = generationOf(img);
    if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
    const path = String(img.dataset.ddgLocalPath || '').trim();
    if (!path) return renderError(img, 'Preview local indisponibil', 'Calea fișierului local lipsește.', generation);

    clearWatchdog(img);
    img.dataset.ddgLocalRetry = '1';
    if (!stopDirectRequest(img, generation)) return;
    setLocalType(reason === 'watchdog' ? 'IMAGE • FALLBACK' : 'IMAGE • RETRY', reason === 'watchdog' ? 'Preview-ul direct nu a răspuns la timp; îl opresc și încerc fallback local.' : 'Reîncarc imaginea locală prin fallback Blob.', img, generation);

    try {
      const probe = await fetchWithTimeout(directURL(path), {
        cache: 'no-store',
        headers: {'Accept':'image/*,*/*;q=0.8', 'Range': `bytes=0-${PROBE_BYTES - 1}`}
      }, img);
      if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
      if (!probe.ok) throw new Error(`HTTP ${probe.status}`);

      const probeBuffer = await probe.arrayBuffer();
      if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
      if (!probeBuffer.byteLength) throw new Error('fișier gol');
      const ext = extOf(path);
      const detected = sniffImageMime(new Uint8Array(probeBuffer), probe.headers.get('Content-Type') || '', ext);
      img.dataset.ddgDetectedMime = detected.mime;

      let fullBuffer = probeBuffer;
      const partial = probe.status === 206 || Boolean(probe.headers.get('Content-Range'));
      if (partial) {
        const full = await fetchWithTimeout(directURL(path), {
          cache: 'no-store',
          headers: {'Accept':'image/*,*/*;q=0.8'}
        }, img);
        if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
        if (!full.ok) throw new Error(`HTTP ${full.status}`);
        fullBuffer = await full.arrayBuffer();
      }
      if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
      if (!fullBuffer.byteLength) throw new Error('fișier gol');

      releaseBlob();
      activeBlobURL = URL.createObjectURL(new Blob([fullBuffer], {type: detected.mime}));
      if (!isCurrent(img, generation)) {
        releaseBlob();
        return;
      }
      img.dataset.ddgLocalStage = 'blob';
      img.src = activeBlobURL;
      setLocalType('IMAGE • DECODARE', `${detected.mime} detectat prin ${detected.source}; aștept decoderul WebView.`, img, generation);
    } catch (error) {
      if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
      const message = error?.name === 'AbortError' ? `citirea locală a depășit ${Math.round(FETCH_TIMEOUT_MS / 1000)} secunde` : (error?.message || String(error));
      renderError(img, 'Imagine locală indisponibilă', `DDG nu a putut finaliza preview-ul local: ${message}`, generation);
    }
  }

  function onImageLoad(img) {
    if (!img) return;
    const generation = generationOf(img);
    if (!isCurrent(img, generation)) return;
    const stage = String(img.dataset.ddgLocalStage || 'direct');
    // A delayed event from the direct request may arrive after fallback began.
    // Ignore it completely: only one path is allowed to win.
    if (stage === 'blob-fetch') return;

    clearWatchdog(img);
    cancelFallback(img);
    img.dataset.ddgSettled = 'success';
    img.style.display = 'block';
    img.style.width = 'auto';
    img.style.height = 'auto';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
    img.style.objectFit = 'contain';

    if (stage === 'blob') {
      const mime = String(img.dataset.ddgDetectedMime || 'image').trim();
      setLocalType('IMAGE • FALLBACK OK', `Preview local încărcat prin Blob • ${mime}.`, img, generation);
    } else {
      setLocalType('IMAGE • DIRECT OK', 'Preview local încărcat direct.', img, generation);
    }

    requestAnimationFrame(() => {
      if (!isCurrent(img, generation) || img.dataset.ddgSettled !== 'success') return;
      const rect = img.getBoundingClientRect();
      if (img.naturalWidth > 0 && img.naturalHeight > 0 && (rect.width < 2 || rect.height < 2) && stage === 'direct') {
        img.dataset.ddgSettled = '';
        void blobRetry(img, 'zero-size');
      }
    });
  }

  function onImageError(img) {
    if (!img) return;
    const generation = generationOf(img);
    if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
    clearWatchdog(img);
    const stage = String(img.dataset.ddgLocalStage || 'direct');
    if (stage === 'direct') return void blobRetry(img, 'error');
    if (stage === 'blob-fetch') return;
    const mime = String(img.dataset.ddgDetectedMime || '').trim();
    renderError(img, 'Imagine locală nerecunoscută de WebView', mime ? `Fișierul a fost citit corect și identificat ca ${mime}, dar decoderul WebView nu îl poate afișa.` : 'Fișierul există, dar WebView nu îl poate decoda. Deschide copia în aplicația externă.', generation);
  }

  function armWatchdog(img) {
    if (!img) return;
    const generation = generationOf(img);
    if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
    clearWatchdog(img);
    if (img.complete) {
      if (img.naturalWidth > 0 && img.naturalHeight > 0) return onImageLoad(img);
      return void blobRetry(img, 'complete-without-image');
    }
    const timer = setTimeout(() => {
      watchdogs.delete(img);
      if (!isCurrent(img, generation) || img.dataset.ddgSettled === 'success') return;
      if (String(img.dataset.ddgLocalStage || '') !== 'direct') return;
      if (img.complete && img.naturalWidth > 0) return onImageLoad(img);
      void blobRetry(img, 'watchdog');
    }, DIRECT_WATCHDOG_MS);
    watchdogs.set(img, timer);
  }

  function onMediaError(media) {
    const path = String(media?.dataset?.ddgLocalPath || '').trim();
    const ext = extOf(path).toUpperCase() || 'MEDIA';
    const box = document.getElementById('localPreview') || media?.parentElement;
    if (!box) return;
    box.innerHTML = `<div class="previewEmpty"><b>${esc(`Preview local ${ext} indisponibil`)}</b><br><br>Fișierul local poate folosi un container/codec pe care WebView nu îl redă.<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', 'Folosește playerul extern.');
  }

  function robustLocalPreviewHTML(path) {
    const old = document.getElementById('localImage');
    if (old) cancelPreviewWork(old);
    releaseBlob();
    previewGeneration += 1;
    const generation = previewGeneration;
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';

    const kind = kindOf(path);
    const ext = extOf(path).toUpperCase();
    const safePath = esc(path);
    const url = directURL(path);

    if (kind === 'image') {
      setTimeout(() => {
        const img = document.querySelector(`#localImage[data-ddg-generation="${generation}"]`);
        if (img) armWatchdog(img);
      }, 0);
      return `<img id="localImage" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" data-ddg-local-stage="direct" data-ddg-settled="" src="${url}" alt="Preview local" style="display:block;width:auto;height:auto;max-width:100%;max-height:420px;object-fit:contain" onload="ddgLocalPreviewV8571.onImageLoad(this)" onerror="ddgLocalPreviewV8571.onImageError(this)"><span class="miniInfo">${esc(ext)}</span>`;
    }
    if (kind === 'video') {
      return `<video id="localVideo" data-ddg-local-path="${safePath}" controls preload="metadata" src="${url}" onerror="ddgLocalPreviewV8571.onMediaError(this)"></video><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    if (kind === 'audio') {
      return `<audio data-ddg-local-path="${safePath}" controls preload="metadata" src="${url}" onerror="ddgLocalPreviewV8571.onMediaError(this)"></audio><span class="miniInfo">${esc(ext)}</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Apasă <b>Local extern</b> pentru aplicația Windows/VLC/MPC-HC.</div>';
  }

  function installStyles() {
    if (document.getElementById('ddgLocalPreviewV8571Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgLocalPreviewV8571Style';
    style.textContent = `
      #localPreview img{object-fit:contain!important;align-self:center;justify-self:center}
      #localPreview .miniPath{display:inline-block;max-width:100%;font-family:Consolas,monospace;font-size:11px;color:#9eb2c8;white-space:normal;word-break:break-all}
    `;
    document.head.appendChild(style);
  }

  function install() {
    installStyles();
    window.localPreviewHTML = robustLocalPreviewHTML;
  }

  window.ddgLocalPreviewV8571 = {
    DIRECT_WATCHDOG_MS,
    FETCH_TIMEOUT_MS,
    sniffImageMime,
    isCurrent,
    armWatchdog,
    onImageLoad,
    onImageError,
    onMediaError,
    blobRetry,
    localPreviewHTML: robustLocalPreviewHTML
  };

  window.addEventListener('beforeunload', () => {
    releaseBlob();
    const img = document.getElementById('localImage');
    if (img) cancelPreviewWork(img);
  }, {once:true});

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();