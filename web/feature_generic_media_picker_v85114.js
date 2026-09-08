// TEST124 — generic HTTP Media Picker bridge integrated into Source Intelligence.
// Uses explicit source-scan events instead of watching status text mutations.
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
    if (kind === 'none') {
      box.innerHTML = 'Media Picker folosește rezultatele scanării curente, fără rescanare HDD.';
      return;
    }
    if (kind === 'web') {
      box.innerHTML = `<b>${String(u?.hostname || 'WEB')}</b> • extracție generică HTTP → yt-dlp → gallery-dl. După analiză deschid automat Media Picker.`;
      return;
    }
    if (kind === 'mega') {
      box.innerHTML = '<b>MEGA</b> folosește preview-ul dedicat. Media Picker rămâne disponibil pentru rezultatele curente.';
      return;
    }
    const names = {gofile:'GoFile', bunkr:'Bunkr', cyberdrop:'Cyberdrop', erome:'Erome'};
    box.innerHTML = `<b>${names[kind] || 'Provider'}</b> • Media Picker folosește rezultatele extrase de provider. Îl poți deschide după scanare.`;
  }

  function arm(raw) {
    pendingURL = isGenericHTTP(raw) ? String(raw || '').trim() : '';
    renderInfo();
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

  function bind() {
    const input = document.getElementById('directUrl');
    if (input && input.dataset.ddgGenericMediaPickerV85124 !== '1') {
      input.dataset.ddgGenericMediaPickerV85124 = '1';
      input.addEventListener('input', renderInfo);
      input.addEventListener('paste', () => setTimeout(renderInfo, 0));
    }

    if (document.documentElement.dataset.ddgGenericMediaEventsV85124 !== '1') {
      document.documentElement.dataset.ddgGenericMediaEventsV85124 = '1';
      window.addEventListener('ddg:source-scan-start', event => arm(event.detail?.url || currentURL()));
      window.addEventListener('ddg:source-scan-complete', event => {
        const raw = String(event.detail?.url || '').trim();
        settleLayout();
        window.ddgSourceFolderHintV85114?.refresh?.();
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
  window.ddgGenericMediaPickerV85114 = {isGenericHTTP, arm, renderBadge:renderInfo, settleLayout};
})();
