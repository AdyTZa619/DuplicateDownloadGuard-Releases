(() => {
  'use strict';
  const SERVICE = 'ddg-detector-self-test-v901';
  const PORT_BASE = 41450;
  const PORT_SPAN = 200;
  const PORT_ATTEMPTS = 8;
  let timer = 0;
  let serviceURL = '';
  const apiCall = (path, options) => fetch(path, {...options, cache:'no-store'}).then(async r => { if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`); return r.json(); });
  const escText = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

  function fnv1a32(value) {
    let hash = 0x811c9dc5;
    for (const b of new TextEncoder().encode(String(value || '').toLowerCase())) {
      hash ^= b;
      hash = Math.imul(hash, 0x01000193) >>> 0;
    }
    return hash >>> 0;
  }

  function seedVariants(appDir) {
    const clean = String(appDir || '').trim().replace(/[\\/]+$/, '');
    if (!clean) return [];
    const out = [clean];
    if (/[\\/]data$/i.test(clean)) out.push(clean.replace(/[\\/]data$/i, ''));
    else out.push(clean + (clean.includes('\\') ? '\\data' : '/data'));
    return [...new Set(out.map(x => x.toLowerCase()))];
  }

  async function discoverService() {
    if (serviceURL) return serviceURL;
    let about = {};
    try { about = await apiCall('/api/about'); } catch (_) {}
    const bases = seedVariants(about.appDir).map(seed => PORT_BASE + (fnv1a32(seed) % PORT_SPAN));
    if (!bases.length) bases.push(PORT_BASE);
    for (const base of [...new Set(bases)]) {
      for (let i = 0; i < PORT_ATTEMPTS; i++) {
        const candidate = `http://127.0.0.1:${base + i}`;
        try {
          const health = await apiCall(candidate + '/health');
          if (health?.ok && health?.service === SERVICE) {
            serviceURL = candidate;
            return candidate;
          }
        } catch (_) {}
      }
    }
    return '';
  }

  function mount() {
    const list = document.getElementById('helpDiag');
    if (!list || document.getElementById('ddgDetectorSelfTestV901')) return;
    const box = document.createElement('div');
    box.id = 'ddgDetectorSelfTestV901';
    box.className = 'diagItem';
    box.innerHTML = `<span style="flex:1"><b>Autotest detector duplicate</b><br><span class="muted small" id="ddgSelfTestText">Generează automat fișiere identice, recodate și diferite. Nu folosește colecția ta.</span><div id="ddgSelfTestCases" class="muted small" style="margin-top:6px"></div></span><button class="btn" id="ddgSelfTestButton">Rulează autotest</button>`;
    list.parentElement.appendChild(box);
    document.getElementById('ddgSelfTestButton').addEventListener('click', start);
    poll();
  }

  function render(state) {
    const text = document.getElementById('ddgSelfTestText');
    const cases = document.getElementById('ddgSelfTestCases');
    const button = document.getElementById('ddgSelfTestButton');
    if (!text || !button) return;
    button.disabled = !!state.running;
    button.textContent = state.running ? `În lucru ${state.step || 0}/${state.stepTotal || 6}` : 'Rulează autotest';
    if (state.error) text.textContent = `EROARE: ${state.error}`;
    else text.textContent = state.message || 'Pregătit pentru autotest.';
    if (state.report) {
      const r = state.report;
      text.textContent = `${r.passed ? 'TRECUT' : 'EȘUAT'} • ${r.passedCases}/${r.totalCases} cazuri • protecție JD2 ${r.jdGuardPassed ? 'OK' : 'EȘUATĂ'}`;
      cases.innerHTML = (r.cases || []).map(x => `<div>${x.passed ? '✓' : '✗'} ${escText(x.name)}: ${escText(x.actual)} (rece ${x.coldMs} ms, cache ${x.warmMs} ms)</div>`).join('') + `<div>Raport: ${escText(state.reportPath || '')}</div>`;
    }
  }

  async function poll() {
    clearTimeout(timer);
    try {
      const base = await discoverService();
      if (!base) throw new Error('Serviciul de autotest nu este disponibil.');
      const state = await apiCall(base + '/state');
      render(state);
      timer = setTimeout(poll, state.running ? 700 : 5000);
    } catch (_) { timer = setTimeout(poll, 5000); }
  }

  async function start() {
    try {
      const base = await discoverService();
      if (!base) throw new Error('Serviciul de autotest nu este disponibil.');
      const state = await apiCall(base + '/start', {method:'POST'});
      render(state);
      poll();
    } catch (error) { window.toast?.(String(error.message || error)); }
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount);
  else mount();
  window.ddgDetectorSelfTestV901 = {start, poll};
})();
