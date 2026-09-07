// TEST v8.5.70 — resilient LOCAL preview pipeline.
// Keeps video/audio streaming on the existing Range-capable endpoint, while
// images get a second, browser-safe Blob attempt if the direct <img> request
// cannot be rendered. No HDD rescan and no remote traffic are triggered here.
(() => {
  'use strict';

  let activeBlobURL = '';

  const MIME = {
    jpg: 'image/jpeg', jpeg: 'image/jpeg', jpe: 'image/jpeg',
    png: 'image/png', gif: 'image/gif', webp: 'image/webp',
    bmp: 'image/bmp', avif: 'image/avif'
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
    if (typeof window.previewKind === 'function') return window.previewKind(path);
    const ext = extOf(path);
    if (['jpg','jpeg','png','gif','webp','bmp','avif'].includes(ext)) return 'image';
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

  function setLocalType(text, title = '') {
    const el = document.getElementById('localType');
    if (!el) return;
    el.textContent = text;
    if (title) el.title = title;
  }

  function errorButtons() {
    return '<button class="btn primary" onclick="playLocal()">▶ Local extern</button> <button class="btn" onclick="openLocal()">⌕ Explorer</button>';
  }

  function renderError(element, title, detail) {
    const box = document.getElementById('localPreview') || element?.parentElement;
    if (!box) return;
    const path = String(element?.dataset?.ddgLocalPath || '').trim();
    box.innerHTML = `<div class="previewEmpty"><b>${esc(title)}</b><br><br>${esc(detail)}${path ? `<br><br><span class="miniPath">${esc(path)}</span>` : ''}<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', detail);
  }

  async function blobRetry(img) {
    if (!img || img.dataset.ddgLocalRetry === '1') return;
    const path = String(img.dataset.ddgLocalPath || '').trim();
    if (!path) return renderError(img, 'Preview local indisponibil', 'Calea fișierului local lipsește.');

    img.dataset.ddgLocalRetry = '1';
    img.dataset.ddgLocalStage = 'blob-fetch';
    setLocalType('IMAGE • RETRY', 'Reîncarc imaginea locală prin fetch + Blob, fără trafic extern.');

    try {
      const response = await fetch(directURL(path), {cache: 'no-store', headers: {'Accept':'image/*,*/*;q=0.8'}});
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const bytes = await response.arrayBuffer();
      if (!bytes.byteLength) throw new Error('fișier gol');

      const ext = extOf(path);
      const headerType = String(response.headers.get('Content-Type') || '').split(';')[0].trim();
      const mime = MIME[ext] || (headerType.startsWith('image/') ? headerType : 'application/octet-stream');
      releaseBlob();
      activeBlobURL = URL.createObjectURL(new Blob([bytes], {type: mime}));
      img.dataset.ddgLocalStage = 'blob';
      img.src = activeBlobURL;
    } catch (error) {
      renderError(img, 'Imagine locală indisponibilă', `DDG nu a putut citi copia locală pentru preview: ${error?.message || String(error)}`);
    }
  }

  function onImageLoad(img) {
    if (!img) return;
    img.style.display = 'block';
    img.style.width = 'auto';
    img.style.height = 'auto';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
    img.style.objectFit = 'contain';

    const stage = String(img.dataset.ddgLocalStage || 'direct');
    setLocalType(stage === 'blob' ? 'IMAGE • FALLBACK OK' : 'IMAGE', stage === 'blob' ? 'Preview local încărcat prin fallback Blob.' : 'Preview local direct.');

    // A decoded image with a zero-sized rendered box is not useful. This is a
    // different failure mode from img.onerror and has appeared in WebView.
    requestAnimationFrame(() => {
      const rect = img.getBoundingClientRect();
      if (img.naturalWidth > 0 && img.naturalHeight > 0 && (rect.width < 2 || rect.height < 2) && stage !== 'blob') {
        void blobRetry(img);
      }
    });
  }

  function onImageError(img) {
    const stage = String(img?.dataset?.ddgLocalStage || 'direct');
    if (stage === 'direct') return void blobRetry(img);
    renderError(img, 'Imagine locală nerecunoscută de player', 'Fișierul există, dar nici încărcarea directă, nici fallback-ul Blob nu au putut fi decodate în preview. Deschide copia în aplicația externă.');
  }

  function onMediaError(media) {
    const path = String(media?.dataset?.ddgLocalPath || '').trim();
    const ext = extOf(path).toUpperCase() || 'MEDIA';
    renderError(media, `Preview local ${ext} indisponibil`, 'Fișierul local poate folosi un container/codec pe care WebView nu îl redă. DDG nu descarcă și nu recodează automat fișierul; folosește playerul extern.');
  }

  function robustLocalPreviewHTML(path) {
    releaseBlob();
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';

    const kind = kindOf(path);
    const ext = extOf(path).toUpperCase();
    const safePath = esc(path);
    const url = directURL(path);

    if (kind === 'image') {
      return `<img id="localImage" data-ddg-local-path="${safePath}" data-ddg-local-stage="direct" src="${url}" alt="Preview local" style="display:block;width:auto;height:auto;max-width:100%;max-height:420px;object-fit:contain" onload="ddgLocalPreviewV8570.onImageLoad(this)" onerror="ddgLocalPreviewV8570.onImageError(this)"><span class="miniInfo">${esc(ext)}</span>`;
    }
    if (kind === 'video') {
      return `<video id="localVideo" data-ddg-local-path="${safePath}" controls preload="metadata" src="${url}" onerror="ddgLocalPreviewV8570.onMediaError(this)"></video><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    if (kind === 'audio') {
      return `<audio data-ddg-local-path="${safePath}" controls preload="metadata" src="${url}" onerror="ddgLocalPreviewV8570.onMediaError(this)"></audio><span class="miniInfo">${esc(ext)}</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Apasă <b>Local extern</b> pentru aplicația Windows/VLC/MPC-HC.</div>';
  }

  function installStyles() {
    if (document.getElementById('ddgLocalPreviewV8570Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgLocalPreviewV8570Style';
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

  window.ddgLocalPreviewV8570 = {
    onImageLoad,
    onImageError,
    onMediaError,
    blobRetry,
    localPreviewHTML: robustLocalPreviewHTML
  };

  window.addEventListener('beforeunload', releaseBlob, {once:true});
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
