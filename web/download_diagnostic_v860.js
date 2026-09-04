(() => {
  'use strict';

  const e = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const label = s => ({pass:'PASS',warn:'ATENȚIE',fail:'EROARE',skip:'SĂRIT'})[String(s||'').toLowerCase()] || String(s||'').toUpperCase();

  function installStyles() {
    if (document.getElementById('downloadDiagStylesV860')) return;
    const style = document.createElement('style');
    style.id = 'downloadDiagStylesV860';
    style.textContent = `
      .ddgDiagList{display:flex;flex-direction:column;gap:8px;margin-top:12px}
      .ddgDiagRow{border:1px solid #2b3a4b;border-radius:9px;background:#0d141d;padding:10px 12px}
      .ddgDiagHead{display:flex;align-items:center;justify-content:space-between;gap:10px}
      .ddgDiagHead b{font-size:12px}.ddgDiagBadge{font-size:10px;font-weight:900;border-radius:999px;padding:3px 8px;border:1px solid #3b4d60}
      .ddgDiagRow.pass .ddgDiagBadge{color:#8ee6bd;border-color:#2d6551;background:#10281f}
      .ddgDiagRow.warn .ddgDiagBadge{color:#ffd979;border-color:#6b5a2a;background:#2a2412}
      .ddgDiagRow.fail .ddgDiagBadge{color:#ff9aa5;border-color:#733842;background:#30181d}
      .ddgDiagDetail{margin-top:6px;color:#b5c4d5;font-size:11px;line-height:1.45;white-space:pre-wrap}
      .ddgDiagAction{margin-top:6px;color:#ffd979;font-size:11px}
    `;
    document.head.appendChild(style);
  }

  function installModal() {
    if (document.getElementById('downloadDiagModalV860')) return;
    document.body.insertAdjacentHTML('beforeend', `
      <div class="smartModal hidden" id="downloadDiagModalV860" role="dialog" aria-modal="true">
        <div class="smartBox">
          <div class="smartHead">
            <div><b>🧪 Diagnostic download</b><div class="muted small" id="downloadDiagSubtitleV860">Testează motorul, resume-ul, Referer-ul și sursa selectată.</div></div>
            <button class="btn" onclick="closeDownloadDiagnosticV860()">×</button>
          </div>
          <div class="smartBody">
            <div class="noticeBlue" id="downloadDiagStateV860">Pregătit.</div>
            <div class="ddgDiagList" id="downloadDiagListV860"></div>
          </div>
          <div class="modalFoot">
            <button class="btn" onclick="closeDownloadDiagnosticV860()">Închide</button>
            <button class="btn primary" id="downloadDiagRunV860" onclick="runDownloadDiagnosticV860(true)">Rulează din nou</button>
          </div>
        </div>
      </div>`);
  }

  function installButton() {
    const panel = document.getElementById('downloads');
    const toolbar = panel?.querySelector('.toolbar');
    if (!toolbar || document.getElementById('downloadDiagBtnV860')) return;
    const btn = document.createElement('button');
    btn.className = 'btn';
    btn.id = 'downloadDiagBtnV860';
    btn.textContent = '🧪 Diagnostic download';
    btn.title = 'Verifică motorul de download și, dacă ai un rezultat deschis, probează sursa fără să descarce fișierul complet.';
    btn.onclick = () => runDownloadDiagnosticV860(true);
    toolbar.appendChild(btn);
  }

  function render(report) {
    const overall = String(report?.overall || 'fail').toLowerCase();
    const state = document.getElementById('downloadDiagStateV860');
    if (state) {
      const selected = report?.resultId ? ` • rezultat #${report.resultId} • ${report.source || '?'} → ${report.engine || '?'}` : ' • fără rezultat selectat';
      state.textContent = `${overall === 'pass' ? '✓ Tot ce s-a testat este funcțional' : overall === 'warn' ? '⚠ Funcțional, dar există atenționări' : '✕ Diagnosticul a găsit o problemă'}${selected}`;
      state.style.borderLeftColor = overall === 'pass' ? '#3ddc97' : overall === 'warn' ? '#ffcc66' : '#ff6b7a';
    }
    const list = document.getElementById('downloadDiagListV860');
    if (!list) return;
    const rows = Array.isArray(report?.checks) ? report.checks : [];
    list.innerHTML = rows.map(c => {
      const status = String(c.status || 'fail').toLowerCase();
      const ms = Number(c.durationMs || 0);
      return `<div class="ddgDiagRow ${e(status)}"><div class="ddgDiagHead"><b>${e(c.name)}</b><span class="ddgDiagBadge">${e(label(status))}${ms ? ` • ${ms} ms` : ''}</span></div><div class="ddgDiagDetail">${e(c.detail || '—')}</div>${c.action ? `<div class="ddgDiagAction">Ce faci: ${e(c.action)}</div>` : ''}</div>`;
    }).join('') || '<div class="muted">Nu a fost returnat niciun test.</div>';
  }

  window.closeDownloadDiagnosticV860 = function() {
    document.getElementById('downloadDiagModalV860')?.classList.add('hidden');
  };

  window.runDownloadDiagnosticV860 = async function(includeNetwork = true) {
    installModal();
    const modal = document.getElementById('downloadDiagModalV860');
    modal?.classList.remove('hidden');
    const state = document.getElementById('downloadDiagStateV860');
    const list = document.getElementById('downloadDiagListV860');
    const run = document.getElementById('downloadDiagRunV860');
    if (state) state.textContent = 'Rulez testele end-to-end…';
    if (list) list.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Verific downloaderul și sursa…</b></div>';
    if (run) run.disabled = true;
    try {
      const id = Number(currentRow?.id || 0);
      const report = await api('/api/download/diagnostic', {
        method: 'POST',
        headers: {'Content-Type':'application/json'},
        body: JSON.stringify({id, includeNetwork: !!includeNetwork})
      });
      render(report);
    } catch (err) {
      if (state) state.textContent = 'Diagnosticul nu a putut porni.';
      if (list) list.innerHTML = `<div class="ddgDiagRow fail"><div class="ddgDiagDetail">${e(err.message || err)}</div></div>`;
    } finally {
      if (run) run.disabled = false;
    }
  };

  function boot() {
    installStyles();
    installModal();
    installButton();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
})();