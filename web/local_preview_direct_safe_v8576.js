(() => {
  'use strict';

  // TEST92: keep the original fast local preview path. Only images that really
  // fail (or stay unresolved for several seconds) are switched once to the
  // existing safe JPEG derivative endpoint. Video/audio are never intercepted.
  const IMAGE_WATCHDOG_MS = 4000;
  const armed = new WeakMap();

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

  function clearArm(img) {
    const timer = armed.get(img);
    if (timer) clearTimeout(timer);
    armed.delete(img);
  }

  function renderFailure(img, message) {
    clearArm(img);
    if (!img || !img.isConnected) return;
    const host = img.parentElement;
    if (!host) return;
    host.innerHTML = `<div class="previewEmpty"><b>Imagine locală indisponibilă.</b><br>${message}<br><br>Poți folosi <b>Redă local</b> / aplicația Windows.</div>`;
  }

  function switchSafe(img, why) {
    if (!img || !img.isConnected) return;
    if (img.dataset.ddgStage !== 'direct') return;

    clearArm(img);
    img.dataset.ddgStage = 'switching';
    const path = img.dataset.ddgPath || '';
    if (!path) return renderFailure(img, 'Calea locală lipsește.');

    // Stop the old request before starting the fallback. This is important for
    // HDDs: there must never be two full reads racing for the same image.
    img.removeAttribute('src');
    img.dataset.ddgStage = 'safe';
    img.dataset.ddgWhy = why || 'fallback';
    img.src = safeURL(path);
  }

  function imageReady(img) {
    if (!img) return;
    clearArm(img);
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

  function armImage(img) {
    if (!img || img.dataset.ddgDirectSafe !== '1' || armed.has(img)) return;
    if (img.complete && img.naturalWidth > 0) {
      imageReady(img);
      return;
    }
    const timer = setTimeout(() => {
      armed.delete(img);
      if (!img.isConnected || img.dataset.ddgDone === '1') return;
      if (img.dataset.ddgStage === 'direct' && (!img.complete || img.naturalWidth <= 0)) {
        switchSafe(img, 'direct-watchdog');
      }
    }, IMAGE_WATCHDOG_MS);
    armed.set(img, timer);
  }

  function localHTML(path) {
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';
    const kind = kindOf(path);
    const direct = directURL(path);
    const ext = extOf(path);

    if (kind === 'image') {
      const p = escAttr(path);
      // Intentionally matches the pre-TEST85 fast path: one plain <img> request.
      return `<img id="localImage" data-ddg-direct-safe="1" data-ddg-stage="direct" data-ddg-path="${p}" src="${direct}" alt="Preview local" onload="ddgLocalPreviewDirectSafeV8576.imageReady(this)" onerror="ddgLocalPreviewDirectSafeV8576.imageFailed(this)"><span class="miniInfo">${ext}</span>`;
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
