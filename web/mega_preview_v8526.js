(() => {
  'use strict';

  const baseLoadRemotePreviewV8526 = window.loadRemotePreview;
  let activeV8526 = null;

  const nowMS = () => Date.now();
  const currentIs = (row, seq) => seq === detailSeq && currentRow && Number(currentRow.id) === Number(row.id);

  function sendEventV8526(generation, event, detail = '') {
    if (!generation) return;
    fetch('/api/remote-preview/event', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({generation, event, detail, clientAt: nowMS()}),
      keepalive: true
    }).catch(() => {});
  }

  function removeRemoteMediaV8526(reason) {
    const previous = activeV8526;
    if (previous) {
      sendEventV8526(previous.generation, 'T11', reason || 'player switch');
      previous.pollAbort?.abort();
    }
    const preview = document.getElementById('remotePreview');
    if (preview) {
      for (const media of preview.querySelectorAll('video,audio')) {
        try { media.onerror = null; } catch (_) {}
        try { media.pause(); } catch (_) {}
        try { media.remove(); } catch (_) {}
      }
      for (const image of preview.querySelectorAll('img')) {
        try { image.onerror = null; } catch (_) {}
        try { image.remove(); } catch (_) {}
      }
    }
    if (previous) {
      requestAnimationFrame(() => sendEventV8526(previous.generation, 'T12', 'element DOM detached'));
    }
    activeV8526 = null;
  }

  function stageTextV8526(trace) {
    const p = trace?.points || {};
    if (trace?.state === 'error') return trace?.problem?.title || trace?.error || 'Eroare MEGA';
    if (p.T10) return 'Primul cadru a fost afișat.';
    if (p.T9) return 'Metadate încărcate; aștept primul cadru…';
    if (p.T8) return 'Primii bytes au ajuns în player…';
    if (p.T5) return 'WebDAV răspunde; playerul citește streamul…';
    if (p.T4) return 'WebDAV este gata; pornesc cererea media…';
    if (p.T3) return 'MEGAcmd pregătește o singură sesiune WebDAV…';
    return 'Rezolv sursa MEGA…';
  }

  function errorHTMLV8526(row, trace, fallbackAllowed) {
    const problem = trace?.problem;
    const title = problem?.title || 'MEGA Preview indisponibil';
    const message = problem?.message || trace?.error || 'Streamul nu a furnizat date playerului.';
    const action = problem?.action || 'Verifică jurnalul MEGA Preview pentru etapa exactă.';
    const retry = fallbackAllowed
      ? `<button class="btn primary" onclick="megaPreviewFallbackV8526(${Number(row.id)})">Încearcă fallback per-fișier</button> `
      : '';
    return `<div class="previewEmpty"><b>${esc(title)}</b><br>${esc(message)}<br><span class="small">${esc(action)}</span><br><br>${retry}<button class="btn" onclick="playRemote()">▶ Player extern</button> <button class="btn" onclick="openRemote()">↗ MEGA</button></div>`;
  }

  async function statusV8526(generation, signal) {
    const response = await fetch(`/api/remote-preview/status?generation=${generation}`, {signal, cache: 'no-store'});
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }

  async function pollStatusV8526(row, seq, generation, forceFallback, controller) {
    const statusEl = document.getElementById('megaPreviewStageV8526');
    while (!controller.signal.aborted && currentIs(row, seq) && activeV8526?.generation === generation) {
      try {
        const trace = await statusV8526(generation, controller.signal);
        if (statusEl) statusEl.textContent = stageTextV8526(trace);
        const source = document.getElementById('remoteSource');
        if (source && trace.route) source.textContent = `MEGA ${String(trace.route).toUpperCase()} • ${trace.state || 'PREPARING'}`;
        if (trace.state === 'error') {
          const preview = document.getElementById('remotePreview');
          if (preview && currentIs(row, seq)) preview.innerHTML = errorHTMLV8526(row, trace, !forceFallback);
          return;
        }
        if (trace.points?.T10) return;
      } catch (error) {
        if (error?.name === 'AbortError') return;
      }
      await new Promise(resolve => setTimeout(resolve, 350));
    }
  }

  function installMediaV8526(row, seq, data, forceFallback) {
    if (!currentIs(row, seq)) return;
    const preview = document.getElementById('remotePreview');
    if (!preview) return;
    const generation = Number(data.generation || 0);
    const kind = data.kind || previewKind(row.remote?.name || row.remote?.path || '');
    const name = row.remote?.name || row.remote?.path || 'remote';
    const ext = (name.split('.').pop() || '').toUpperCase();
    const pollAbort = new AbortController();
    activeV8526 = {generation, row, seq, forceFallback, pollAbort};

    preview.innerHTML = '';
    const media = document.createElement(kind === 'image' ? 'img' : kind);
    if (kind === 'image') {
      media.id = 'remoteImage';
      media.alt = 'Preview remote';
    } else {
      media.controls = true;
      media.preload = 'metadata';
      if (kind === 'video') media.id = 'remoteVideo';
    }
    media.addEventListener('loadstart', () => sendEventV8526(generation, 'T7', `${kind} loadstart`), {once: true});
    media.addEventListener('loadedmetadata', () => sendEventV8526(generation, 'T9', `${kind} metadata`), {once: true});
    media.addEventListener('error', () => window.megaPreviewErrorV8526(generation), {once: true});
    if (kind === 'image') {
      media.addEventListener('load', () => {
        sendEventV8526(generation, 'T9', 'image decoded');
        sendEventV8526(generation, 'T10', 'image rendered');
      }, {once: true});
    } else if (kind === 'video') {
      media.addEventListener('loadeddata', () => {
        if (typeof media.requestVideoFrameCallback === 'function') {
          let reported = false;
          media.requestVideoFrameCallback(() => {
            reported = true;
            sendEventV8526(generation, 'T10', 'video frame callback');
          });
          setTimeout(() => {
            if (!reported) sendEventV8526(generation, 'T10', 'video first frame loaded');
          }, 250);
        } else {
          sendEventV8526(generation, 'T10', 'video first frame loaded');
        }
      }, {once: true});
    } else {
      media.addEventListener('canplay', () => sendEventV8526(generation, 'T10', 'audio can play'), {once: true});
    }

    preview.appendChild(media);
    const info = document.createElement('span');
    info.className = 'miniInfo';
    info.textContent = `${ext} • MEGA managed stream`;
    preview.appendChild(info);
    const stage = document.createElement('span');
    stage.className = 'trafficNote';
    stage.id = 'megaPreviewStageV8526';
    stage.textContent = 'Se pregătește previzualizarea…';
    preview.appendChild(stage);

    sendEventV8526(generation, 'T6', `${kind} URL assigned`);
    media.src = data.url;
    try { remotePreviewActive = true; } catch (_) {}
    const stop = document.getElementById('stopRemote');
    if (stop) stop.disabled = false;
    const source = document.getElementById('remoteSource');
    if (source) {
      source.textContent = `${data.source || 'MEGA MANAGED PREVIEW'} • LIVE`;
      source.classList.add('remoteLive');
    }
    pollStatusV8526(row, seq, generation, forceFallback, pollAbort);
  }

  async function requestPreviewV8526(row, seq, forceFallback) {
    removeRemoteMediaV8526(forceFallback ? 'explicit fallback' : 'new selection');
    if (!currentIs(row, seq)) return;
    const preview = document.getElementById('remotePreview');
    if (preview) preview.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Se pregătește previzualizarea…</b><span class="small">Controler MEGA unic • timpi T0–T12 activi</span></div>';
    const clientT0 = nowMS();
    try {
      const data = await api('/api/remote-preview/start', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({id: row.id, forceFallback: !!forceFallback, clientT0})
      });
      if (!currentIs(row, seq)) return;
      installMediaV8526(row, seq, data, forceFallback);
    } catch (error) {
      if (!currentIs(row, seq)) return;
      if (preview) preview.innerHTML = `<div class="previewEmpty"><b>MEGA Preview indisponibil.</b><br>${esc(error.message)}<br><br><button class="btn" onclick="playRemote()">▶ Player extern</button></div>`;
    }
  }

  window.loadRemotePreview = function (row, seq) {
    const source = String(row?.remote?.source || '').toUpperCase();
    if (source !== 'MEGA') return baseLoadRemotePreviewV8526(row, seq);
    const kind = previewKind(row.remote?.name || row.remote?.path || '');
    if (!row.remote?.url || kind === 'other') return baseLoadRemotePreviewV8526(row, seq);
    return requestPreviewV8526(row, seq, false);
  };

  window.megaPreviewFallbackV8526 = function (id) {
    if (!currentRow || Number(currentRow.id) !== Number(id)) return;
    return requestPreviewV8526(currentRow, detailSeq, true);
  };

  window.megaPreviewErrorV8526 = async function (generation) {
    const active = activeV8526;
    if (!active || active.generation !== Number(generation) || !currentIs(active.row, active.seq)) return;
    try {
      const trace = await statusV8526(generation, active.pollAbort.signal);
      const preview = document.getElementById('remotePreview');
      if (preview && activeV8526?.generation === Number(generation)) {
        preview.innerHTML = errorHTMLV8526(active.row, trace, !active.forceFallback);
      }
    } catch (_) {}
  };

  function installFinalOverridesV8526() {
    // preview_quick_v86 installs legacy error wrappers on DOMContentLoaded.
    // Reclaim final ownership so a browser error cannot trigger two fallbacks.
    window.remotePreviewError = function () {
      if (activeV8526) window.megaPreviewErrorV8526(activeV8526.generation);
    };
    window.stopRemotePreview = async function () {
      removeRemoteMediaV8526('user stop');
      try { await fetch('/api/remote-preview/stop', {method: 'POST', keepalive: true}); } catch (_) {}
      try { remotePreviewActive = false; } catch (_) {}
      const stop = document.getElementById('stopRemote');
      if (stop) stop.disabled = true;
      const preview = document.getElementById('remotePreview');
      if (preview) preview.innerHTML = '<div class="previewEmpty">Preview remote oprit. Sesiunea MEGA rămâne pregătită pentru următorul fișier.</div>';
    };
    window.addEventListener('pagehide', () => {
      removeRemoteMediaV8526('pagehide');
      try { navigator.sendBeacon('/api/remote-preview/stop', new Blob([], {type: 'text/plain'})); } catch (_) {}
    }, {capture: true});
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', installFinalOverridesV8526, {once: true});
  } else {
    installFinalOverridesV8526();
  }
})();
