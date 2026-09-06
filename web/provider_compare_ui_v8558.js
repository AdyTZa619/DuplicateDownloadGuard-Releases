(() => {
  'use strict';

  function rowNow() {
    try { return typeof currentRow !== 'undefined' ? currentRow : null; }
    catch (_) { return null; }
  }

  function sourceName(row = rowNow()) {
    const value = String(row?.remote?.source || 'REMOTE').trim().toUpperCase();
    return value || 'REMOTE';
  }

  function htmlEscape(value) {
    if (typeof window.esc === 'function') return window.esc(value);
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  function bunkrPublicMediaURL(row) {
    if (sourceName(row) !== 'BUNKR' || !row?.remote?.handle) return '';
    try {
      const album = new URL(row.remote.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(row.remote.handle))}`;
    } catch (_) {
      return '';
    }
  }

  function providerRemoteMediaHTML(url, kind, name) {
    const source = sourceName();
    const ext = String(name || '').split('.').pop().toUpperCase();
    const note = '<span class="trafficNote">REMOTE • streaming la cerere</span>';
    const tag = source === 'MEGA' ? 'MEGA' : source;
    if (kind === 'image') {
      return `<img id="remoteImage" src="${url}" alt="Preview remote" onerror="remotePreviewError()"><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)}</span>${note}`;
    }
    if (kind === 'video') {
      return `<video id="remoteVideo" controls preload="metadata" src="${url}" onerror="remotePreviewError()"></video><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)} stream</span>${note}`;
    }
    if (kind === 'audio') {
      return `<audio controls preload="metadata" src="${url}" onerror="remotePreviewError()"></audio><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)}</span>${note}`;
    }
    return '<div class="previewEmpty">Format fără player remote integrat.</div>';
  }

  function providerRemotePreviewError() {
    const box = document.getElementById('remotePreview');
    if (!box) return;
    const source = sourceName();
    box.innerHTML = `<div class="previewEmpty">Browserul nu poate reda streamul ${htmlEscape(source)} în forma curentă.<br><br><button class="btn primary" onclick="playRemote()">▶ Încearcă în player extern</button> <button class="btn" onclick="openRemote()">↗ Deschide sursa</button></div>`;
  }

  async function providerOpenRemote() {
    const row = rowNow();
    if (!row?.remote) return;
    const source = sourceName(row);
    const bunkr = bunkrPublicMediaURL(row);
    const target = bunkr || String(row.remote.url || '').trim();
    if (!target) return;
    try {
      await window.api('/api/open-remote', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({
          url: target,
          handle: source === 'MEGA' ? (row.remote.handle || '') : '',
          source
        })
      });
    } catch (error) {
      window.toast?.(error?.message || String(error));
    }
  }

  function tuneCompareUI(row = rowNow()) {
    if (!row?.remote) return;
    const source = sourceName(row);
    const open = document.getElementById('openRemote');
    if (open) {
      open.textContent = source === 'MEGA' ? '↗ MEGA' : `↗ ${source}`;
      open.title = source === 'MEGA' ? 'Deschide în MEGA' : `Deschide fișierul selectat în ${source}`;
    }

    const mediaInfo = document.getElementById('mediaInfo');
    if (mediaInfo && source !== 'MEGA' && /stream-ul MEGA/i.test(mediaInfo.textContent || '')) {
      mediaInfo.innerHTML = '<span class="muted">Apasă <b>MediaInfo</b> pentru analiza tehnică REMOTE ↔ LOCAL. Remote-ul este citit prin proxy-ul local DDG al providerului și poate transfera o cantitate mică de date pentru headere.</span>';
    }

    const detail = document.getElementById('detail');
    if (detail && source !== 'MEGA') {
      for (const label of detail.querySelectorAll('b')) {
        if (label.textContent.trim() === 'Handle remote') label.textContent = 'ID/handle provider';
      }
    }
  }

  function install() {
    window.remoteMediaHTML = providerRemoteMediaHTML;
    window.remotePreviewError = providerRemotePreviewError;
    window.openRemote = providerOpenRemote;

    const original = window.showDetail;
    if (typeof original === 'function' && !original.__providerAwareV8558) {
      const wrapped = async function(row) {
        const out = await original.apply(this, arguments);
        tuneCompareUI(row);
        setTimeout(() => tuneCompareUI(row), 0);
        return out;
      };
      wrapped.__providerAwareV8558 = true;
      window.showDetail = wrapped;
    }
    tuneCompareUI();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
