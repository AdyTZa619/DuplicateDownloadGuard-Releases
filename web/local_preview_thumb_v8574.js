// TEST v8.5.74 — thumbnail-first LOCAL preview.
// Established media-browser pattern: show a small cached derivative immediately,
// and do not make the browser parse/decode the full local file until the user
// explicitly presses Play. The backend cache is keyed by path + size + mtime.
(() => {
  'use strict';

  let previewGeneration = 0;

  const IMAGE_EXTS = new Set(['jpg','jpeg','jpe','jfif','png','gif','webp','bmp','avif','heic','heif','tif','tiff']);
  const VIDEO_EXTS = new Set(['mp4','webm','ogv','mov','m4v','mkv','avi','flv','ts','mts','m2ts']);
  const AUDIO_EXTS = new Set(['mp3','wav','ogg','m4a','aac','flac','opus']);

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
    if (IMAGE_EXTS.has(ext)) return 'image';
    if (VIDEO_EXTS.has(ext)) return 'video';
    if (AUDIO_EXTS.has(ext)) return 'audio';
    if (typeof window.previewKind === 'function') return window.previewKind(path);
    return 'other';
  }

  function fullURL(path) {
    return `/api/local-preview?path=${encodeURIComponent(path)}`;
  }

  function thumbURL(path) {
    return `/api/local-thumb?path=${encodeURIComponent(path)}`;
  }

  function generationOf(el) {
    return Number(el?.dataset?.ddgGeneration || 0);
  }

  function isCurrent(el) {
    return Boolean(el && el.isConnected && generationOf(el) === previewGeneration);
  }

  function setLocalType(text, title = '') {
    const el = document.getElementById('localType');
    if (!el) return;
    el.textContent = text;
    el.title = title || '';
  }

  function errorButtons() {
    return '<button class="btn primary" onclick="playLocal()">▶ Local extern</button> <button class="btn" onclick="openLocal()">⌕ Explorer</button>';
  }

  function renderImageError(img, detail) {
    if (!isCurrent(img)) return;
    const box = document.getElementById('localPreview') || img.parentElement;
    if (!box) return;
    const path = String(img.dataset.ddgLocalPath || '').trim();
    box.innerHTML = `<div class="previewEmpty"><b>Imagine locală indisponibilă</b><br><br>${esc(detail)}${path ? `<br><br><span class="miniPath">${esc(path)}</span>` : ''}<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', detail);
  }

  function onImageLoad(img) {
    if (!isCurrent(img)) return;
    const stage = String(img.dataset.ddgStage || 'thumb');
    img.style.display = 'block';
    img.style.width = 'auto';
    img.style.height = 'auto';
    img.style.maxWidth = '100%';
    img.style.maxHeight = '420px';
    img.style.objectFit = 'contain';
    if (stage === 'thumb') {
      setLocalType('IMAGE • CACHE', 'Thumbnail local 1280×960 max, memorat după path + size + mtime. Fișierul original nu este recitit integral la fiecare selectare.');
    } else {
      setLocalType('IMAGE • ORIGINAL', 'Thumbnail-ul nu a fost disponibil; WebView afișează originalul direct.');
    }
  }

  function onImageError(img) {
    if (!isCurrent(img)) return;
    const stage = String(img.dataset.ddgStage || 'thumb');
    if (stage === 'thumb') {
      img.dataset.ddgStage = 'original';
      setLocalType('IMAGE • DIRECT', 'Thumbnail-ul nu a putut fi generat; încerc originalul o singură dată.');
      img.src = fullURL(String(img.dataset.ddgLocalPath || ''));
      return;
    }
    renderImageError(img, 'Nici thumbnail-ul, nici încărcarea directă a originalului nu au putut fi afișate de WebView.');
  }

  function onPosterLoad(img) {
    if (!isCurrent(img)) return;
    setLocalType('VIDEO • POSTER CACHE', 'Afișez un cadru local cache-uit. Video-ul complet nu este deschis până nu apeși Redă.');
  }

  function onPosterError(img) {
    if (!isCurrent(img)) return;
    img.style.display = 'none';
    const shell = img.closest('.ddgVideoShell');
    if (shell) shell.classList.add('noPoster');
    setLocalType('VIDEO • GATA DE REDARE', 'Posterul nu a fost disponibil; fișierul video rămâne neatins până la Play.');
  }

  function activateVideo(shell) {
    if (!shell || generationOf(shell) !== previewGeneration) return;
    const path = String(shell.dataset.ddgLocalPath || '').trim();
    const generation = generationOf(shell);
    if (!path) return;
    shell.innerHTML = `<video id="localVideo" data-ddg-generation="${generation}" data-ddg-local-path="${esc(path)}" controls playsinline autoplay preload="metadata" src="${fullURL(path)}" onloadstart="ddgLocalPreviewThumbV8574.onVideoLoadStart(this)" onloadedmetadata="ddgLocalPreviewThumbV8574.onVideoReady(this)" onplaying="ddgLocalPreviewThumbV8574.onVideoPlaying(this)" onerror="ddgLocalPreviewThumbV8574.onVideoError(this)"></video>`;
    setLocalType('VIDEO • PORNESC', 'Deschid originalul local numai acum, la cererea ta.');
  }

  function onVideoLoadStart(video) {
    if (!isCurrent(video)) return;
    setLocalType('VIDEO • ÎNCARC', 'Browserul citește metadatele/Range din fișierul local la Play.');
  }

  function onVideoReady(video) {
    if (!isCurrent(video)) return;
    setLocalType('VIDEO • READY', 'Metadatele sunt gata.');
  }

  function onVideoPlaying(video) {
    if (!isCurrent(video)) return;
    setLocalType('VIDEO • PLAYING', 'Redare locală activă.');
  }

  function onVideoError(video) {
    if (!isCurrent(video)) return;
    const box = document.getElementById('localPreview') || video.parentElement;
    if (!box) return;
    box.innerHTML = `<div class="previewEmpty"><b>Preview video indisponibil în WebView</b><br><br>Containerul/codecul poate necesita VLC/MPC-HC. Fișierul nu este recodat automat.<br><br>${errorButtons()}</div>`;
    setLocalType('LOCAL • EROARE', 'WebView nu poate reda acest container/codec.');
  }

  function onAudioPlay(audio) {
    if (!isCurrent(audio)) return;
    setLocalType('AUDIO • PLAY', 'Audio-ul local este încărcat la cerere.');
  }

  function localPreviewHTMLV8574(path) {
    previewGeneration += 1;
    const generation = previewGeneration;
    if (!path) return '<div class="previewEmpty">Nu există o copie locală asociată acestui rezultat.</div>';

    const kind = kindOf(path);
    const ext = extOf(path).toUpperCase();
    const safePath = esc(path);

    if (kind === 'image') {
      return `<img id="localImage" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" data-ddg-stage="thumb" src="${thumbURL(path)}" alt="Preview local" onload="ddgLocalPreviewThumbV8574.onImageLoad(this)" onerror="ddgLocalPreviewThumbV8574.onImageError(this)"><span class="miniInfo">${esc(ext)} • local cache</span>`;
    }
    if (kind === 'video') {
      setTimeout(() => {
        if (generation === previewGeneration) setLocalType('VIDEO • POSTER', 'Generez sau refolosesc cadrul cache-uit; originalul video nu este încă deschis.');
      }, 0);
      return `<div class="ddgVideoShell" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}"><img id="localVideoPoster" data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" src="${thumbURL(path)}" alt="Poster local" onload="ddgLocalPreviewThumbV8574.onPosterLoad(this)" onerror="ddgLocalPreviewThumbV8574.onPosterError(this)"><button class="ddgVideoPlay" onclick="ddgLocalPreviewThumbV8574.activateVideo(this.parentElement)">▶ Redă</button><span class="miniInfo">${esc(ext)} • poster local</span></div>`;
    }
    if (kind === 'audio') {
      setTimeout(() => {
        if (generation === previewGeneration) setLocalType('AUDIO • LA CERERE', 'Nu citesc metadatele până când nu apeși Play.');
      }, 0);
      return `<audio data-ddg-generation="${generation}" data-ddg-local-path="${safePath}" controls preload="none" src="${fullURL(path)}" onplay="ddgLocalPreviewThumbV8574.onAudioPlay(this)"></audio><span class="miniInfo">${esc(ext)} • local</span>`;
    }
    return '<div class="previewEmpty">Previzualizarea încorporată nu este disponibilă pentru acest format.<br>Folosește <b>Local extern</b>.</div>';
  }

  function installStyles() {
    if (document.getElementById('ddgLocalPreviewThumbV8574Style')) return;
    const style = document.createElement('style');
    style.id = 'ddgLocalPreviewThumbV8574Style';
    style.textContent = `
      #localPreview .ddgVideoShell{position:relative;width:100%;min-height:260px;display:flex;align-items:center;justify-content:center;background:#05090d;border-radius:7px;overflow:hidden}
      #localPreview .ddgVideoShell>img{display:block;max-width:100%;max-height:420px;object-fit:contain;background:#000}
      #localPreview .ddgVideoShell.noPoster:before{content:'VIDEO';font-size:34px;font-weight:800;color:#33465b;letter-spacing:.14em}
      #localPreview .ddgVideoPlay{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);border:1px solid rgba(255,255,255,.28);background:rgba(8,14,22,.82);color:#fff;border-radius:999px;padding:13px 20px;font-weight:800;cursor:pointer;backdrop-filter:blur(6px)}
      #localPreview .ddgVideoPlay:hover{background:rgba(28,49,72,.94)}
      #localPreview .miniPath{display:inline-block;max-width:100%;font-family:Consolas,monospace;font-size:11px;word-break:break-all}
    `;
    document.head.appendChild(style);
  }

  function install() {
    installStyles();
    window.localPreviewHTML = localPreviewHTMLV8574;
  }

  window.ddgLocalPreviewThumbV8574 = {
    onImageLoad,
    onImageError,
    onPosterLoad,
    onPosterError,
    activateVideo,
    onVideoLoadStart,
    onVideoReady,
    onVideoPlaying,
    onVideoError,
    onAudioPlay,
    localPreviewHTML: localPreviewHTMLV8574
  };

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
