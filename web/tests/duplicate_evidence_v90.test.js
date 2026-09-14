const test = require('node:test');
const assert = require('node:assert/strict');
const {render} = require('../duplicate_evidence_v90.js');
test('unknown evidence keeps a missing score and escapes source text', () => {
  const html = render({classification:'DE VERIFICAT', score:null, signals:['<img src=x onerror=alert(1)>']});
  assert.ok(html.includes('scor date insuficiente'));
  assert.ok(!html.includes('<img'));
  assert.ok(!html.includes('scor 0/100'));
});
test('measured evidence shows both technical versions and cache use', () => {
  const html = render({classification:'ACELAȘI CONȚINUT', score:93, videoScore:96, audioScore:91, framesMatched:15, framesTotal:15, durationDeltaSeconds:0.06, remote:{width:1280,height:720}, local:{width:1920,height:1080}, remoteCacheHit:true, remoteBytes:0});
  for (const text of ['93/100','Video fingerprint: 96%','Audio fingerprint: 91%','15/15 cadre','0.060 sec','ONLINE','LOCAL','1280','1920','cache']) assert.ok(html.includes(text));
});
