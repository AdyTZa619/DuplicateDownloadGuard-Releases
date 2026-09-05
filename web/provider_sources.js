(() => {
  'use strict';

  const PROVIDERS = [
    {id:'mega', name:'MEGA', adapter:'mega', hosts:[/^mega\.nz$/i, /^mega\.co\.nz$/i], note:'motor MEGAcmd dedicat'},
    {id:'gofile', name:'GoFile', adapter:'gallery-dl', hosts:[/(^|\.)gofile\.io$/i], note:'gallery-dl • foldere recursive'},
    {id:'bunkr', name:'Bunkr', adapter:'gallery-dl', hosts:[/(^|\.)bunkr[a-z0-9-]*\.[a-z]{2,}$/i, /(^|\.)bunkrr\.[a-z]{2,}$/i], note:'gallery-dl • albume/media'},
    {id:'cyberdrop', name:'Cyberdrop', adapter:'gallery-dl', hosts:[/(^|\.)cyberdrop\.[a-z]{2,}$/i], note:'gallery-dl • albume/media'},
    {id:'pixeldrain', name:'Pixeldrain', adapter:'auto', hosts:[/(^|\.)pixeldrain\.com$/i], note:'detecție automată'},
    {id:'mediafire', name:'MediaFire', adapter:'auto', hosts:[/(^|\.)mediafire\.com$/i], note:'detecție automată'}
  ];

  let galleryReady = null;
  let galleryCheckAt = 0;

  function parseURL(raw) {
    try {
      const value = String(raw || '').trim();
      if (!value) return null;
      return new URL(/^https?:\/\//i.test(value) ? value : 'https://' + value);
    } catch (_) {
      return null;
    }
  }

  function detectProvider(raw) {
    const u = parseURL(raw);
    if (!u) return null;
    const host = u.hostname.toLowerCase();
    for (const p of PROVIDERS) {
      if (p.hosts.some(rx => rx.test(host))) return {...p, host};
    }
    if (u.protocol === 'http:' || u.protocol === 'https:') {
      return {id:'web', name:'Web / HTTP', adapter:'auto', host, note:'HTTP → yt-dlp → gallery-dl'};
    }
    return null;
  }

  async function jsonFetch(url, options) {
    const response = await fetch(url, options);
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : response.text();
  }

  async function checkGalleryDL(force = false) {
    if (!force && galleryReady !== null && Date.now() - galleryCheckAt < 15000) return galleryReady;
    galleryCheckAt = Date.now();
    try {
      const tools = await jsonFetch('/api/tools');
      const item = Array.isArray(tools) ? tools.find(x => String(x.name || '').toLowerCase() === 'gallery-dl') : null;
      galleryReady = !!item?.found;
    } catch (_) {
      galleryReady = null;
    }
    renderProviderState();
    return galleryReady;
  }

  function stateBox() {
    return document.getElementById('universalProviderState');
  }

  function renderProviderState() {
    const input = document.getElementById('directUrl');
    const box = stateBox();
    if (!input || !box) return;
    const p = detectProvider(input.value);
    if (!p) {
      box.innerHTML = '<span class="muted">Lipește un link. DDG va alege automat motorul potrivit.</span>';
      return;
    }
    const needsGallery = p.adapter === 'gallery-dl';
    const ready = !needsGallery ? '' : galleryReady === true ? '<span class="badge VERIFIED">MOTOR GATA</span>' : galleryReady === false ? '<span class="badge DIFFERENT">GALLERY-DL LIPSEȘTE</span>' : '<span class="badge POSSIBLE">VERIFIC MOTORUL</span>';
    const action = needsGallery && galleryReady === false ? '<button class="btn" id="prepareGalleryDL" type="button">Instalează / actualizează gallery-dl</button>' : '';
    box.innerHTML = `<div class="providerStateLine"><b>${escapeHTML(p.name)}</b><span class="sourcePill">${escapeHTML(p.note)}</span>${ready}</div><div class="muted small providerStateHint">Listarea și comparația folosesc metadata/URL-urile extrase fără a descărca în prealabil tot conținutul.</div>${action}`;
    document.getElementById('prepareGalleryDL')?.addEventListener('click', installGalleryDL);
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  async function installGalleryDL() {
    const button = document.getElementById('prepareGalleryDL');
    if (button) { button.disabled = true; button.textContent = 'Pregătesc gallery-dl…'; }
    try {
      await jsonFetch('/api/tools/manage', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({tool:'gallery-dl', action:'install'})
      });
      if (typeof window.toast === 'function') window.toast('Instalarea/actualizarea gallery-dl a pornit');
      galleryReady = null;
      galleryCheckAt = 0;
      setTimeout(() => checkGalleryDL(true), 2500);
    } catch (err) {
      if (typeof window.toast === 'function') window.toast(err.message);
      if (button) { button.disabled = false; button.textContent = 'Reîncearcă gallery-dl'; }
    }
  }

  async function universalScanV2() {
    const input = document.getElementById('directUrl');
    const select = document.getElementById('sourceAdapter');
    const raw = String(input?.value || '').trim();
    if (!raw) {
      if (typeof window.toast === 'function') window.toast('Lipește un link');
      return;
    }
    const provider = detectProvider(raw);
    if (provider?.id === 'mega') {
      const mega = document.getElementById('megaUrl');
      if (mega) mega.value = raw;
      if (typeof window.scanMega === 'function') return window.scanMega();
    }

    let adapter = String(select?.value || 'auto').toLowerCase();
    if (adapter === 'auto' && provider?.adapter && provider.adapter !== 'auto') adapter = provider.adapter;
    if (adapter === 'gallery-dl') {
      const ready = await checkGalleryDL();
      if (ready === false) {
        renderProviderState();
        if (typeof window.toast === 'function') window.toast('gallery-dl lipsește. Folosește butonul de instalare/actualizare de sub link.');
        return;
      }
    }

    const top = document.getElementById('topStatus');
    if (top) top.textContent = provider ? `Analizez ${provider.name}…` : 'Analizez sursa…';
    try {
      const mode = document.getElementById('mode')?.value || 'balanced';
      const data = await jsonFetch('/api/source/scan', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({url:raw, mode, adapter})
      });
      const used = provider?.name || data.adapter || adapter;
      if (typeof window.toast === 'function') window.toast(`${used}: ${Number(data.items || 0).toLocaleString('ro-RO')} fișier(e) comparate`);
      if (typeof window.goTab === 'function') window.goTab('results');
    } catch (err) {
      if (typeof window.toast === 'function') window.toast(err.message);
      else if (top) top.textContent = err.message;
    }
  }

  function installUI() {
    const input = document.getElementById('directUrl');
    if (!input) return;
    input.placeholder = 'GoFile, Bunkr, Cyberdrop, MEGA, link direct, galerie sau pagină video…';
    const section = input.closest('.section');
    const head = section?.querySelector('.sectionHead h3');
    if (head) head.textContent = 'Sursă online — universal';
    const body = input.closest('.sectionBody');
    if (body && !stateBox()) {
      const firstRow = input.nextElementSibling;
      const html = `<div id="universalProviderState" class="providerStateBox"><span class="muted">Lipește un link. DDG va alege automat motorul potrivit.</span></div><div class="providerHostList"><span>GoFile</span><span>Bunkr</span><span>Cyberdrop</span><span>MEGA</span><span>HTTP</span></div>`;
      if (firstRow) firstRow.insertAdjacentHTML('afterend', html);
      else body.insertAdjacentHTML('beforeend', html);
    }
    if (!document.getElementById('providerSourceStyles')) {
      const style = document.createElement('style');
      style.id = 'providerSourceStyles';
      style.textContent = `
        .providerStateBox{margin-top:10px;padding:10px 12px;border:1px solid #2c3c4e;background:#0d151e;border-radius:9px}
        .providerStateLine{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.providerStateHint{margin-top:6px}
        .providerStateBox .btn{margin-top:9px}.providerHostList{display:flex;gap:6px;flex-wrap:wrap;margin-top:8px}
        .providerHostList span{font-size:10px;font-weight:750;color:#a9bed4;border:1px solid #304256;border-radius:999px;padding:3px 7px;background:#101923}
      `;
      document.head.appendChild(style);
    }
    input.addEventListener('input', renderProviderState);
    input.addEventListener('paste', () => setTimeout(renderProviderState, 0));
    window.scanUniversal = universalScanV2;
    renderProviderState();
    checkGalleryDL();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', installUI, {once:true});
  else installUI();
})();