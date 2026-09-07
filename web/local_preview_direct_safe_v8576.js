(() => {
  'use strict';

  // TEST93: native browser image formats stay on the original direct request.
  // Do NOT abort a slow JPEG/PNG/WebP/AVIF read just because the HDD needed a
  // few seconds to answer: aborting and starting FFmpeg was the source of the
  // long delays seen in TEST92. Only a real direct decode/load error switches
  // to the safe JPEG derivative. Formats that WebView normally cannot decode
  // reliably (HEIC/HEIF/TIFF) go straight to the safe derivative.
  const SAFE_FIRST_IMAGE_EXTS = new Set(['heic', 'heif', 'tif', 'tiff']);

  function escAttr(value) {
    return String(value ?? '')
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  function extOf(path) {
    const m = String(path || '').match(/\.([^.\\/]+)$/);
    return m ? m[1].toUpperCase() : '';
  }

  function kindOf(path) {
    const ext = extOf(path).toLowerCase();
    if (['jpg','jpeg','jpe','jfif','png','gif','webp','bmp','avif','heic','heif','tif','tiff'].includes(ext)) return 'image';
    if (['mp4','webm','ogv','mov','m4v','mkv','avi','flv','ts','mts','m2ts'].includes(ext)) return 'video';
    if (['mp3','wav','ogg','m4a','aac','flac','opus'].includes(ext)) return 'audio';
    return 'other';
  }

  function directURL(path) {
    return '/api/local-preview?path=' + encodeURIComponent(path);
  }

  function safeURL(path) {
    return '/api/local-thumb?path=' + encodeURIComponent(path);
  }

  function renderFailure(img, message) {
    if (!img || !img.isConnected) return;
    const host = img.parentElement;
    if (!host) return;
    host.innerHTML = `<div class="previewEmpty"><b>Imagine locală indisponibilă.</b><br>${message}<br><br>Poți folosi <b>Redă local</b> / aplicația Windows.</div>`;
  }

  function switchSafe(img, why) {
    if (!img || !img.isConnected) return;
    if (img.dataset.ddgStage !== 'direct') return;

    img.dataset.ddgStage = 'switching';
    const path = img.dataset.ddgPath || '';
    if (!path) return renderFailure(img, 'Calea locală lipsește.');

    // A fallback is allowed only after a real direct load/decode error. Stop
    // that failed request before starting the safe derivative so we still have
    // at most one HDD read for the selected image.
    img.removeAttribute('src');
    img.dataset.ddgStage = 'safe';
    img.dataset.ddgWhy = why || 'direct-error';
    img.src = safeURL(path);
  }

  function imageReady(img) {
    if (!img) return;
    img.dataset.ddgDone = '1';
    img.style.objectFit = 'contain';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
  }

  function imageFailed(img) {
    if (!img || !img.isConnected) return;
    if (img.dataset.ddgStage === 'direct') {
      switchSafe(img, 'direct-error');
      return;
    }
    if (img.dataset.ddgStage === 'safe') {
      renderFailure(img, 'Nici preview-ul direct, nici fallback-ul sigur nu au putut decoda fișierul.');
    }
  }

  // Kept for API compatibility with TEST92 observers. There is intentionally
  // no watchdog/timer for native images anymore.
  function armImage(img) {
    if (!img || img.dataset.ddgDirectSafe !== '1') return;
    if (img.complete && img.naturalWidth > 0) imageReady(img);
  }

  function localHTML(path) {
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';
    const kind = kindOf(path);
    const direct = directURL(path);
    const ext = extOf(path);

    if (kind === 'image') {
      const p = escAttr(path);
      const lowerExt = ext.toLowerCase();
      const safeFirst = SAFE_FIRST_IMAGE_EXTS.has(lowerExt);
      const stage = safeFirst ? 'safe' : 'direct';
      const src = safeFirst ? safeURL(path) : direct;
      const why = safeFirst ? ' data-ddg-why="format-safe-first"' : '';
      return `<img id="localImage" data-ddg-direct-safe="1" data-ddg-stage="${stage}" data-ddg-path="${p}"${why} src="${src}" alt="Preview local" loading="eager" decoding="async" fetchpriority="high" onload="ddgLocalPreviewDirectSafeV8576.imageReady(this)" onerror="ddgLocalPreviewDirectSafeV8576.imageFailed(this)"><span class="miniInfo">${ext}</span>`;
    }
    if (kind === 'video') {
      return `<video id="localVideo" controls preload="metadata" src="${direct}"></video><span class="miniInfo">${ext} • local</span>`;
    }
    if (kind === 'audio') {
      return `<audio controls preload="metadata" src="${direct}"></audio><span class="miniInfo">${ext}</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Apasă <b>Redă local</b> pentru aplicația Windows/VLC/MPC-HC.</div>';
  }

  function installOwner() {
    if (typeof window.localPreviewHTML === 'function') {
      window.localPreviewHTML = localHTML;
    }
  }

  function observeLocalPreview() {
    const host = document.getElementById('localPreview');
    if (!host || host.dataset.ddgDirectSafeObserver === '1') return;
    host.dataset.ddgDirectSafeObserver = '1';
    const armCurrent = () => {
      const img = host.querySelector('img[data-ddg-direct-safe="1"]');
      if (img) armImage(img);
    };
    new MutationObserver(armCurrent).observe(host, {childList:true, subtree:true});
    armCurrent();
  }

  function boot() {
    installOwner();
    observeLocalPreview();
  }

  window.ddgLocalPreviewDirectSafeV8576 = {
    imageReady,
    imageFailed,
    switchSafe,
    armImage,
    localHTML,
    directURL,
    safeURL
  };

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
})();
