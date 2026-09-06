(() => {
  'use strict';

  const TEST_MANIFEST_URL = 'https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/testing/update-test.json';
  const CORNER_ID = 'ddgUpdateCorner';
  const TEST_BOX_ID = 'ddgTestUpdaterBox';
  let stableState = null;
  let testState = null;
  let currentVersion = '';
  let checking = false;

  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

  function addStyles() {
    if (document.getElementById('ddgUpdateChannelsStyle')) return;
    const style = document.createElement('style');
    style.id = 'ddgUpdateChannelsStyle';
    style.textContent = `
      #${CORNER_ID}{display:inline-flex;align-items:center;gap:7px;margin-right:10px;vertical-align:middle}
      #${CORNER_ID} .ddgUpdateChip{border:1px solid #3c5066;background:#172131;color:#eaf2ff;padding:7px 10px;border-radius:8px;cursor:pointer;font-weight:750;line-height:1}
      #${CORNER_ID} .ddgUpdateChip:hover{border-color:#4da3ff}
      #${CORNER_ID} .ddgUpdateChip.stable{border-color:#2d6f58;background:#10281f;color:#8ff0c8}
      #${CORNER_ID} .ddgUpdateChip.test{border-color:#6d57a2;background:#211a32;color:#d2c0ff}
      #${CORNER_ID} .ddgUpdateChip.busy{opacity:.65;cursor:wait}
      .ddgTestUpdater{margin-top:14px;border:1px solid #4d3f72;background:#171326;border-radius:10px;padding:13px}
      .ddgTestUpdaterHead{display:flex;align-items:center;gap:8px;margin-bottom:7px}
      .ddgTestBadge{display:inline-flex;padding:3px 6px;border:1px solid #6d57a2;border-radius:999px;background:#2d244b;color:#d8caff;font-size:10px;font-weight:800;letter-spacing:.08em}
      .ddgTestActions{display:flex;gap:8px;flex-wrap:wrap;margin-top:10px}
    `;
    document.head.appendChild(style);
  }

  function ensureCorner() {
    addStyles();
    let box = document.getElementById(CORNER_ID);
    if (box) return box;
    const status = document.getElementById('topStatus');
    if (!status || !status.parentElement) return null;
    box = document.createElement('span');
    box.id = CORNER_ID;
    status.parentElement.insertBefore(box, status.parentElement.firstChild);
    return box;
  }

  function ensureTestBox() {
    if (document.getElementById(TEST_BOX_ID)) return;
    const stableText = document.getElementById('updateStatusText');
    if (!stableText || !stableText.parentElement) return;
    const box = document.createElement('div');
    box.id = TEST_BOX_ID;
    box.className = 'ddgTestUpdater';
    box.innerHTML = `
      <div class="ddgTestUpdaterHead"><span class="ddgTestBadge">TEST</span><b>Canal separat pentru versiuni de probă</b></div>
      <div class="muted small">Build-urile TEST sunt separate de Stable. Le poți instala pentru a verifica un fix înainte să fie publicat normal. Instalarea folosește același updater local cu backup, health-check și rollback.</div>
      <div class="ddgTestActions"><button class="btn" id="ddgCheckTestUpdate">↻ Verifică TEST</button><button class="btn" id="ddgInstallTestUpdate">⬇ Instalează TEST</button></div>
      <div class="muted small" id="ddgTestUpdateStatus" style="margin-top:8px">Nu a fost verificat încă.</div>`;
    stableText.parentElement.appendChild(box);
    document.getElementById('ddgCheckTestUpdate').onclick = () => checkTest(false);
    document.getElementById('ddgInstallTestUpdate').onclick = () => installTest();
  }

  function parseVersion(value) {
    const m = String(value || '').match(/(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?/i);
    if (!m) return null;
    return {core: [+m[1], +m[2], +m[3]], pre: m[4] ? m[4].toLowerCase().split('.') : []};
  }

  function comparePre(a, b) {
    if (!a.length && !b.length) return 0;
    if (!a.length) return 1;
    if (!b.length) return -1;
    for (let i = 0; i < Math.min(a.length, b.length); i++) {
      if (a[i] === b[i]) continue;
      const an = /^\d+$/.test(a[i]);
      const bn = /^\d+$/.test(b[i]);
      if (an && bn) return Number(a[i]) > Number(b[i]) ? 1 : -1;
      if (an !== bn) return an ? -1 : 1;
      return a[i] > b[i] ? 1 : -1;
    }
    return a.length === b.length ? 0 : (a.length > b.length ? 1 : -1);
  }

  function isNewer(remote, local) {
    const a = parseVersion(remote);
    const b = parseVersion(local);
    if (!a || !b) return false;
    for (let i = 0; i < 3; i++) {
      if (a.core[i] !== b.core[i]) return a.core[i] > b.core[i];
    }
    return comparePre(a.pre, b.pre) > 0;
  }

  async function localVersion() {
    if (currentVersion) return currentVersion;
    const info = await window.api('/api/about');
    currentVersion = info.version || '';
    return currentVersion;
  }

  async function checkStable(silent = true) {
    try {
      stableState = await window.api('/api/update/check');
      renderCorner();
      if (stableState?.newer && !silent && typeof window.toast === 'function') {
        window.toast(`Update Stable disponibil: ${stableState.manifest.version}`);
      }
      return stableState;
    } catch (_) {
      stableState = null;
      renderCorner();
      return null;
    }
  }

  async function fetchTestManifest() {
    const sep = TEST_MANIFEST_URL.includes('?') ? '&' : '?';
    const response = await fetch(`${TEST_MANIFEST_URL}${sep}_ddg=${Date.now()}`, {
      cache: 'no-store',
      headers: {'Cache-Control': 'no-cache'}
    });
    if (!response.ok) throw new Error(`manifest TEST HTTP ${response.status}`);
    const manifest = await response.json();
    if (!manifest?.version || !manifest?.url || !manifest?.sha256) {
      throw new Error('manifest TEST incomplet');
    }
    return manifest;
  }

  async function checkTest(silent = true) {
    const text = document.getElementById('ddgTestUpdateStatus');
    try {
      const [manifest, installed] = await Promise.all([fetchTestManifest(), localVersion()]);
      testState = {configured: true, manifest, newer: isNewer(manifest.version, installed)};
      if (text) {
        text.textContent = testState.newer
          ? `TEST disponibil: ${manifest.version} — ${manifest.notes || ''}`
          : `Niciun TEST mai nou • ${manifest.version}`;
      }
      renderCorner();
      if (testState.newer && !silent && typeof window.toast === 'function') {
        window.toast(`Versiune TEST disponibilă: ${manifest.version}`);
      }
      return testState;
    } catch (err) {
      testState = null;
      if (text) text.textContent = `Canal TEST indisponibil momentan: ${err.message}`;
      renderCorner();
      return null;
    }
  }

  function renderCorner() {
    const box = ensureCorner();
    if (!box) return;
    const html = [];
    if (stableState?.newer) {
      html.push(`<button class="ddgUpdateChip stable" data-channel="stable" title="Instalează update-ul Stable">↑ Update ${stableState.manifest.version}</button>`);
    }
    if (testState?.newer) {
      html.push(`<button class="ddgUpdateChip test" data-channel="test" title="Instalează versiunea TEST">TEST ${testState.manifest.version}</button>`);
    }
    box.innerHTML = html.join('');
    box.querySelectorAll('[data-channel="stable"]').forEach(btn => btn.onclick = () => installStable(btn));
    box.querySelectorAll('[data-channel="test"]').forEach(btn => btn.onclick = () => installTest(btn));
  }

  async function installStable(button) {
    const version = stableState?.manifest?.version || 'Stable';
    if (!confirm(`Instalez DDG ${version} Stable? Aplicația se va închide și reporni automat.`)) return;
    if (button) {
      button.disabled = true;
      button.classList.add('busy');
      button.textContent = 'Se instalează…';
    }
    try {
      await window.api('/api/update/install-online', {method: 'POST'});
    } catch (err) {
      if (typeof window.toast === 'function') window.toast('Update Stable: ' + err.message);
      if (button) {
        button.disabled = false;
        button.classList.remove('busy');
        button.textContent = `↑ Update ${version}`;
      }
    }
  }

  async function sha256Hex(buffer) {
    const digest = await crypto.subtle.digest('SHA-256', buffer);
    return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('');
  }

  async function installTest(button) {
    try {
      if (!testState?.newer) await checkTest(false);
      if (!testState?.newer) {
        if (typeof window.toast === 'function') window.toast('Nu există un build TEST mai nou.');
        return;
      }
      const manifest = testState.manifest;
      if (!confirm(`Instalez DDG ${manifest.version} din canalul TEST?\n\nEste un build de probă. Updaterul păstrează backup și face rollback automat dacă noua versiune nu pornește.`)) return;
      const btn = button || document.getElementById('ddgInstallTestUpdate');
      if (btn) {
        btn.disabled = true;
        btn.classList.add('busy');
        btn.textContent = 'Descarc TEST…';
      }
      const response = await fetch(manifest.url, {cache: 'no-store'});
      if (!response.ok) throw new Error(`download TEST HTTP ${response.status}`);
      const buffer = await response.arrayBuffer();
      if (buffer.byteLength > 100 * 1024 * 1024) throw new Error('build TEST prea mare (>100 MB)');
      const got = await sha256Hex(buffer);
      if (got.toLowerCase() !== String(manifest.sha256).trim().toLowerCase()) {
        throw new Error('SHA-256 TEST nu corespunde manifestului; instalarea a fost refuzată');
      }
      if (btn) btn.textContent = 'Aplic TEST…';
      const form = new FormData();
      form.append('file', new Blob([buffer], {type: 'application/vnd.microsoft.portable-executable'}), `DuplicateDownloadGuard_TEST_${manifest.version}.exe`);
      const apply = await fetch('/api/update/apply', {method: 'POST', body: form});
      if (!apply.ok) throw new Error((await apply.text()).trim() || `updater local HTTP ${apply.status}`);
      await apply.json();
    } catch (err) {
      if (typeof window.toast === 'function') window.toast('Update TEST: ' + err.message);
      const btn = button || document.getElementById('ddgInstallTestUpdate');
      if (btn) {
        btn.disabled = false;
        btn.classList.remove('busy');
        btn.textContent = '⬇ Instalează TEST';
      }
    }
  }

  async function checkAll() {
    if (checking || typeof window.api !== 'function') return;
    checking = true;
    try {
      await Promise.all([checkStable(true), checkTest(true)]);
    } finally {
      checking = false;
    }
  }

  async function boot() {
    for (let i = 0; i < 40 && typeof window.api !== 'function'; i++) await sleep(100);
    if (typeof window.api !== 'function') return;
    ensureCorner();
    ensureTestBox();
    setTimeout(checkAll, 1400);
    setInterval(checkAll, 10 * 60 * 1000);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, {once: true});
  } else {
    boot();
  }

  window.ddgCheckAllUpdateChannels = checkAll;
})();
