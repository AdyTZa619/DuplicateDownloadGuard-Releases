from pathlib import Path

HUD_MARKER = "// Operation HUD v8.5.1"

HUD_MODULE = r'''

// Operation HUD v8.5.1 — immersive live status in the top-right corner.
// It deliberately consumes only the existing local /api/status endpoint.
(() => {
  'use strict';

  const STORE_KEY = 'ddg.operationHud.v851';
  let hudTimer = null;
  let hudOpen = false;
  let lastSnapshot = null;
  let consecutiveFailures = 0;

  const el = id => document.getElementById(id);

  function injectStyles() {
    if (el('operationHudStyles')) return;
    const style = document.createElement('style');
    style.id = 'operationHudStyles';
    style.textContent = `
      .operationHudWrap{position:relative;display:flex;align-items:center;justify-content:flex-end;min-width:310px;max-width:min(520px,48vw)}
      .operationHudCompact{appearance:none;border:1px solid rgba(90,121,153,.28);background:linear-gradient(135deg,rgba(17,28,40,.78),rgba(12,19,28,.68));color:var(--text);border-radius:12px;padding:7px 10px 7px 9px;display:grid;grid-template-columns:12px minmax(0,1fr) 18px;gap:8px;align-items:center;min-width:310px;max-width:520px;cursor:pointer;text-align:left;box-shadow:0 7px 24px rgba(0,0,0,.2),inset 0 1px 0 rgba(255,255,255,.025);backdrop-filter:blur(15px);transition:border-color .18s ease,background .18s ease,box-shadow .18s ease,transform .18s ease}
      .operationHudCompact:hover,.operationHudCompact[aria-expanded="true"]{border-color:rgba(77,163,255,.62);background:linear-gradient(135deg,rgba(21,39,57,.94),rgba(14,24,35,.92));box-shadow:0 10px 34px rgba(0,0,0,.32),0 0 0 1px rgba(77,163,255,.08)}
      .operationHudCompact:active{transform:translateY(1px)}
      .operationHudText{min-width:0;display:flex;flex-direction:column;gap:1px}.operationHudText strong{font-size:12.5px;line-height:1.2;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.operationHudText small{font-size:10.5px;color:#91a7bd;line-height:1.2;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
      .operationHudChevron{color:#7990a7;font-size:13px;text-align:center;transition:transform .18s ease}.operationHudCompact[aria-expanded="true"] .operationHudChevron{transform:rotate(180deg)}
      .operationHudWrap .statusDot{width:9px;height:9px;margin:0;background:#607489;box-shadow:0 0 0 3px rgba(96,116,137,.10);transition:background .2s ease,box-shadow .2s ease}
      .operationHudWrap[data-state="running"] .statusDot{background:#4da3ff;box-shadow:0 0 0 3px rgba(77,163,255,.14),0 0 14px rgba(77,163,255,.60);animation:operationHudPulse 1.35s ease-in-out infinite}
      .operationHudWrap[data-state="success"] .statusDot{background:#3ddc97;box-shadow:0 0 0 3px rgba(61,220,151,.12),0 0 12px rgba(61,220,151,.34)}
      .operationHudWrap[data-state="error"] .statusDot{background:#ff6b7a;box-shadow:0 0 0 3px rgba(255,107,122,.13),0 0 13px rgba(255,107,122,.38)}
      .operationHudWrap[data-state="cancelled"] .statusDot{background:#ffcc66;box-shadow:0 0 0 3px rgba(255,204,102,.13)}
      @keyframes operationHudPulse{0%,100%{transform:scale(.88);opacity:.78}50%{transform:scale(1.12);opacity:1}}
      .operationHudPanel{position:absolute;right:0;top:calc(100% + 10px);width:min(430px,calc(100vw - 92px));background:linear-gradient(160deg,rgba(17,27,39,.985),rgba(9,15,22,.985));border:1px solid #31465b;border-radius:14px;box-shadow:0 28px 80px rgba(0,0,0,.58),inset 0 1px 0 rgba(255,255,255,.025);overflow:hidden;opacity:0;visibility:hidden;transform:translateY(-6px) scale(.985);transform-origin:top right;transition:opacity .16s ease,transform .16s ease,visibility .16s;z-index:80;backdrop-filter:blur(20px)}
      .operationHudPanel.on{opacity:1;visibility:visible;transform:translateY(0) scale(1)}
      .operationHudHead{padding:13px 14px 11px;border-bottom:1px solid rgba(62,83,105,.6);display:flex;align-items:flex-start;justify-content:space-between;gap:12px;background:linear-gradient(90deg,rgba(77,163,255,.075),transparent 62%)}
      .operationHudEyebrow{color:#7f97ae;font-size:9px;font-weight:800;letter-spacing:.12em;text-transform:uppercase}.operationHudHead b{display:block;margin-top:3px;font-size:14px;line-height:1.3}.operationHudBadge{font-size:9px;font-weight:900;letter-spacing:.08em;border:1px solid #3b536a;border-radius:999px;padding:4px 7px;white-space:nowrap;background:#111d29;color:#bcd2e7}
      .operationHudWrap[data-state="running"] .operationHudBadge{border-color:#315f8d;color:#82c4ff;background:#10243a}.operationHudWrap[data-state="success"] .operationHudBadge{border-color:#2d6650;color:#82edbd;background:#10281f}.operationHudWrap[data-state="error"] .operationHudBadge{border-color:#79404a;color:#ff9da7;background:#311820}.operationHudWrap[data-state="cancelled"] .operationHudBadge{border-color:#725d2e;color:#ffdb86;background:#2c2413}
      .operationHudBody{padding:13px 14px 12px}.operationHudMessage{font-size:12.5px;font-weight:700;line-height:1.42;color:#e5effa;word-break:break-word}.operationHudDetail{margin-top:5px;color:#9eb0c3;font-size:11px;line-height:1.45;max-height:48px;overflow:auto;word-break:break-word}
      .operationHudProgress{height:5px;background:#121d28;border:1px solid #233446;border-radius:999px;overflow:hidden;margin:12px 0 11px}.operationHudProgress i{display:block;height:100%;width:0;background:linear-gradient(90deg,#4da3ff,#8b5cf6 55%,#3ddc97);border-radius:999px;transition:width .35s ease}.operationHudWrap[data-state="running"] .operationHudProgress.indeterminate i{width:34%!important;animation:operationHudMove 1.2s linear infinite}.operationHudProgress:not(.indeterminate) i{animation:none}@keyframes operationHudMove{from{transform:translateX(-120%)}to{transform:translateX(390%)}}
      .operationHudMetrics{display:grid;grid-template-columns:1fr 1fr;gap:7px}.operationHudMetric{border:1px solid #253749;background:rgba(10,18,26,.72);border-radius:9px;padding:8px 9px;min-width:0}.operationHudMetric span{display:block;color:#718aa2;font-size:8.5px;font-weight:800;letter-spacing:.08em;text-transform:uppercase}.operationHudMetric b{display:block;margin-top:3px;font-size:11.5px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#dce9f6}
      .operationHudNext{margin-top:9px;padding:8px 9px;border-left:2px solid #4da3ff;background:#0e1b28;border-radius:7px;color:#a9c3db;font-size:10.5px;line-height:1.4}.operationHudWrap[data-state="error"] .operationHudNext{border-left-color:#ff6b7a;background:#28141a;color:#efb7bd}.operationHudWrap[data-state="success"] .operationHudNext{border-left-color:#3ddc97;background:#10231c;color:#a8d9c4}
      .operationHudFoot{display:flex;gap:6px;justify-content:flex-end;padding:9px 12px;border-top:1px solid rgba(62,83,105,.55);background:#0b121a}.operationHudMiniBtn{border:1px solid #2d4257;background:#111d29;color:#adc4db;border-radius:7px;padding:5px 8px;font-size:10px;cursor:pointer}.operationHudMiniBtn:hover{border-color:#4d769d;color:#fff;background:#152638}
      @media(max-width:900px){.operationHudWrap{min-width:0;max-width:58vw}.operationHudCompact{min-width:0;width:min(360px,58vw)}.operationHudText small{display:none}}
      @media(max-width:650px){.operationHudWrap{max-width:46vw}.operationHudCompact{width:46vw;padding:7px}.operationHudText strong{font-size:11px}.operationHudPanel{position:fixed;right:10px;left:82px;top:69px;width:auto}.operationHudChevron{display:none}}
    `;
    document.head.appendChild(style);
  }

  function stateName(p) {
    const state = String(p?.state || (p?.active ? 'running' : 'idle')).toLowerCase();
    if (p?.active) return 'running';
    return ['success', 'error', 'cancelled'].includes(state) ? state : 'idle';
  }

  function stateLabel(state) {
    return ({running:'ÎN LUCRU',success:'REUȘIT',error:'EROARE',cancelled:'ANULAT',idle:'PREGĂTIT'})[state] || String(state || '').toUpperCase();
  }

  function phaseLabel(phase) {
    const raw = String(phase || '').trim();
    if (!raw) return 'Sistem';
    const key = raw.toLowerCase();
    const known = {
      index:'Index local',mega:'MEGA',compare:'Comparare',comparison:'Comparare',source:'Sursă remote',
      universal:'Sursă universală',batch:'Lot surse',download:'Descărcare',guard:'Smart Guard',verify:'Verificare'
    };
    return known[key] || raw.replace(/[-_]+/g,' ').replace(/\b\w/g,c=>c.toUpperCase());
  }

  function formatDuration(seconds) {
    seconds = Math.max(0, Math.floor(Number(seconds) || 0));
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = seconds % 60;
    if (h) return `${h}h ${String(m).padStart(2,'0')}m ${String(s).padStart(2,'0')}s`;
    if (m) return `${m}m ${String(s).padStart(2,'0')}s`;
    return `${s}s`;
  }

  function relativeTime(seconds) {
    seconds = Math.max(0, Math.floor(Number(seconds) || 0));
    if (seconds < 5) return 'chiar acum';
    if (seconds < 60) return `acum ${seconds}s`;
    const m = Math.floor(seconds / 60);
    if (m < 60) return `acum ${m}m`;
    const h = Math.floor(m / 60);
    return `acum ${h}h`;
  }

  function localTime(epochSeconds) {
    if (!epochSeconds) return '—';
    try { return new Date(epochSeconds * 1000).toLocaleTimeString('ro-RO',{hour:'2-digit',minute:'2-digit',second:'2-digit'}); }
    catch (_) { return '—'; }
  }

  function compactProcessed(p) {
    const parts = [];
    if (Number(p?.files || 0) > 0) parts.push(`${Number(p.files).toLocaleString('ro-RO')} fiș.`);
    if (Number(p?.bytes || 0) > 0 && typeof fmt === 'function') parts.push(fmt(p.bytes));
    return parts.join(' • ');
  }

  function readFinishRecord() {
    try { return JSON.parse(localStorage.getItem(STORE_KEY) || 'null'); } catch (_) { return null; }
  }

  function rememberFinish(p, state) {
    if (!p?.startedAt || !['success','error','cancelled'].includes(state)) return readFinishRecord();
    const key = `${p.phase || ''}|${p.startedAt}|${state}`;
    let record = readFinishRecord();
    if (!record || record.key !== key) {
      record = {key, finishedAt: Math.floor(Date.now()/1000), startedAt:Number(p.startedAt)||0, state};
      try { localStorage.setItem(STORE_KEY, JSON.stringify(record)); } catch (_) {}
    }
    return record;
  }

  function clearOldFinishIfNeeded(p, state) {
    if (state !== 'idle') return;
    try { localStorage.removeItem(STORE_KEY); } catch (_) {}
  }

  function progressPercent(p) {
    if (Number(p?.total || 0) > 0) return Math.max(0,Math.min(100,Number(p.current || 0)/Number(p.total)*100));
    if (Number(p?.stepTotal || 0) > 0) return Math.max(0,Math.min(100,Number(p.step || 0)/Number(p.stepTotal)*100));
    return null;
  }

  function nextAction(p, state) {
    if (state === 'running') return p?.canCancel ? 'Operația rulează. Poți continua să folosești interfața sau o poți anula din Dashboard.' : 'Operația rulează și își finalizează etapa curentă.';
    if (state === 'success') return 'Operația s-a terminat corect. Poți continua cu următoarea verificare sau deschide rezultatele.';
    if (state === 'error') return 'Operația s-a oprit. Deschide Jurnal pentru mesajul complet și pasul la care a apărut problema.';
    if (state === 'cancelled') return 'Operația a fost anulată. O poți relua când dorești.';
    return 'Sistem pregătit. Nu există nicio operație activă.';
  }

  function installHud() {
    if (el('operationHud')) return true;
    const topStatus = el('topStatus');
    const top = document.querySelector('.top');
    if (!topStatus || !top) return false;
    const host = topStatus.parentElement;
    if (!host) return false;
    host.className = 'operationHudWrap';
    host.id = 'operationHud';
    host.dataset.state = 'idle';
    host.innerHTML = `
      <button class="operationHudCompact" id="operationHudToggle" type="button" aria-expanded="false" aria-controls="operationHudPanel" title="Detalii operație">
        <span class="statusDot" aria-hidden="true"></span>
        <span class="operationHudText"><strong id="topStatus">Pregătit</strong><small id="operationHudSub">Monitor operațional • gata</small></span>
        <span class="operationHudChevron" aria-hidden="true">⌄</span>
      </button>
      <div class="operationHudPanel" id="operationHudPanel" role="dialog" aria-label="Monitor operațional">
        <div class="operationHudHead"><div><span class="operationHudEyebrow">Monitor operațional live</span><b id="operationHudPanelTitle">Sistem pregătit</b></div><span class="operationHudBadge" id="operationHudBadge">PREGĂTIT</span></div>
        <div class="operationHudBody">
          <div class="operationHudMessage" id="operationHudMessage">Nicio operație activă.</div>
          <div class="operationHudDetail" id="operationHudDetail"></div>
          <div class="operationHudProgress" id="operationHudProgress"><i id="operationHudProgressBar"></i></div>
          <div class="operationHudMetrics">
            <div class="operationHudMetric"><span>Etapă</span><b id="operationHudStep">—</b></div>
            <div class="operationHudMetric"><span>Durată</span><b id="operationHudDuration">—</b></div>
            <div class="operationHudMetric"><span>Procesat</span><b id="operationHudProcessed">—</b></div>
            <div class="operationHudMetric"><span>Moment</span><b id="operationHudClock">—</b></div>
          </div>
          <div class="operationHudNext" id="operationHudNext">Sistem pregătit.</div>
        </div>
        <div class="operationHudFoot"><button class="operationHudMiniBtn" type="button" data-hud-tab="dashboard">Dashboard</button><button class="operationHudMiniBtn" type="button" data-hud-tab="logs">Jurnal</button></div>
      </div>`;

    const toggle = el('operationHudToggle');
    toggle.addEventListener('click', event => {
      event.stopPropagation();
      hudOpen = !hudOpen;
      toggle.setAttribute('aria-expanded', hudOpen ? 'true' : 'false');
      el('operationHudPanel')?.classList.toggle('on', hudOpen);
    });
    el('operationHudPanel')?.addEventListener('click', event => event.stopPropagation());
    host.querySelectorAll('[data-hud-tab]').forEach(button => button.addEventListener('click', () => {
      if (typeof goTab === 'function') goTab(button.dataset.hudTab);
      hudOpen = false;
      toggle.setAttribute('aria-expanded','false');
      el('operationHudPanel')?.classList.remove('on');
    }));
    document.addEventListener('click', () => {
      if (!hudOpen) return;
      hudOpen = false;
      toggle.setAttribute('aria-expanded','false');
      el('operationHudPanel')?.classList.remove('on');
    });
    document.addEventListener('keydown', event => {
      if (event.key !== 'Escape' || !hudOpen) return;
      hudOpen = false;
      toggle.setAttribute('aria-expanded','false');
      el('operationHudPanel')?.classList.remove('on');
      toggle.focus();
    });
    return true;
  }

  function render(p) {
    if (!installHud()) return;
    lastSnapshot = p || {};
    const state = stateName(p);
    const host = el('operationHud');
    if (host) host.dataset.state = state;
    const phase = phaseLabel(p?.phase);
    const now = Math.floor(Date.now()/1000);
    let finish = null;
    if (!p?.active && ['success','error','cancelled'].includes(state)) finish = rememberFinish(p,state);
    clearOldFinishIfNeeded(p,state);
    const end = p?.active ? now : (finish?.finishedAt || now);
    const duration = p?.startedAt ? Math.max(0,end-Number(p.startedAt)) : 0;
    const finishedAgo = finish?.finishedAt ? relativeTime(now-finish.finishedAt) : '';
    const pct = progressPercent(p);
    const stepText = Number(p?.stepTotal||0)>0 ? `${Number(p.step||0)}/${Number(p.stepTotal)}` : (p?.phase ? phase : '—');
    const processed = compactProcessed(p) || '—';

    const subParts = [];
    if (p?.active) {
      subParts.push(phase);
      if (Number(p?.stepTotal||0)>0) subParts.push(`pas ${p.step}/${p.stepTotal}`);
      if (p?.startedAt) subParts.push(formatDuration(duration));
      if (processed !== '—') subParts.push(processed);
    } else if (state !== 'idle') {
      subParts.push(phase);
      if (p?.startedAt) subParts.push(formatDuration(duration));
      if (processed !== '—') subParts.push(processed);
      if (finishedAgo) subParts.push(finishedAgo);
    } else {
      subParts.push('Monitor operațional');
      subParts.push('gata');
    }
    if (el('operationHudSub')) el('operationHudSub').textContent = subParts.join(' • ');
    if (el('operationHudBadge')) el('operationHudBadge').textContent = stateLabel(state);
    if (el('operationHudPanelTitle')) el('operationHudPanelTitle').textContent = state === 'idle' ? 'Sistem pregătit' : `${phase} • ${stateLabel(state)}`;
    if (el('operationHudMessage')) el('operationHudMessage').textContent = p?.message || (state==='idle'?'Nicio operație activă.':'Operație finalizată.');
    if (el('operationHudDetail')) el('operationHudDetail').textContent = p?.detail || '';
    if (el('operationHudStep')) el('operationHudStep').textContent = Number(p?.stepTotal||0)>0 ? `Pas ${stepText}` : phase;
    if (el('operationHudDuration')) el('operationHudDuration').textContent = p?.startedAt ? formatDuration(duration) : '—';
    if (el('operationHudProcessed')) el('operationHudProcessed').textContent = processed;
    if (el('operationHudClock')) el('operationHudClock').textContent = p?.active ? `start ${localTime(p.startedAt)}` : (finish?.finishedAt ? `gata ${localTime(finish.finishedAt)}` : (p?.startedAt ? `start ${localTime(p.startedAt)}` : '—'));
    if (el('operationHudNext')) el('operationHudNext').textContent = nextAction(p,state);
    const progress = el('operationHudProgress');
    const bar = el('operationHudProgressBar');
    if (progress && bar) {
      progress.classList.toggle('indeterminate',p?.active && pct === null);
      if (pct !== null) bar.style.width = `${pct.toFixed(1)}%`;
      else if (!p?.active) bar.style.width = state === 'success' ? '100%' : '0%';
    }
  }

  async function refreshHud() {
    try {
      const response = await fetch('/api/status',{cache:'no-store'});
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      consecutiveFailures = 0;
      render(data);
    } catch (_) {
      consecutiveFailures++;
      if (consecutiveFailures >= 3 && installHud()) {
        const host = el('operationHud');
        if (host) host.dataset.state = 'error';
        if (el('operationHudSub')) el('operationHudSub').textContent = 'Monitor local indisponibil';
        if (el('operationHudBadge')) el('operationHudBadge').textContent = 'OFFLINE';
        if (el('operationHudMessage')) el('operationHudMessage').textContent = 'Nu pot citi temporar /api/status.';
        if (el('operationHudNext')) el('operationHudNext').textContent = 'Interfața încearcă automat să refacă legătura cu serviciul local.';
      }
    } finally {
      clearTimeout(hudTimer);
      hudTimer = setTimeout(refreshHud, lastSnapshot?.active ? 800 : 1300);
    }
  }

  function boot() {
    injectStyles();
    if (!installHud()) {
      setTimeout(boot,100);
      return;
    }
    refreshHud();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded',boot,{once:true});
  else boot();
})();
'''

CHANGELOG_SECTION = '''## [8.5.1] — 2026-09-04

### Monitor operațional & interfață
- HUD operațional nou în colțul din dreapta sus: status live, operația curentă/ultima operație și stare colorată clar.
- Panou extensibil imersiv cu etapă, progres, durată, fișiere/date procesate, momentul pornirii/finalizării și detaliul tehnic raportat de backend.
- Indicator discret animat numai cât timp rulează o operație; succesul, eroarea și anularea au stări vizuale distincte.
- Durata ultimei operații este înghețată la finalizare în interfață, iar momentul finalizării este păstrat pentru sesiunea curentă.
- Acțiune contextuală în panou: explică ce urmează și oferă acces rapid la Dashboard și Jurnal.
- HUD-ul folosește exclusiv endpointul local `/api/status`; nu introduce trafic extern și nu modifică motorul de detecție/download.
- Layout responsive: rămâne compact pe ferestre înguste și se deschide într-un panou adaptat fără să acopere inutil interfața.

'''


def main():
    root = Path(__file__).resolve().parents[1]
    js_path = root / 'web' / 'exact_guard.js'
    js = js_path.read_text(encoding='utf-8')
    if HUD_MARKER not in js:
        js_path.write_text(js.rstrip() + HUD_MODULE + '\n', encoding='utf-8', newline='\n')

    version_path = root / 'VERSION'
    version_path.write_text('8.5.1\n', encoding='utf-8', newline='\n')

    changelog_path = root / 'CHANGELOG.md'
    changelog = changelog_path.read_text(encoding='utf-8')
    if '## [8.5.1]' not in changelog:
        marker = '## [8.5.0]'
        pos = changelog.find(marker)
        if pos < 0:
            raise SystemExit('Nu găsesc secțiunea 8.5.0 în CHANGELOG.md')
        changelog = changelog[:pos] + CHANGELOG_SECTION + changelog[pos:]
        changelog_path.write_text(changelog, encoding='utf-8', newline='\n')

    print('Operation HUD v8.5.1 applied.')


if __name__ == '__main__':
    main()
