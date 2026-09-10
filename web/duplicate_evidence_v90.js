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
