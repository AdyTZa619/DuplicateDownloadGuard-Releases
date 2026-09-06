(() => {
  'use strict';

  let installAttempts = 0;

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
      .providerBufferStatus[data-state="waiting"],.providerBufferStatus[data-state="stalled"]{border-color:#927326;color:#ffe08a}
      .providerBufferStatus[data-state="playing"]{border-color:#286a50;color:#8af0c1}
    `;
    document.head.appendChild(style);
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

  function bufferedPercent(media) {
    const duration = Number(media?.duration || 0);
    if (!Number.isFinite(duration) || duration <= 0 || !media?.buffered?.length) return 0;
    let end = 0;
    try {
      for (let i = 0; i < media.buffered.length; i++) end = Math.max(end, media.buffered.end(i));
    } catch (_) {}
    return Math.max(0, Math.min(100, end / duration * 100));
  }

  function statusText(media, state) {
    const source = currentSource();
    const ahead = bufferedAhead(media);
    const pct = bufferedPercent(media);
    const aheadText = ahead >= 0.1 ? `${ahead.toFixed(ahead >= 10 ? 0 : 1)}s buffer înainte` : 'buffer aproape gol';
    const pctText = pct >= 1 ? ` • ${pct.toFixed(0)}% încărcat` : '';

    switch (state) {
      case 'loadstart': return `${source} • pregătesc bufferul…`;
      case 'loadedmetadata': return `${source} • metadata gata • ${aheadText}${pctText}`;
      case 'progress': return `${source} • ${aheadText}${pctText}`;
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

  function providerBufferEvent(media, state) {
    const status = document.getElementById('providerBufferStatus');
    if (!status || !media) return;
    status.dataset.state = state || '';
    status.textContent = statusText(media, state || 'progress');
  }

  function bunkrVideoHTML(url, name) {
    const ext = String(name || '').split('.').pop().toUpperCase();
    return `<video id="remoteVideo" controls playsinline preload="auto" src="${url}" ` +
      `onloadstart="providerBufferEvent(this,'loadstart')" ` +
      `onloadedmetadata="providerBufferEvent(this,'loadedmetadata')" ` +
      `onprogress="providerBufferEvent(this,'progress')" ` +
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
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
