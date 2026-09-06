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

  function tuneCompareUI(row = rowNow()) {
    if (!row?.remote) return;
    const source = sourceName(row);
    const open = document.getElementById('openRemote');
    if (open) {
      open.textContent = source === 'MEGA' ? '↗ MEGA' : `↗ ${source}`;
      open.title = source === 'MEGA' ? 'Deschide în MEGA' : `Deschide sursa ${source}`;
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
