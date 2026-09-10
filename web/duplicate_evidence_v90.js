(() => {
  'use strict';
  const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  function render(evidence) {
    if (!evidence) return '';
    const score = Number.isFinite(evidence.score) ? `${evidence.score}/100` : 'date insuficiente';
    const signals = (evidence.signals || []).map(value => `<li>${escape(value)}</li>`).join('');
    let technical = '';
    if (evidence.remote && evidence.local) {
      const r = evidence.remote, l = evidence.local;
      const fields = [
        ['Durată', `${r.duration ?? '?'} s`, `${l.duration ?? '?'} s`],
        ['Rezoluție', `${r.width ?? '?'} × ${r.height ?? '?'}`, `${l.width ?? '?'} × ${l.height ?? '?'}`],
        ['Codec video', r.videoCodec, l.videoCodec],
        ['Bitrate', r.bitRate, l.bitRate],
        ['FPS', r.fps, l.fps],
        ['Audio', r.audioCodec || 'fără audio', l.audioCodec || 'fără audio']
      ];
      technical = '<table style="width:100%;margin-top:8px"><thead><tr><th>Criteriu</th><th>ONLINE</th><th>LOCAL</th></tr></thead><tbody>' +
        fields.map(row => '<tr>' + row.map(value => `<td>${escape(value ?? '—')}</td>`).join('') + '</tr>').join('') + '</tbody></table>';
    }
    return `<div class="guardReason"><b>${escape(evidence.classification)}</b> · scor ${escape(score)}<div class="muted small">${escape(evidence.basis)}</div>` +
      `<ul>${signals}</ul>${technical}${evidence.quality ? `<div><b>${escape(evidence.quality)}</b></div>` : ''}` +
      `<div class="muted small">Video citit pentru analiză: ${Number(evidence.remoteBytes || 0).toLocaleString('ro-RO')} bytes${evidence.remoteCacheHit ? ' · amprentă online din cache, validată prin antetul sursei' : ''}` +
      `${evidence.pending ? ` · ${Number(evidence.pending)} candidați rămași de verificat` : ''}</div></div>`;
  }
  const api = {render};
  if (typeof module === 'object' && module.exports) module.exports = api;
  else globalThis.DDGDuplicateEvidenceV90 = api;
})();

// Own DOM listeners and status endpoint; no replacement of shared UI functions.
if (typeof document !== 'undefined') document.addEventListener('DOMContentLoaded', () => {
  const panel = document.querySelector('#results .sectionBody');
  if (!panel) return;
  const card = document.createElement('div');
  card.className = 'row muted small';
  const label = document.createElement('span');
  const cancel = document.createElement('button'); cancel.className = 'btn'; cancel.textContent = 'Oprește analiza';
  const retry = document.createElement('button'); retry.className = 'btn'; retry.textContent = 'Reverifică duplicatele';
  card.append(label, cancel, retry); panel.prepend(card);
  let busy = false;
  async function refresh() {
    if (busy || document.hidden) return;
    busy = true;
    try {
      const response = await fetch('/api/duplicates/status');
      if (!response.ok) return;
      const status = await response.json();
      label.textContent = `Detector: ${status.completed}/${status.total} · ${status.message || 'Așteaptă scanarea sursei.'}`;
      cancel.hidden = !status.active; retry.disabled = status.active || !status.total;
    } catch (_) { label.textContent = 'Starea detectorului nu este disponibilă.'; }
    finally { busy = false; }
  }
  for (const [button, action] of [[cancel, 'cancel'], [retry, 'start']]) button.addEventListener('click', async () => {
    button.disabled = true;
    try { await fetch('/api/duplicates/' + action, {method: 'POST'}); await refresh(); }
    catch (_) { label.textContent = 'Nu am putut modifica analiza.'; }
    finally { button.disabled = false; }
  });
  refresh(); setInterval(refresh, 1500);
});
