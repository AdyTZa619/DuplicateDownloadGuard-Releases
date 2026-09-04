(() => {
  'use strict';

  let row = null;
  const meta = { remote: {}, local: {} };
  let restartPrewarmStarted = false;

  const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
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

  function install() {
    if (document.getElementById('previewQuickV86Styles')) return;
    const style = document.createElement('style');
    style.id = 'previewQuickV86Styles';
    style.textContent = `
      .previewHead .previewQuickV86{flex:1;min-width:0;text-align:center;color:#d7e7f8;font-size:11px;font-weight:750;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;padding:0 8px}
      .previewHead .previewQuickV86.good{color:#8ee6bd}.previewHead .previewQuickV86.warn{color:#ffd979}.previewHead .previewQuickV86.bad{color:#ff9aa5}
      @media(max-width:760px){.previewHead{flex-wrap:wrap;gap:6px}.previewHead .previewQuickV86{order:3;flex-basis:100%;text-align:left;padding:0}}
    `;
    document.head.appendChild(style);

    const remoteHead = document.getElementById('remotePreview')?.closest('.previewCard')?.querySelector('.previewHead');
    const localHead = document.getElementById('localPreview')?.closest('.previewCard')?.querySelector('.previewHead');
    if (remoteHead && !document.getElementById('remoteQuickV86')) remoteHead.querySelector('b')?.insertAdjacentHTML('afterend','<span class="previewQuickV86" id="remoteQuickV86">—</span>');
    if (localHead && !document.getElementById('localQuickV86')) localHead.querySelector('b')?.insertAdjacentHTML('afterend','<span class="previewQuickV86" id="localQuickV86">—</span>');
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
      reset(r);
      const out = await original.apply(this, arguments);
      setTimeout(render, 0);
      return out;
    };
    wrapped.__previewQuickV86 = true;
    window.showDetail = wrapped;
  }

  // The backend still prepares/reuses MEGAcmd WebDAV, but Edge never receives
  // that raw localhost port anymore. All embedded MEGA media is served through
  // DDG's same-origin proxy, which forwards HTTP Range/206 for seek and avoids
  // browser↔MEGAcmd CORS/origin/port failures.
  function installMegaProxyV860() {
    if (typeof window.remoteMediaHTML !== 'function' || window.remoteMediaHTML.__ddgProxyV860) return;
    const original = window.remoteMediaHTML;
    const wrapped = function(u, kind, name) {
      let mediaURL = u;
      const isMega = String(currentRow?.remote?.source || '').toUpperCase() === 'MEGA';
      const id = Number(currentRow?.id || 0);
      if (isMega && id > 0) mediaURL = `/api/remote-preview/proxy?id=${encodeURIComponent(id)}`;
      let html = original(mediaURL, kind, name);
      if (isMega && id > 0) html = html.replace('REMOTE • streaming la cerere', 'REMOTE • DDG proxy • Range');
      return html;
    };
    wrapped.__ddgProxyV860 = true;
    window.remoteMediaHTML = wrapped;
  }

  function observe() {
    for (const id of ['remotePreview','localPreview']) {
      const el = document.getElementById(id);
      if (!el || el.dataset.quickObserverV86) continue;
      el.dataset.quickObserverV86 = '1';
      new MutationObserver(() => setTimeout(render, 0)).observe(el, {childList:true, subtree:true});
    }
  }

  async function prewarmMegaAfterRestart() {
    if (restartPrewarmStarted) return;
    restartPrewarmStarted = true;
    try {
      const st = await api('/api/status');
      if (st?.active) return;
      const d = await api('/api/results?offset=0&limit=100');
      const candidate = (d?.rows || []).find(r => String(r?.remote?.source || '').toUpperCase() === 'MEGA' && r?.remote?.url && ['image','video','audio'].includes(previewKind(r.remote.name || r.remote.path || '')));
      if (!candidate) return;
      await api('/api/remote-preview/start', {
        method: 'POST',
        headers: {'Content-Type':'application/json'},
        body: JSON.stringify({id:candidate.id})
      });
      const src = document.getElementById('remoteSource');
      if (src && !currentRow) src.title = 'MEGA preview preîncălzit după restart';
    } catch (_) {
      // Opportunistic only: normal row selection keeps the compatibility path
      // and will surface a concrete MEGA error when the user actually needs it.
    }
  }

  function boot() {
    install();
    wrapShowDetail();
    installMegaProxyV860();
    observe();
    render();
    setTimeout(prewarmMegaAfterRestart, 2500);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
})();