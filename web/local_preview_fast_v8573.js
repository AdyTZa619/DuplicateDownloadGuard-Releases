// TEST v8.5.73 — fast LOCAL preview without cache-busting the normal path.
// Normal images/video/audio use one stable Range-capable /api/local-preview URL so
// Edge/WebView and Windows can reuse cache. Blob fallback is reserved for actual
// image failures or a genuine long stall; it never runs after 1.25s anymore.
(() => {
  'use strict';

  const IMAGE_HARD_TIMEOUT_MS = 8000;
  const FALLBACK_FETCH_TIMEOUT_MS = 12000;

  let previewGeneration = 0;
  let activeBlobURL = '';
  let imageTimer = 0;
  let fallbackController = null;

  const MIME_BY_EXT = {
    jpg: 'image/jpeg', jpeg: 'image/jpeg', jpe: 'image/jpeg', jfif: 'image/jpeg',
    png: 'image/png', gif: 'image/gif', webp: 'image/webp', bmp: 'image/bmp',
    avif: 'image/avif', tif: 'image/tiff', tiff: 'image/tiff',
    heic: 'image/heic', heif: 'image/heif'
  };

  function esc(value) {
    if (typeof window.esc === 'function') return window.esc(value);
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
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

  function stableURL(path) {
    return `/api/local-preview?path=${encodeURIComponent(path)}`;
  }

  function freshURL(path) {
    return `${stableURL(path)}&_ddg=${Date.now()}`;
  }

  function setLocalType(text, title = '') {
    const el = document.getElementById('localType');
    if (!el) return;
    el.textContent = text;
    el.title = title || '';
  }

  function clearImageTimer() {
    if (imageTimer) clearTimeout(imageTimer);
    imageTimer = 0;
  }

  function abortFallback() {
    if (fallbackController) {
      try { fallbackController.abort(); } catch (_) {}
    }
    fallbackController = null;
  }

  function releaseBlob() {
    if (!activeBlobURL) return;
    try { URL.revokeObjectURL(activeBlobURL); } catch (_) {}
    activeBlobURL = '';
  }

  function cancelActive() {
    clearImageTimer();
    abortFallback();
    releaseBlob();
  }

  function generationOf(el) {
    return Number(el?.dataset?.ddgGeneration || 0);
  }

  function isCurrent(el) {
    return Boolean(el && el.isConnected && generationOf(el) === previewGeneration);
  }

  function errorButtons() {
    return '<button class="btn primary" onclick="playLocal()">▶ Local extern</button> <button class="btn" onclick="openLocal()">⌕ Explorer</button>';
  }

  function renderError(el, title, detail) {
    if (!isCurrent(el)) return;
    clearImageTimer();
    abortFallback();
    const box = document.getElementById('localPreview') || el.parentElement;
    if (!box) return;
    const path = String(el.dataset.ddgLocalPath || '').trim();
    box.innerHTML = `<div class="previewEmpty"><b>${esc(title)}</b><br><br>${esc(detail)}${path ? `<br><br><span class="miniPath">${esc(path)}</span>` : ''}<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', detail);
  }

  function sniffMime(bytes, ext, headerType = '') {
    const b = i => bytes[i] ?? -1;
    const ascii = (start, len) => String.fromCharCode(...bytes.slice(start, start + len));
    if (bytes.length >= 3 && b(0) === 0xff && b(1) === 0xd8 && b(2) === 0xff) return 'image/jpeg';
    if (bytes.length >= 8 && b(0) === 0x89 && ascii(1, 3) === 'PNG') return 'image/png';
    if (bytes.length >= 6 && (ascii(0, 6) === 'GIF87a' || ascii(0, 6) === 'GIF89a')) return 'image/gif';
    if (bytes.length >= 12 && ascii(0, 4) === 'RIFF' && ascii(8, 4) === 'WEBP') return 'image/webp';
    if (bytes.length >= 2 && ascii(0, 2) === 'BM') return 'image/bmp';
    const header = String(headerType || '').split(';')[0].trim().toLowerCase();
    if (header.startsWith('image/')) return header;
    return MIME_BY_EXT[ext] || 'application/octet-stream';
  }

  async function blobFallback(img, reason) {
    if (!isCurrent(img) || img.dataset.ddgRetry === '1' || img.dataset.ddgSettled === '1') return;
    const path = String(img.dataset.ddgLocalPath || '').trim();
    if (!path) return renderError(img, 'Imagine locală indisponibilă', 'Calea locală lipsește.');

    clearImageTimer();
    img.dataset.ddgRetry = '1';
    img.dataset.ddgStage = 'fallback-fetch';
    try { img.removeAttribute('src'); } catch (_) {}
    setLocalType('IMAGE • FALLBACK', reason === 'stall' ? 'Încărcarea directă a rămas blocată peste 8 secunde; încerc fallback local.' : 'Încărcarea directă a eșuat; încerc fallback local.');

    const controller = new AbortController();
    fallbackController = controller;
    const timeout = setTimeout(() => controller.abort(), FALLBACK_FETCH_TIMEOUT_MS);
    try {
      const response = await fetch(freshURL(path), {
        cache: 'no-store',
        signal: controller.signal,
        headers: {'Accept':'image/*,*/*;q=0.8'}
      });
      if (!isCurrent(img)) return;
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const buffer = await response.arrayBuffer();
      if (!isCurrent(img)) return;
      if (!buffer.byteLength) throw new Error('fișier gol');
      const mime = sniffMime(new Uint8Array(buffer.slice(0, 64)), extOf(path), response.headers.get('Content-Type') || '');
      releaseBlob();
      activeBlobURL = URL.createObjectURL(new Blob([buffer], {type: mime}));
      img.dataset.ddgStage = 'blob';
      img.src = activeBlobURL;
    } catch (error) {
      if (!isCurrent(img)) return;
      const msg = error?.name === 'AbortError' ? 'fallback-ul local a depășit 12 secunde' : (error?.message || String(error));
      renderError(img, 'Imagine locală indisponibilă', msg);
    } finally {
      clearTimeout(timeout);
      if (fallbackController === controller) fallbackController = null;
    }
  }

  function armImageHardTimeout(img) {
    clearImageTimer();
    if (!isCurrent(img) || img.dataset.ddgSettled === '1') return;
    imageTimer = setTimeout(() => {
      imageTimer = 0;
      if (!isCurrent(img) || img.dataset.ddgSettled === '1') return;
      if (img.complete && img.naturalWidth > 0) return onImageLoad(img);
      void blobFallback(img, 'stall');
    }, IMAGE_HARD_TIMEOUT_MS);
  }

  function onImageLoad(img) {
    if (!isCurrent(img)) return;
    clearImageTimer();
    abortFallback();
    img.dataset.ddgSettled = '1';
    img.style.display = 'block';
    img.style.width = 'auto';
    img.style.height = 'auto';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
    img.style.objectFit = 'contain';
    const stage = String(img.dataset.ddgStage || 'direct');
    setLocalType(stage === 'blob' ? 'IMAGE • FALLBACK OK' : 'IMAGE • DIRECT OK', stage === 'blob' ? 'Preview local încărcat prin fallback.' : 'Preview local încărcat direct/cached.');
  }

  function onImageError(img) {
    if (!isCurrent(img) || img.dataset.ddgSettled === '1') return;
    if (String(img.dataset.ddgStage || 'direct') === 'fallback-fetch') return;
    if (String(img.dataset.ddgStage || '') === 'blob') return renderError(img, 'Imagine locală nerecunoscută de WebView', 'Fișierul există, dar decoderul WebView nu îl poate afișa.');
    void blobFallback(img, 'error');
  }

  function onMediaReady(media, label) {
    if (!media || generationOf(media) !== previewGeneration) return;
    setLocalType(`${label} • READY`, 'Metadatele locale au fost încărcate prin endpoint-ul Range-capable, fără cache-busting.');
  }

  function onMediaError(media) {
    if (!media || generationOf(media) !== previewGeneration) return;
    const ext = extOf(media.dataset.ddgLocalPath || '').toUpperCase() || 'MEDIA';
    const box = document.getElementById('localPreview') || media.parentElement;
    if (!box) return;
    box.innerHTML = `<div class="previewEmpty"><b>${esc(`Preview local ${ext} indisponibil`)}</b><br><br>Containerul/codecul nu poate fi redat de WebView. Fișierul poate fi deschis în playerul extern.<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', 'WebView nu poate reda acest format/codec.');
  }

  function fastLocalPreviewHTML(path) {
    cancelActive();
    previewGeneration += 1;
    const generation = previewGeneration;
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';

    const kind = kindOf(path);
    const ext = extOf(path).toUpperCase();
    const safePath = esc(path);
    const stable = stableURL(path);

    if (kind === 'image') {
      setTimeout(() => {
        const img = document.querySelector(`#localImage[data-ddg-generation="${generation}"]`);
        if (img) armImageHardTimeout(img);
      }, 0);
      return `<img id="localImage" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" data-ddg-stage="direct" data-ddg-settled="0" src="${stable}" alt="Preview local" style="display:block;width:auto;height:auto;max-width:100%;max-height:420px;object-fit:contain" onload="ddgLocalPreviewFastV8573.onImageLoad(this)" onerror="ddgLocalPreviewFastV8573.onImageError(this)"><span class="miniInfo">${esc(ext)}</span>`;
    }
    if (kind === 'video') {
      return `<video id="localVideo" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" controls playsinline preload="metadata" src="${stable}" onloadedmetadata="ddgLocalPreviewFastV8573.onMediaReady(this,'VIDEO')" onerror="ddgLocalPreviewFastV8573.onMediaError(this)"></video><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    if (kind === 'audio') {
      return `<audio data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" controls preload="metadata" src="${stable}" onloadedmetadata="ddgLocalPreviewFastV8573.onMediaReady(this,'AUDIO')" onerror="ddgLocalPreviewFastV8573.onMediaError(this)"></audio><span class="miniInfo">${esc(ext)}</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Apasă <b>Local extern</b> pentru aplicația Windows/VLC/MPC-HC.</div>';
  }

  function install() {
    window.localPreviewHTML = fastLocalPreviewHTML;
  }

  window.ddgLocalPreviewFastV8573 = {
    onImageLoad,
    onImageError,
    onMediaReady,
    onMediaError,
    blobFallback,
    localPreviewHTML: fastLocalPreviewHTML
  };

  window.addEventListener('beforeunload', cancelActive, {once:true});
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();