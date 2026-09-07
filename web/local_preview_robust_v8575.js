// TEST v8.5.75 — targeted LOCAL preview repair.
// Keep the proven direct Range-capable path for video/audio and normal images.
// For images only, fall back once to the browser-safe cached derivative endpoint.
// No blob full-file fetches, no stacked preview owners, no forced video poster UI.
(() => {
  'use strict';

  const IMAGE_FALLBACK_MS = 2500;
  const IMAGE_EXTS = new Set(['jpg','jpeg','jpe','jfif','png','gif','webp','bmp','avif','heic','heif','tif','tiff']);
  const VIDEO_EXTS = new Set(['mp4','webm','ogv','mov','m4v','mkv','avi','flv','ts','mts','m2ts']);
  const AUDIO_EXTS = new Set(['mp3','wav','ogg','m4a','aac','flac','opus']);

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
    if (IMAGE_EXTS.has(ext)) return 'image';
    if (VIDEO_EXTS.has(ext)) return 'video';
    if (AUDIO_EXTS.has(ext)) return 'audio';
    if (typeof window.previewKind === 'function') return window.previewKind(path);
    return 'other';
  }

  function directURL(path) {
    return `/api/local-preview?path=${encodeURIComponent(path)}`;
  }

  function safeImageURL(path) {
    return `/api/local-thumb?path=${encodeURIComponent(path)}`;
  }

  function isLive(el) {
    const box = document.getElementById('localPreview');
    return Boolean(el && el.isConnected && box && box.contains(el));
  }

  function setLocalType(text, title = '') {
    const el = document.getElementById('localType');
    if (!el) return;
    el.textContent = text;
    el.title = title || '';
  }

  function clearFallbackTimer(img) {
    const timer = Number(img?.dataset?.ddgFallbackTimer || 0);
    if (timer) clearTimeout(timer);
    if (img?.dataset) delete img.dataset.ddgFallbackTimer;
  }

  function errorButtons() {
    return '<button class="btn primary" onclick="playLocal()">▶ Local extern</button> <button class="btn" onclick="openLocal()">⌕ Explorer</button>';
  }

  function renderImageError(img) {
    if (!isLive(img)) return;
    clearFallbackTimer(img);
    const path = String(img.dataset.ddgLocalPath || '').trim();
    const box = document.getElementById('localPreview');
    if (!box) return;
    box.innerHTML = `<div class="previewEmpty"><b>Imagine locală indisponibilă</b><br><br>Am încercat o singură dată originalul și o singură dată varianta browser-safe. Fișierul poate fi corupt sau într-un format fără decoder disponibil.${path ? `<br><br><span class="miniPath">${esc(path)}</span>` : ''}<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', 'Original + fallback browser-safe au eșuat.');
  }

  function switchToSafeImage(img, reason) {
    if (!isLive(img)) return;
    if (String(img.dataset.ddgStage || 'direct') !== 'direct') return;
    clearFallbackTimer(img);
    img.dataset.ddgStage = 'safe';
    const path = String(img.dataset.ddgLocalPath || '').trim();
    if (!path) return renderImageError(img);
    setLocalType('IMAGE • SAFE', reason || 'Folosesc derivata locală browser-safe.');
    img.src = safeImageURL(path);
  }

  function armImageFallback(img) {
    if (!isLive(img)) return;
    clearFallbackTimer(img);
    const timer = setTimeout(() => {
      if (!isLive(img)) return;
      if (img.complete && img.naturalWidth > 0) return onImageLoad(img);
      switchToSafeImage(img, 'Originalul nu a devenit afișabil în 2,5 secunde; trec la derivata locală fără blob/full-file fetch.');
    }, IMAGE_FALLBACK_MS);
    img.dataset.ddgFallbackTimer = String(timer);
  }

  function onImageLoad(img) {
    if (!isLive(img)) return;
    clearFallbackTimer(img);
    img.style.display = 'block';
    img.style.width = 'auto';
    img.style.height = 'auto';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
    img.style.objectFit = 'contain';
    const safe = String(img.dataset.ddgStage || 'direct') === 'safe';
    setLocalType(safe ? 'IMAGE • SAFE OK' : 'IMAGE • DIRECT OK', safe ? 'Preview browser-safe din cache local.' : 'Original local afișat direct.');
  }

  function onImageError(img) {
    if (!isLive(img)) return;
    if (String(img.dataset.ddgStage || 'direct') === 'direct') {
      switchToSafeImage(img, 'WebView nu a acceptat originalul; încerc derivata browser-safe.');
      return;
    }
    renderImageError(img);
  }

  function onMediaReady(media, label) {
    if (!isLive(media)) return;
    setLocalType(`${label} • READY`, 'Metadatele locale au fost încărcate direct prin endpoint-ul Range-capable.');
  }

  function onMediaError(media) {
    if (!isLive(media)) return;
    const box = document.getElementById('localPreview');
    if (!box) return;
    box.innerHTML = `<div class="previewEmpty"><b>Preview media indisponibil în WebView</b><br><br>Containerul/codecul poate necesita VLC/MPC-HC.<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', 'WebView nu poate reda acest container/codec.');
  }

  function localPreviewHTMLV8575(path) {
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';

    const kind = kindOf(path);
    const ext = extOf(path).toUpperCase();
    const safePath = esc(path);
    const direct = directURL(path);

    if (kind === 'image') {
      setTimeout(() => {
        const img = document.getElementById('localImage');
        if (img && String(img.dataset.ddgLocalPath || '') === path) armImageFallback(img);
      }, 0);
      return `<img id="localImage" data-ddg-local-path="${safePath}" data-ddg-stage="direct" src="${direct}" alt="Preview local" style="display:block;width:auto;height:auto;max-width:100%;max-height:420px;object-fit:contain" onload="ddgLocalPreviewRobustV8575.onImageLoad(this)" onerror="ddgLocalPreviewRobustV8575.onImageError(this)"><span class="miniInfo">${esc(ext)}</span>`;
    }
    if (kind === 'video') {
      return `<video id="localVideo" data-ddg-local-path="${safePath}" controls playsinline preload="metadata" src="${direct}" onloadedmetadata="ddgLocalPreviewRobustV8575.onMediaReady(this,'VIDEO')" onerror="ddgLocalPreviewRobustV8575.onMediaError(this)"></video><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    if (kind === 'audio') {
      return `<audio data-ddg-local-path="${safePath}" controls preload="metadata" src="${direct}" onloadedmetadata="ddgLocalPreviewRobustV8575.onMediaReady(this,'AUDIO')" onerror="ddgLocalPreviewRobustV8575.onMediaError(this)"></audio><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Folosește <b>Local extern</b>.</div>';
  }

  function installStyles() {
    if (document.getElementById('ddgLocalPreviewRobustV8575Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgLocalPreviewRobustV8575Style';
    style.textContent = '#localPreview .miniPath{display:inline-block;max-width:100%;font-family:Consolas,monospace;font-size:11px;word-break:break-all}';
    document.head.appendChild(style);
  }

  function install() {
    installStyles();
    window.localPreviewHTML = localPreviewHTMLV8575;
  }

  window.ddgLocalPreviewRobustV8575 = {
    onImageLoad,
    onImageError,
    onMediaReady,
    onMediaError,
    localPreviewHTML: localPreviewHTMLV8575
  };

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
