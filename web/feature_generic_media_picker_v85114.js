// TEST v8.5.114 — generic HTTP Media Picker bridge.
// Isolated feature: it does not override DDG preview, MEGA, JDownloader or selection functions.
(() => {
  'use strict';

  const ID = 'ddgGenericMediaPickerV85114';
  let pendingURL = '';
  let openSeq = 0;

  function parseURL(raw) {
    try {
      const value = String(raw || '').trim();
      if (!value) return null;
      return new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`);
    } catch (_) {
      return null;
    }
  }

  function isGenericHTTP(raw) {
    const u = parseURL(raw);
    if (!u || !/^https?:$/i.test(u.protocol)) return false;
    const h = u.hostname.toLowerCase();
    if (/^(?:www\.)?mega\.(?:nz|co\.nz)$/i.test(h)) return false;
    if (/(^|\.)gofile\.io$/i.test(h)) return false;
    if (/(^|\.)bunkr[a-z0-9-]*\.[a-z]{2,}$/i.test(h) || /(^|\.)bunkrr\.[a-z]{2,}$/i.test(h)) return false;
    if (/(^|\.)cyberdrop\.[a-z]{2,}$/i.test(h)) return false;
    if (/(^|\.)erome\.com$/i.test(h)) return false;
    return true;
  }

  function currentURL() {
    return String(document.getElementById('directUrl')?.value || '').trim();
  }

  function ensureBadge() {
    const state = document.getElementById('universalProviderState');
    if (!state?.parentElement) return null;
    let box = document.getElementById(ID);
    if (!box) {
      box = document.createElement('div');
      box.id = ID;
      box.className = 'muted small';
      box.style.cssText = 'margin-top:7px;padding:7px 9px;border:1px dashed #304256;border-radius:8px;background:#0b121a;display:none';
      state.insertAdjacentElement('afterend', box);
    }
    return box;
  }

  function renderBadge() {
    const box = ensureBadge();
    if (!box) return;
    const raw = currentURL();
    if (!isGenericHTTP(raw)) {
      box.style.display = 'none';
      return;
    }
    const u = parseURL(raw);
    box.style.display = '';
    box.innerHTML = `<b>Media Picker generic</b> • ${String(u?.hostname || 'WEB')} • după scanare deschid automat fișierele detectate de HTTP → yt-dlp → gallery-dl.`;
  }

  function arm(raw) {
    pendingURL = isGenericHTTP(raw) ? String(raw || '').trim() : '';
    if (pendingURL) renderBadge();
  }

  async function openPickerWhenReady(raw) {
    const seq = ++openSeq;
    for (let i = 0; i < 30; i++) {
      if (seq !== openSeq || pendingURL !== raw) return;
      const picker = window.ddgMediaPickerV8566;
      if (picker?.open) {
        const modal = document.getElementById('ddgMediaPickerV8566');
        if (!modal || modal.classList.contains('hidden')) await picker.open(raw);
        pendingURL = '';
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  }

  function watchStatus() {
    const top = document.getElementById('topStatus');
    if (!top || top.dataset.ddgGenericMediaPickerWatchV85114 === '1') return;
    top.dataset.ddgGenericMediaPickerWatchV85114 = '1';
    const observer = new MutationObserver(() => {
      const text = String(top.textContent || '').toLowerCase();
      if (text.includes('eroare')) {
        pendingURL = '';
        return;
      }
      if (!pendingURL || !text.includes('analiză terminată')) return;
      const raw = pendingURL;
      setTimeout(() => openPickerWhenReady(raw), 80);
    });
    observer.observe(top, {childList:true, characterData:true, subtree:true});
  }

  function bind() {
    const input = document.getElementById('directUrl');
    if (input && input.dataset.ddgGenericMediaPickerV85114 !== '1') {
      input.dataset.ddgGenericMediaPickerV85114 = '1';
      input.addEventListener('input', renderBadge);
      input.addEventListener('paste', () => setTimeout(renderBadge, 0));
      input.addEventListener('keydown', event => {
        if (event.key === 'Enter') arm(input.value);
      }, true);
    }

    document.addEventListener('click', event => {
      const button = event.target?.closest?.('#universalScanButton, button[onclick="scanUniversal()"]');
      if (!button) return;
      arm(currentURL());
    }, true);

    renderBadge();
    watchStatus();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true});
  else bind();
  setTimeout(bind, 500);
  window.ddgGenericMediaPickerV85114 = {isGenericHTTP, arm, renderBadge};
})();
