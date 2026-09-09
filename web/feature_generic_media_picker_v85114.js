// TEST127 — generic HTTP Media Picker bridge integrated into Source Intelligence.
// Media Picker is intentionally a generic-site tool only. Dedicated hosts such
// as MEGA/GoFile/Bunkr/Cyberdrop/Erome keep their provider-specific flow.
(() => {
  'use strict';

  let pendingURL = '';
  let openSeq = 0;
  let layoutAttempts = 0;

  function parseURL(raw) {
    try {
      const value = String(raw || '').trim();
      if (!value) return null;
      return new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`);
    } catch (_) {
      return null;
    }
  }

  function providerKind(raw) {
    const u = parseURL(raw);
    if (!u || !/^https?:$/i.test(u.protocol)) return 'none';
    const h = u.hostname.toLowerCase();
    if (/^(?:www\.)?mega\.(?:nz|co\.nz)$/i.test(h)) return 'mega';
    if (/(^|\.)gofile\.io$/i.test(h)) return 'gofile';
    if (/(^|\.)bunkr[a-z0-9-]*\.[a-z]{2,}$/i.test(h) || /(^|\.)bunkrr\.[a-z]{2,}$/i.test(h)) return 'bunkr';
    if (/(^|\.)cyberdrop\.[a-z]{2,}$/i.test(h)) return 'cyberdrop';
    if (/(^|\.)erome\.com$/i.test(h)) return 'erome';
    return 'web';
  }

  function isGenericHTTP(raw) {
    return providerKind(raw) === 'web';
  }

  function currentURL() {
    return String(document.getElementById('directUrl')?.value || '').trim();
  }

  function mediaInfoBox() {
    return document.getElementById('ddgSourceMediaInfoV85124');
  }

  function loadAdvancedDiscoveryV85127() {
    if (document.getElementById('ddgGenericMediaDiscoveryScriptV85127')) return;
    const script = document.createElement('script');
    script.id = 'ddgGenericMediaDiscoveryScriptV85127';
    script.defer = true;
    script.src = '/features/generic_media_discovery_v85127.js';
    document.head.appendChild(script);
  }

  function syncPickerVisibility() {
    const generic = isGenericHTTP(currentURL());
    const actions = document.getElementById('ddgSourceMediaActionsV85124');
    const pickerButton = document.getElementById('ddgMediaPickerLaunchV8566');
    if (pickerButton) {
      pickerButton.hidden = !generic;
      pickerButton.style.display = generic ? '' : 'none';
      pickerButton.setAttribute('aria-hidden', generic ? 'false' : 'true');
    }
    if (actions) actions.dataset.genericMedia = generic ? '1' : '0';
    return generic;
  }

  function settleLayout() {
    const actions = document.getElementById('ddgSourceMediaActionsV85124');
    const pickerButton = document.getElementById('ddgMediaPickerLaunchV8566');
    if (actions && pickerButton && pickerButton.parentElement !== actions) {
      actions.appendChild(pickerButton);
      pickerButton.style.marginTop = '0';
    }

    const folderSlot = document.getElementById('ddgSourceFolderSlotV85124');
    const folderPanel = document.getElementById('ddgSourceFolderHintV85114');
    if (folderSlot && folderPanel && folderPanel.parentElement !== folderSlot) {
      folderSlot.replaceChildren(folderPanel);
    }

    syncPickerVisibility();

    if ((!pickerButton || !folderPanel) && layoutAttempts < 32) {
      layoutAttempts++;
      setTimeout(settleLayout, 250);
    }
  }

  function renderInfo() {
    const box = mediaInfoBox();
    if (!box) return;
    const raw = currentURL();
    const kind = providerKind(raw);
    const u = parseURL(raw);
    syncPickerVisibility();

    if (kind === 'none') {
      box.innerHTML = 'Media Picker apare numai pentru site-uri web generice, după ce DDG extrage media din pagină.';
      return;
    }
    if (kind === 'web') {
      box.innerHTML = `<b>${String(u?.hostname || 'WEB')}</b> • site generic: HTML/iframe + HLS/DASH + yt-dlp + gallery-dl, fără download înainte de comparație. După analiză deschid automat Media Picker.`;
      return;
    }
    if (kind === 'mega') {
      box.innerHTML = '<b>MEGA</b> este host dedicat și folosește fluxul/preview-ul MEGA. Media Picker generic este dezactivat aici.';
      return;
    }
    const names = {gofile:'GoFile', bunkr:'Bunkr', cyberdrop:'Cyberdrop', erome:'Erome'};
    box.innerHTML = `<b>${names[kind] || 'Provider'}</b> este host dedicat. DDG folosește extractorul providerului; Media Picker generic nu se afișează pentru această sursă.`;
  }

  function arm(raw) {
    pendingURL = isGenericHTTP(raw) ? String(raw || '').trim() : '';
    renderInfo();
  }

  async function openPickerWhenReady(raw) {
    const seq = ++openSeq;
    for (let i = 0; i < 30; i++) {
      if (seq !== openSeq || pendingURL !== raw || !isGenericHTTP(raw)) return;
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

  async function openDiscoveryWhenReady(raw, discovery) {
    const seq = ++openSeq;
    for (let i = 0; i < 30; i++) {
      if (seq !== openSeq || !isGenericHTTP(raw)) return;
      const picker = window.ddgMediaPickerV8566;
      if (picker?.openDiscovery) {
        await picker.openDiscovery(raw, discovery);
        pendingURL = '';
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  }

  function bind() {
    loadAdvancedDiscoveryV85127();
    const input = document.getElementById('directUrl');
    if (input && input.dataset.ddgGenericMediaPickerV85124 !== '1') {
      input.dataset.ddgGenericMediaPickerV85124 = '1';
      input.addEventListener('input', renderInfo);
      input.addEventListener('paste', () => setTimeout(renderInfo, 0));
    }

    if (document.documentElement.dataset.ddgGenericMediaEventsV85124 !== '1') {
      document.documentElement.dataset.ddgGenericMediaEventsV85124 = '1';
      window.addEventListener('ddg:source-scan-start', event => arm(event.detail?.url || currentURL()));
      window.addEventListener('ddg:generic-media-discovered', event => {
        const raw = String(event.detail?.url || '').trim();
        if (!isGenericHTTP(raw)) return;
        settleLayout();
        void openDiscoveryWhenReady(raw, event.detail?.discovery || {});
      });
      window.addEventListener('ddg:source-scan-complete', event => {
        const raw = String(event.detail?.url || '').trim();
        settleLayout();
        window.ddgSourceFolderHintV85114?.refresh?.();
        if (event.detail?.mediaPicker) { pendingURL = ''; return; }
        if (!pendingURL || pendingURL !== raw || !isGenericHTTP(raw)) return;
        setTimeout(() => openPickerWhenReady(raw), 80);
      });
      window.addEventListener('ddg:source-scan-error', () => { pendingURL = ''; });
    }

    renderInfo();
    settleLayout();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind, {once:true});
  else bind();
  setTimeout(bind, 500);
  window.ddgGenericMediaPickerV85114 = {isGenericHTTP, providerKind, arm, renderBadge:renderInfo, settleLayout, syncPickerVisibility};
})();
