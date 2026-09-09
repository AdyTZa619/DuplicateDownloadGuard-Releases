// Useful diagnostics layered onto the existing Operation HUD.
// Keeps the popup compact but adds information that helps during real DDG use
// and makes stale/offline windows actionable after an updater restart.
(() => {
  'use strict';

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  let installed = false;
  let versionText = '—';
  let indexText = '—';
  let backendOK = true;
  let lastDiagRefresh = 0;

  function installStyles() {
    if (document.getElementById('operationHudPlusStyles')) return;
    const style = document.createElement('style');
    style.id = 'operationHudPlusStyles';
    style.textContent = `
      .operationHudUseful{display:grid;grid-template-columns:1fr 1fr;gap:7px;margin-top:9px}
      .operationHudUsefulCard{border:1px solid #253749;background:rgba(10,18,26,.72);border-radius:9px;padding:8px 9px;min-width:0}
      .operationHudUsefulCard span{display:block;color:#718aa2;font-size:8.5px;font-weight:800;letter-spacing:.08em;text-transform:uppercase}
      .operationHudUsefulCard b{display:block;margin-top:3px;font-size:10.5px;line-height:1.35;color:#dce9f6;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
      .operationHudRecovery{margin-top:9px;padding:9px 10px;border-left:3px solid #ff6b7a;background:#2a141b;border-radius:7px;color:#ffc1c7;font-size:10.5px;line-height:1.45}
      .operationHudRecovery.hidden{display:none}
      .operationHudFoot{flex-wrap:wrap}
      @media(max-width:650px){.operationHudUseful{grid-template-columns:1fr}}
    `;
    document.head.appendChild(style);
  }

  function journalLatest() {
    try {
      const rows = window.ddgCheckActivityV8563?.readJournal?.() || [];
      return Array.isArray(rows) && rows.length ? rows[rows.length - 1] : null;
    } catch (_) { return null; }
  }

  function fmtLastCheck(row) {
    if (!row) return 'Nicio verificare încă';
    const provider = String(row.provider || row.kind || 'LINK').toUpperCase();
    const status = String(row.status || '—');
    const ms = Number(row.ms || 0);
    return `${provider} • ${status}${ms > 0 ? ` • ${(ms/1000).toFixed(1)}s` : ''}`;
  }

  function updateCards() {
    const version = document.getElementById('operationHudPlusVersion');
    const index = document.getElementById('operationHudPlusIndex');
    const backend = document.getElementById('operationHudPlusBackend');
    const last = document.getElementById('operationHudPlusLast');
    if (version) version.textContent = versionText;
    if (index) index.textContent = indexText;
    if (backend) {
      backend.textContent = `${location.host || 'localhost'} • ${backendOK ? 'OK' : 'OFFLINE'}`;
      backend.title = location.href;
    }
    if (last) {
      const row = journalLatest();
      last.textContent = fmtLastCheck(row);
      last.title = row?.url || row?.detail || '';
    }
  }

  function updateRecovery() {
    const badge = document.getElementById('operationHudBadge');
    const recovery = document.getElementById('operationHudRecovery');
    if (!recovery) return;
    const offline = !backendOK || String(badge?.textContent || '').trim().toUpperCase() === 'OFFLINE';
    recovery.classList.toggle('hidden', !offline);
    if (offline) {
      recovery.innerHTML = '<b>Fereastră fără backend local.</b> Dacă a apărut după un update, aceasta este probabil fereastra veche. Închide fereastra DDG și pornește din nou EXE-ul TEST actual; nu trebuie șterse datele programului.';
      const next = document.getElementById('operationHudNext');
      if (next) next.textContent = 'Backendul acestei ferestre nu mai răspunde. După update, redeschiderea DDG este recuperarea corectă.';
    }
    updateCards();
  }

  async function refreshDiagnostics(force = false) {
    const now = Date.now();
    if (!force && now - lastDiagRefresh < 15000) {
      updateRecovery();
      return;
    }
    lastDiagRefresh = now;
    try {
      const [aboutResp, indexResp] = await Promise.all([
        fetch('/api/about?_hudplus=' + now, {cache:'no-store'}),
        fetch('/api/index/stats?_hudplus=' + now, {cache:'no-store'})
      ]);
      if (!aboutResp.ok || !indexResp.ok) throw new Error('backend offline');
      const [about, index] = await Promise.all([aboutResp.json(), indexResp.json()]);
      versionText = String(about?.version || '—');
      const files = Number(index?.files || 0);
      indexText = `${files.toLocaleString('ro-RO')} fiș.${index?.size ? ` • ${index.size}` : ''}`;
      backendOK = true;
    } catch (_) {
      backendOK = false;
    }
    updateRecovery();
  }

  async function copyDiagnostic() {
    const latest = journalLatest();
    const badge = document.getElementById('operationHudBadge')?.textContent?.trim() || '—';
    const message = document.getElementById('operationHudMessage')?.textContent?.trim() || '—';
    const text = [
      `DDG ${versionText}`,
      `Backend: ${location.href} • ${backendOK ? 'OK' : 'OFFLINE'}`,
      `Monitor: ${badge} • ${message}`,
      `Index: ${indexText}`,
      `Ultima verificare: ${fmtLastCheck(latest)}`,
      latest?.url ? `Link: ${latest.url}` : ''
    ].filter(Boolean).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      window.toast?.('Diagnosticul monitorului a fost copiat');
    } catch (_) {
      window.toast?.('Nu am putut copia automat diagnosticul');
    }
  }

  function install() {
    if (installed) return true;
    const panel = document.getElementById('operationHudPanel');
    const metrics = panel?.querySelector('.operationHudMetrics');
    const foot = panel?.querySelector('.operationHudFoot');
    if (!panel || !metrics || !foot) return false;

    installStyles();
    metrics.insertAdjacentHTML('afterend', `
      <div class="operationHudUseful" id="operationHudUseful">
        <div class="operationHudUsefulCard"><span>Versiune</span><b id="operationHudPlusVersion">—</b></div>
        <div class="operationHudUsefulCard"><span>Backend local</span><b id="operationHudPlusBackend">—</b></div>
        <div class="operationHudUsefulCard"><span>Index local</span><b id="operationHudPlusIndex">—</b></div>
        <div class="operationHudUsefulCard"><span>Ultima verificare</span><b id="operationHudPlusLast">Nicio verificare încă</b></div>
      </div>
      <div class="operationHudRecovery hidden" id="operationHudRecovery"></div>`);

    if (!foot.querySelector('[data-hud-plus="results"]')) {
      const results = document.createElement('button');
      results.className = 'operationHudMiniBtn';
      results.type = 'button';
      results.dataset.hudPlus = 'results';
      results.textContent = 'Rezultate';
      results.addEventListener('click', () => window.goTab?.('results'));
      foot.appendChild(results);
    }
    if (!foot.querySelector('[data-hud-plus="checks"]')) {
      const checks = document.createElement('button');
      checks.className = 'operationHudMiniBtn';
      checks.type = 'button';
      checks.dataset.hudPlus = 'checks';
      checks.textContent = 'Verificări';
      checks.addEventListener('click', () => window.ddgCheckActivityV8563?.openJournal?.());
      foot.appendChild(checks);
    }
    if (!foot.querySelector('[data-hud-plus="copy"]')) {
      const copy = document.createElement('button');
      copy.className = 'operationHudMiniBtn';
      copy.type = 'button';
      copy.dataset.hudPlus = 'copy';
      copy.textContent = 'Copiază diagnostic';
      copy.addEventListener('click', copyDiagnostic);
      foot.appendChild(copy);
    }

    const badge = document.getElementById('operationHudBadge');
    if (badge) new MutationObserver(updateRecovery).observe(badge, {childList:true,subtree:true,characterData:true});
    installed = true;
    refreshDiagnostics(true);
    setInterval(() => refreshDiagnostics(false), 2500);
    return true;
  }

  function boot() {
    if (!install()) setTimeout(boot, 150);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();

  window.ddgOperationHudPlusV8565 = {refresh: () => refreshDiagnostics(true), copyDiagnostic};
})();
