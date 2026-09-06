// Audible notification when a new Stable/TEST update appears in the DDG top bar.
// Plays once per detected update version/channel and never loops every poll.
(() => {
  'use strict';

  const SEEN_KEY = 'ddg.updateSound.lastSeen.v1';
  let audioCtx = null;
  let armed = false;
  let pendingKey = '';
  let pendingLabel = '';
  let observer = null;

  function updateEntries() {
    const corner = document.getElementById('ddgUpdateCorner');
    if (!corner) return [];
    return [...corner.querySelectorAll('.ddgUpdateChip[data-channel]')]
      .map(btn => ({
        channel: String(btn.dataset.channel || '').trim().toLowerCase(),
        label: String(btn.textContent || '').trim()
      }))
      .filter(x => x.channel && x.label);
  }

  function makeKey(entries) {
    return entries
      .map(x => `${x.channel}:${x.label}`)
      .sort()
      .join('|');
  }

  function lastSeen() {
    try { return localStorage.getItem(SEEN_KEY) || ''; } catch (_) { return ''; }
  }

  function remember(key) {
    try { localStorage.setItem(SEEN_KEY, key); } catch (_) {}
  }

  function ensureAudio() {
    if (audioCtx) return audioCtx;
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) return null;
    try { audioCtx = new Ctx(); } catch (_) { audioCtx = null; }
    return audioCtx;
  }

  async function armAudio() {
    const ctx = ensureAudio();
    if (!ctx) return false;
    try {
      if (ctx.state === 'suspended') await ctx.resume();
      armed = ctx.state === 'running';
    } catch (_) {
      armed = false;
    }
    if (armed && pendingKey) {
      const key = pendingKey;
      const label = pendingLabel;
      pendingKey = '';
      pendingLabel = '';
      await playNotification(key, label);
    }
    return armed;
  }

  async function tone(ctx, frequency, start, duration, gainValue) {
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = 'sine';
    osc.frequency.setValueAtTime(frequency, start);
    gain.gain.setValueAtTime(0.0001, start);
    gain.gain.exponentialRampToValueAtTime(gainValue, start + 0.018);
    gain.gain.exponentialRampToValueAtTime(0.0001, start + duration);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start(start);
    osc.stop(start + duration + 0.03);
  }

  async function playNotification(key, label) {
    if (!key || key === lastSeen()) return;
    const ctx = ensureAudio();
    if (!ctx) return;
    try {
      if (ctx.state === 'suspended') await ctx.resume();
      if (ctx.state !== 'running') {
        pendingKey = key;
        pendingLabel = label;
        return;
      }
      const now = ctx.currentTime + 0.02;
      await tone(ctx, 784, now, 0.18, 0.16);
      await tone(ctx, 1046, now + 0.22, 0.24, 0.18);
      remember(key);
      if (typeof window.toast === 'function') {
        window.toast(`Update nou disponibil: ${label}`);
      }
    } catch (_) {
      pendingKey = key;
      pendingLabel = label;
    }
  }

  function scan() {
    const entries = updateEntries();
    if (!entries.length) return;
    const key = makeKey(entries);
    if (!key || key === lastSeen()) return;
    const label = entries.map(x => x.label).join(' + ');
    if (!armed) {
      pendingKey = key;
      pendingLabel = label;
      armAudio();
      return;
    }
    playNotification(key, label);
  }

  function boot() {
    // Browser/WebView audio policies may require one user interaction first.
    // Arm on the first normal interaction; if an update was already detected,
    // the pending sound plays immediately then.
    const arm = () => armAudio();
    document.addEventListener('pointerdown', arm, {passive: true});
    document.addEventListener('keydown', arm, {passive: true});

    if (!observer && document.body) {
      observer = new MutationObserver(scan);
      observer.observe(document.body, {childList: true, subtree: true, characterData: true});
    }

    setInterval(scan, 1500);
    setTimeout(scan, 1800);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, {once: true});
  } else {
    boot();
  }

  window.ddgUpdateSoundV8552 = {scan, armAudio};
})();
