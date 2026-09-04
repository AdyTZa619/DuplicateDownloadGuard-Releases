(() => {
  'use strict';

  let row = null;
  const meta = { remote: {}, local: {} };
  let trueFallbackIDV858 = 0;
  let trueFallbackAtV858 = 0;
  let appClockTimerV8512 = 0;

  const fmtDuration = sec => {
    sec = Number(sec);
    if (!Number.isFinite(sec) || sec <= 0) return '';
    const s = Math.round(sec);
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
    return h ? `${h}:${String(m).padStart(2,'0')}:${String(r).padStart(2,'0')}` : `${m}:${String(r).padStart(2,'0')}`;
  };

  function verdict(r) {
    if (!r) return '';
    const method = String(r.guardMethod || '');
    if (method === 'media-same-content') return 'ACELAȘI CONȚINUT';
    if (method === 'media-version') return 'ALTĂ VERSIUNE';
    if (method === 'media-looks-same') return 'PARE ACELAȘI';
    if (method === 'download-history') return 'DESCĂRCAT DEJA';
    if (method === 'download-history-source') return 'DESCĂRCAT ÎNAINTE';
    const s = r.status || r.autoStatus || '';
    return ({VERIFIED:'VERIFICAT',SAMPLED:'MOSTRE OK',HAVE:'AI DEJA',POSSIBLE:'POSIBIL',DIFFERENT:'DIFERIT',MISSING:'NU ÎL AI'})[s] || s;
  }

  function scoreText(r) {
    if (!r) return '';
    const visual = Number(r.visualScore || 0);
    if (visual > 0) return `${visual}% similar`;
    const score = Number(r.matchScore || 0);
    return score > 0 ? `scor ${score}/100` : '';
  }

  function tickAppClockV8512() {
    const el = document.getElementById('appClockV8512');
    if (!el) return;
    const now = new Date();
    const hh = String(now.getHours()).padStart(2, '0');
    const mm = String(now.getMinutes()).padStart(2, '0');
    const ss = String(now.getSeconds()).padStart(2, '0');
    el.textContent = `${hh}:${mm}:${ss}`;
    el.title = now.toLocaleString('ro-RO');
  }

  function install() {
    if (!document.getElementById('previewQuickV86Styles')) {
      const style = document.createElement('style');
      style.id = 'previewQuickV86Styles';
      style.textContent = `
        .previewHead .previewQuickV86{flex:1;min-width:0;text-align:center;color:#d7e7f8;font-size:11px;font-weight:750;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;padding:0 8px}
        .previewHead .previewQuickV86.good{color:#8ee6bd}.previewHead .previewQuickV86.warn{color:#ffd979}.previewHead .previewQuickV86.bad{color:#ff9aa5}
        #appClockV8512{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);margin:0;padding:8px 18px;border:1px solid #3a536b;border-radius:10px;background:#0d151e;color:#c8e5ff;font-family:Consolas,monospace;font-size:20px;line-height:1;font-weight:900;letter-spacing:.08em;white-space:nowrap;box-shadow:0 4px 18px rgba(0,0,0,.25)}
        @media(max-width:760px){.previewHead{flex-wrap:wrap;gap:6px}.previewHead .previewQuickV86{order:3;flex-basis:100%;text-align:left;padding:0}#appClockV8512{font-size:16px;padding:6px 11px;letter-spacing:.05em}}
      `;
      document.head.appendChild(style);
    }
    const remoteHead = document.getElementById('remotePreview')?.closest('.previewCard')?.querySelector('.previewHead');
    const localHead = document.getElementById('localPreview')?.closest('.previewCard')?.querySelector('.previewHead');
    if (remoteHead && !document.getElementById('remoteQuickV86')) remoteHead.querySelector('b')?.insertAdjacentHTML('afterend','<span class="previewQuickV86" id="remoteQuickV86">—</span>');
    if (localHead && !document.getElementById('localQuickV86')) localHead.querySelector('b')?.insertAdjacentHTML('afterend','<span class="previewQuickV86" id="localQuickV86">—</span>');
    const top = document.querySelector('.top');
    if (top && !document.getElementById('appClockV8512')) {
      top.insertAdjacentHTML('beforeend','<div id="appClockV8512">--:--:--</div>');
      tickAppClockV8512();
      if (appClockTimerV8512) clearInterval(appClockTimerV8512);
      appClockTimerV8512 = setInterval(tickAppClockV8512, 250);
    }
  }

  function fixDownloadHelpV857() {
    const help = document.getElementById('help-download');
    if (!help) return;
    for (const tr of help.querySelectorAll('tbody tr')) {
      const cells = tr.querySelectorAll('td');
      if (cells.length < 2) continue;
      if (cells[0].textContent.trim() === 'HTTP/CDN direct' && cells[1].textContent.trim() === 'aria2') {
        cells[1].textContent = 'Downloader intern (Auto); aria2 opțional';
      }
    }
  }

  // v8.5.18: never call media.load() while tearing down the previous player.
  // Chromium may synchronously wait for the old media pipeline/network request to
  // settle, which can block the UI before the next preview request is even sent.
  // Detaching the element is enough: the browser cancels its stream asynchronously.
  function releaseRemoteMediaV8512() {
    const preview = document.getElementById('remotePreview');
    if (!preview) return;
    for (const media of preview.querySelectorAll('video,audio')) {
      try { media.onerror = null; } catch (_) {}
      try { media.removeAttribute('onerror'); } catch (_) {}
      try { media.pause(); } catch (_) {}
      try { media.remove(); } catch (_) {}
    }
    for (const img of preview.querySelectorAll('img')) {
      try { img.onerror = null; } catch (_) {}
      try { img.removeAttribute('onerror'); } catch (_) {}
      try { img.remove(); } catch (_) {}
    }
  }
  window.releaseRemoteMediaV8512 = releaseRemoteMediaV8512;

  // v8.5.17: "Stop stream" is a browser stop, not a MEGAcmd teardown. Removing
  // the media element stops the transfer while keeping the MEGA session and
  // WebDAV endpoint warm for the next row. The old backend /stop could restore
  // sessions for tens of seconds and the intentional media abort could also fire
  // remotePreviewError(), creating a false per-file fallback behind the next click.
  function markIntentionalRemoteAbortV8517() {
    const current = typeof currentRow !== 'undefined' ? currentRow : null;
    const id = Number(current?.id || 0);
    if (id) {
      trueFallbackIDV858 = id;
      trueFallbackAtV858 = Date.now();
    }
    releaseRemoteMediaV8512();
  }

  function installSoftRemoteStopV8517() {
    const original = window.stopRemotePreview;
    if (typeof original !== 'function' || original.__softStopV8517) return;
    const wrapped = async function () {
      markIntentionalRemoteAbortV8517();
      try { remotePreviewActive = false; } catch (_) {}
      const stop = document.getElementById('stopRemote');
      if (stop) stop.disabled = true;
      const sourceEl = document.getElementById('remoteSource');
      if (sourceEl) {
        sourceEl.classList.remove('remoteLive');
        const current = typeof currentRow !== 'undefined' ? currentRow : null;
        sourceEl.textContent = current?.remote?.source || 'REMOTE';
      }
      const preview = document.getElementById('remotePreview');
      if (preview) preview.innerHTML = '<div class="previewEmpty">Preview remote oprit. Sesiunea MEGA rămâne pregătită; selectează din nou rândul pentru pornire rapidă.</div>';
    };
    wrapped.__softStopV8517 = true;
    window.stopRemotePreview = wrapped;
  }

  function installSoftClearResultsV8517() {
    const original = window.clearResults;
    if (typeof original !== 'function' || original.__softClearV8517) return;
    const wrapped = async function () {
      if (!confirm('Golești rezultatele curente?')) return;
      markIntentionalRemoteAbortV8517();
      try { remotePreviewActive = false; } catch (_) {}
      await api('/api/results/clear', {method:'POST'});
      try { currentRow = null; } catch (_) {}
      const remote = document.getElementById('remotePreview');
      const local = document.getElementById('localPreview');
      if (remote) remote.innerHTML = '<div class="previewEmpty">Selectează un rezultat.</div>';
      if (local) local.innerHTML = '<div class="previewEmpty">Selectează un rezultat.</div>';
      if (typeof loadResults === 'function') loadResults();
    };
    wrapped.__softClearV8517 = true;
    window.clearResults = wrapped;
  }

  // One browser-level retry is allowed for a root-derived stream. After this
  // patch a cold resume normally promotes to the whole-folder root too, even
  // though the older API label still says MEGA DIRECT RESUME.
  function wrapMegaRootFailureV858() {
    const original = window.remotePreviewError;
    if (typeof original !== 'function' || original.__megaTrueFallbackV858) return;
    const wrapped = async function () {
      const current = typeof currentRow !== 'undefined' ? currentRow : null;
      const id = Number(current?.id || 0);
      const sourceEl = document.getElementById('remoteSource');
      const sourceText = String(sourceEl?.textContent || '').toUpperCase();
      const rootMode = sourceText.includes('MEGA FAST ROOT') || sourceText.includes('MEGA FAST RESUME') || sourceText.includes('MEGA DIRECT RESUME');
      const now = Date.now();
      const recentlyRetried = id && trueFallbackIDV858 === id && now - trueFallbackAtV858 < 15000;
      if (!id || !rootMode || recentlyRetried) return original.apply(this, arguments);

      trueFallbackIDV858 = id;
      trueFallbackAtV858 = now;
      releaseRemoteMediaV8512();
      const preview = document.getElementById('remotePreview');
      if (preview) preview.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Streamul rapid nu a fost acceptat; trec pe fallback MEGA per-fișier…</b><span class="small">Root-ul rămâne activ pentru următorul fișier.</span></div>';
      try {
        const d = await api('/api/remote-preview/start', {
          method: 'POST',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify({id, forceFallback:true})
        });
        if (!currentRow || Number(currentRow.id) !== id) return;
        const kind = d.kind || previewKind(current.remote?.name || current.remote?.path || '');
        remotePreviewActive = true;
        const stop = document.getElementById('stopRemote');
        if (stop) stop.disabled = false;
        if (sourceEl) {
          sourceEl.textContent = `${d.source || 'MEGA TRUE FALLBACK'}${Number.isFinite(Number(d.prepareMs)) ? ` • ${Number(d.prepareMs)} ms` : ''} • LIVE`;
          sourceEl.classList.add('remoteLive');
        }
        if (preview) preview.innerHTML = remoteMediaHTML(d.url, kind, current.remote?.name || current.remote?.path || 'remote');
        return;
      } catch (_) {
        // exact_guard.js contains an older fallback wrapper. Change the source
        // marker before delegating so that wrapper cannot issue a second
        // forceFallback request for the same browser error.
        if (sourceEl) sourceEl.textContent = 'MEGA TRUE FALLBACK • EROARE';
        return original.apply(this, arguments);
      }
    };
    wrapped.__megaTrueFallbackV858 = true;
    window.remotePreviewError = wrapped;
  }

  function readElement(which) {
    const body = document.getElementById(which === 'remote' ? 'remotePreview' : 'localPreview');
    if (!body) return;
    const video = body.querySelector('video');
    const audio = body.querySelector('audio');
    const img = body.querySelector('img');
    const m = meta[which];
    if (video) {
      if (Number.isFinite(video.duration) && video.duration > 0) m.duration = video.duration;
      if (video.videoWidth > 0 && video.videoHeight > 0) { m.width = video.videoWidth; m.height = video.videoHeight; }
      if (!video.dataset.quickV86) {
        video.dataset.quickV86 = '1';
        video.addEventListener('loadedmetadata', () => { readElement(which); render(); });
        video.addEventListener('durationchange', () => { readElement(which); render(); });
      }
    } else if (audio) {
      if (Number.isFinite(audio.duration) && audio.duration > 0) m.duration = audio.duration;
      if (!audio.dataset.quickV86) {
        audio.dataset.quickV86 = '1';
        audio.addEventListener('loadedmetadata', () => { readElement(which); render(); });
        audio.addEventListener('durationchange', () => { readElement(which); render(); });
      }
    } else if (img) {
      if (img.naturalWidth > 0 && img.naturalHeight > 0) { m.width = img.naturalWidth; m.height = img.naturalHeight; }
      if (!img.dataset.quickV86) {
        img.dataset.quickV86 = '1';
        img.addEventListener('load', () => { readElement(which); render(); });
      }
    }
  }

  function tone() {
    const v = verdict(row);
    if (['VERIFICAT','AI DEJA','ACELAȘI CONȚINUT','DESCĂRCAT DEJA'].includes(v)) return 'good';
    if (['DIFERIT','NU ÎL AI'].includes(v)) return 'bad';
    return 'warn';
  }

  function parts(which) {
    const m = meta[which] || {};
    const p = [];
    const d = fmtDuration(m.duration);
    if (d) p.push(d);
    if (m.width && m.height) p.push(`${m.width}×${m.height}`);
    const score = scoreText(row);
    if (score) p.push(score);
    const v = verdict(row);
    if (v) p.push(v);
    return p.length ? p : ['aștept metadatele…'];
  }

  function render() {
    readElement('remote');
    readElement('local');
    for (const which of ['remote','local']) {
      const el = document.getElementById(which === 'remote' ? 'remoteQuickV86' : 'localQuickV86');
      if (!el) continue;
      el.className = `previewQuickV86 ${tone()}`;
      el.textContent = parts(which).join(' • ');
      el.title = el.textContent;
    }
  }

  function reset(r) {
    row = r || null;
    meta.remote = {};
    meta.local = {};
    render();
  }

  function wrapShowDetail() {
    const original = window.showDetail;
    if (typeof original !== 'function' || original.__previewQuickV86) return;
    const wrapped = async function(r) {
      // A new intentional selection is allowed to use one real fallback again;
      // the stop marker above only suppresses the abort caused by stopping.
      trueFallbackIDV858 = 0;
      trueFallbackAtV858 = 0;
      // Detach the previous player without forcing HTMLMediaElement.load().
      // The old load() call could synchronously stall Chromium before the new
      // /api/remote-preview/start request was sent.
      releaseRemoteMediaV8512();
      reset(r);
      const out = await original.apply(this, arguments);
      setTimeout(render, 0);
      return out;
    };
    wrapped.__previewQuickV86 = true;
    window.showDetail = wrapped;
  }

  function observe() {
    for (const id of ['remotePreview','localPreview']) {
      const el = document.getElementById(id);
      if (!el || el.dataset.quickObserverV86) continue;
      el.dataset.quickObserverV86 = '1';
      new MutationObserver(() => setTimeout(render, 0)).observe(el, {childList:true, subtree:true});
    }
  }

  function boot() {
    install();
    fixDownloadHelpV857();
    wrapMegaRootFailureV858();
    wrapShowDetail();
    installSoftRemoteStopV8517();
    installSoftClearResultsV8517();
    observe();
    render();
    window.addEventListener('pagehide', releaseRemoteMediaV8512, {capture:true});
    // No MEGA startup prewarm. A user click must never wait behind unsolicited
    // background login/WebDAV work. Fresh scans may still leave their root warm.
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
})();