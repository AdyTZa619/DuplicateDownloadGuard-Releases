(() => {
  'use strict';

  let installAttempts = 0;
  const states = new WeakMap();
  const WATCHDOG_MS = 1000;
  const PROBE_AFTER_MS = 8000;
  const RETRY_AFTER_MS = 12000;

  function currentSource() {
    try {
      const value = String(currentRow?.remote?.source || 'REMOTE').trim().toUpperCase();
      return value || 'REMOTE';
    } catch (_) {
      return 'REMOTE';
    }
  }

  function esc(value) {
    if (typeof window.esc === 'function') return window.esc(value);
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  function installStyle() {
    if (document.getElementById('providerBufferV8561Style')) return;
    const style = document.createElement('style');
    style.id = 'providerBufferV8561Style';
    style.textContent = `
      .providerBufferStatus{position:absolute;left:14px;top:14px;z-index:4;max-width:calc(100% - 28px);padding:6px 9px;border-radius:8px;background:rgba(6,12,18,.82);border:1px solid #31485e;color:#c9d9e8;font-size:11px;font-weight:700;pointer-events:none;backdrop-filter:blur(4px)}
      .providerBufferStatus[data-state="waiting"],.providerBufferStatus[data-state="stalled"],.providerBufferStatus[data-state="probing"],.providerBufferStatus[data-state="slow"]{border-color:#927326;color:#ffe08a}
      .providerBufferStatus[data-state="playing"],.providerBufferStatus[data-state="ready"]{border-color:#286a50;color:#8af0c1}
      .providerBufferStatus[data-state="problem"]{border-color:#8b4050;color:#ff9fb0}
    `;
    document.head.appendChild(style);
  }

  function stateFor(media) {
    let state = states.get(media);
    if (!state) {
      state = {
        startedAt: Date.now(),
        playStartedAt: 0,
        lastProgressAt: Date.now(),
        lastBufferedEnd: 0,
        lastCurrentTime: Number(media?.currentTime || 0),
        playRequested: false,
        everPlayable: false,
        everPlayed: false,
        probeStarted: false,
        probeDone: false,
        rangeSupported: null,
        probeStatus: 0,
        probeLatencyMs: 0,
        retryDone: false,
        probeMessage: ''
      };
      states.set(media, state);
    }
    return state;
  }

  function bufferedAhead(media) {
    if (!media || !media.buffered || !media.buffered.length) return 0;
    const now = Number(media.currentTime || 0);
    let best = 0;
    try {
      for (let i = 0; i < media.buffered.length; i++) {
        const start = media.buffered.start(i);
        const end = media.buffered.end(i);
        if (now >= start - 0.05 && now <= end + 0.05) best = Math.max(best, end - now);
      }
    } catch (_) {}
    return Math.max(0, best);
  }

  function bufferedEnd(media) {
    let end = 0;
    try {
      for (let i = 0; i < (media?.buffered?.length || 0); i++) end = Math.max(end, media.buffered.end(i));
    } catch (_) {}
    return end;
  }

  function bufferedPercent(media) {
    const duration = Number(media?.duration || 0);
    if (!Number.isFinite(duration) || duration <= 0 || !media?.buffered?.length) return 0;
    return Math.max(0, Math.min(100, bufferedEnd(media) / duration * 100));
  }

  function statusText(media, stateName) {
    const source = currentSource();
    const ahead = bufferedAhead(media);
    const pct = bufferedPercent(media);
    const aheadText = ahead >= 0.1 ? `${ahead.toFixed(ahead >= 10 ? 0 : 1)}s buffer înainte` : 'buffer aproape gol';
    const pctText = pct >= 1 ? ` • ${pct.toFixed(0)}% încărcat` : '';

    switch (stateName) {
      case 'loadstart': return `${source} • pregătesc bufferul…`;
      case 'loadedmetadata': return `${source} • metadata gata • ${aheadText}${pctText}`;
      case 'progress': return `${source} • ${aheadText}${pctText}`;
      case 'play': return `${source} • pornesc redarea • ${aheadText}`;
      case 'waiting': return `${source} • BUFFERING… • ${aheadText}`;
      case 'stalled': return `${source} • server lent / aștept date… • ${aheadText}`;
      case 'seeking': return `${source} • caut poziția cerută…`;
      case 'canplay': return `${source} • gata de redare • ${aheadText}`;
      case 'playing': return `${source} • redare • ${aheadText}`;
      case 'pause': return `${source} • pauză • ${aheadText}`;
      case 'ended': return `${source} • redare terminată`;
      default: return `${source} • ${aheadText}${pctText}`;
    }
  }

  function setStatus(media, stateName, text) {
    const status = document.getElementById('providerBufferStatus');
    if (!status || !media) return;
    status.dataset.state = stateName || '';
    status.textContent = text || statusText(media, stateName || 'progress');
  }

  function noteProgress(media) {
    const state = stateFor(media);
    const end = bufferedEnd(media);
    const now = Number(media.currentTime || 0);
    if (end > state.lastBufferedEnd + 0.05 || Math.abs(now - state.lastCurrentTime) > 0.05) {
      state.lastBufferedEnd = Math.max(state.lastBufferedEnd, end);
      state.lastCurrentTime = now;
      state.lastProgressAt = Date.now();
    }
    if (now > 0.05) state.everPlayed = true;
    if (media.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA) state.everPlayable = true;
  }

  function resetProbeState(state) {
    state.probeStarted = false;
    state.probeDone = false;
    state.rangeSupported = null;
    state.probeStatus = 0;
    state.probeLatencyMs = 0;
    state.probeMessage = '';
    state.retryDone = false;
  }

  function providerBufferEvent(media, stateName) {
    if (!media) return;
    const state = stateFor(media);
    const now = Date.now();
    if (stateName === 'loadstart') {
      state.startedAt = now;
      state.playStartedAt = 0;
      state.lastProgressAt = now;
      state.lastBufferedEnd = 0;
      state.lastCurrentTime = 0;
      state.playRequested = false;
      state.everPlayable = false;
      state.everPlayed = false;
      resetProbeState(state);
    } else if (stateName === 'play') {
      state.playRequested = true;
      state.playStartedAt = now;
      state.lastProgressAt = now;
      resetProbeState(state);
    } else if (stateName === 'waiting' || stateName === 'stalled') {
      state.playRequested = !media.paused && !media.ended;
      if (state.playRequested && !state.playStartedAt) state.playStartedAt = now;
    } else if (stateName === 'canplay') {
      state.everPlayable = true;
    } else if (stateName === 'playing') {
      state.playRequested = true;
      state.everPlayable = true;
      state.everPlayed = true;
      state.lastProgressAt = now;
    } else if (stateName === 'pause' || stateName === 'ended') {
      state.playRequested = false;
      state.playStartedAt = 0;
    }
    noteProgress(media);
    setStatus(media, stateName, '');
  }

  async function probeRange(media) {
    const state = stateFor(media);
    if (state.probeStarted || state.probeDone || !state.playRequested || media.paused || media.ended) return;
    const src = String(media.currentSrc || media.src || '').trim();
    if (!src) return;
    state.probeStarted = true;
    setStatus(media, 'probing', 'BUNKR • verific dacă serverul suportă streaming Range…');
    const controller = new AbortController();
    const started = performance.now();
    try {
      const response = await fetch(src, {
        method: 'GET',
        headers: {'Range':'bytes=0-0'},
        cache: 'no-store',
        signal: controller.signal
      });
      state.probeLatencyMs = Math.round(performance.now() - started);
      state.probeStatus = response.status;
      const contentRange = String(response.headers.get('Content-Range') || '').trim();
      const acceptRanges = String(response.headers.get('Accept-Ranges') || '').trim().toLowerCase();
      state.rangeSupported = response.status === 206 || /^bytes\s+/i.test(contentRange) || acceptRanges === 'bytes';
      controller.abort();

      if (response.status >= 400) {
        state.probeMessage = `BUNKR • serverul răspunde HTTP ${response.status}; preview-ul nu poate primi date acum.`;
      } else if (!state.rangeSupported) {
        state.probeMessage = 'BUNKR • fișierul există, dar serverul nu respectă Range; unele MP4 pot porni foarte greu dacă metadata este la final.';
      } else if (state.probeLatencyMs >= 3000) {
        state.probeMessage = `BUNKR • Range OK, dar CDN-ul răspunde lent (${(state.probeLatencyMs / 1000).toFixed(1)}s până la headere).`;
      } else {
        state.probeMessage = `BUNKR • Range OK • CDN răspunde în ${(state.probeLatencyMs / 1000).toFixed(1)}s; verific playerul/containerul.`;
      }
    } catch (error) {
      if (error?.name !== 'AbortError') {
        state.probeMessage = `BUNKR • testul de streaming a eșuat: ${error?.message || String(error)}`;
      }
    } finally {
      state.probeDone = true;
      state.probeStarted = false;
    }
  }

  function maybeRetry(media) {
    const state = stateFor(media);
    if (state.retryDone || state.rangeSupported !== true || !state.playRequested || media.paused || media.ended) return false;
    if (media.readyState > HTMLMediaElement.HAVE_NOTHING || bufferedEnd(media) > 0.05) return false;
    state.retryDone = true;
    setStatus(media, 'probing', 'BUNKR • Range este OK, refac o singură dată conexiunea CDN…');
    try {
      media.load();
      state.startedAt = Date.now();
      state.playStartedAt = state.startedAt;
      state.lastProgressAt = state.startedAt;
      return true;
    } catch (_) {
      return false;
    }
  }

  function watchdog() {
    const media = document.getElementById('remoteVideo');
    if (!media || currentSource() !== 'BUNKR') return;
    const state = stateFor(media);
    noteProgress(media);

    if (media.error || media.ended) return;

    // A paused video is not buffering for playback. Do not overwrite a valid
    // paused/ready state with a stale watchdog warning, which was the cause of
    // false "no buffer" messages after a clip had already played successfully.
    if (media.paused || !state.playRequested) return;

    // If the browser already has future data, playback is healthy right now.
    // Normal progress/canplay/playing events own the status in this state.
    if (media.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA && bufferedAhead(media) >= 0.1) return;

    const base = state.playStartedAt || state.startedAt;
    const age = Date.now() - base;
    const idle = Date.now() - state.lastProgressAt;
    const ahead = bufferedAhead(media);

    if (age >= PROBE_AFTER_MS && ahead < 0.1 && !state.probeDone && !state.probeStarted) {
      probeRange(media);
      return;
    }

    if (age >= RETRY_AFTER_MS && idle >= RETRY_AFTER_MS && state.probeDone && maybeRetry(media)) return;

    if (state.probeDone && idle >= RETRY_AFTER_MS && ahead < 0.1) {
      if (state.probeStatus >= 400 || state.rangeSupported === false) {
        setStatus(media, 'problem', state.probeMessage);
      } else if (media.readyState === HTMLMediaElement.HAVE_NOTHING) {
        setStatus(media, 'slow', `${state.probeMessage} Metadata video încă nu a sosit după ${Math.round(age / 1000)}s.`);
      } else if (media.readyState < HTMLMediaElement.HAVE_FUTURE_DATA) {
        const context = state.everPlayed
          ? ' Redarea a funcționat înainte, dar poziția curentă nu primește suficient buffer.'
          : ' Metadata există, dar nu s-a format buffer suficient pentru redare.';
        setStatus(media, 'slow', `${state.probeMessage}${context}`);
      }
    }
  }

  function bunkrVideoHTML(url, name) {
    const ext = String(name || '').split('.').pop().toUpperCase();
    return `<video id="remoteVideo" controls playsinline preload="auto" src="${url}" ` +
      `onloadstart="providerBufferEvent(this,'loadstart')" ` +
      `onloadedmetadata="providerBufferEvent(this,'loadedmetadata')" ` +
      `onprogress="providerBufferEvent(this,'progress')" ` +
      `onplay="providerBufferEvent(this,'play')" ` +
      `onwaiting="providerBufferEvent(this,'waiting')" ` +
      `onstalled="providerBufferEvent(this,'stalled')" ` +
      `onseeking="providerBufferEvent(this,'seeking')" ` +
      `oncanplay="providerBufferEvent(this,'canplay')" ` +
      `onplaying="providerBufferEvent(this,'playing')" ` +
      `ontimeupdate="providerBufferEvent(this,'progress')" ` +
      `onpause="providerBufferEvent(this,'pause')" ` +
      `onended="providerBufferEvent(this,'ended')" ` +
      `onerror="remotePreviewError(this)"></video>` +
      `<div id="providerBufferStatus" class="providerBufferStatus" data-state="loadstart">BUNKR • pregătesc bufferul…</div>` +
      `<span class="miniInfo">${esc(ext)} • BUNKR stream</span>` +
      `<span class="trafficNote">BUNKR • buffer automat în player</span>`;
  }

  function install() {
    installStyle();
    if (typeof window.remoteMediaHTML !== 'function') {
      if (++installAttempts < 30) setTimeout(install, 100);
      return;
    }
    if (window.remoteMediaHTML.__ddgBunkrBufferV8561) return;

    const original = window.remoteMediaHTML;
    const wrapped = function(url, kind, name) {
      if (currentSource() === 'BUNKR' && kind === 'video') return bunkrVideoHTML(url, name);
      return original.apply(this, arguments);
    };
    wrapped.__ddgBunkrBufferV8561 = true;
    window.remoteMediaHTML = wrapped;
    window.providerBufferEvent = providerBufferEvent;
    setInterval(watchdog, WATCHDOG_MS);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
