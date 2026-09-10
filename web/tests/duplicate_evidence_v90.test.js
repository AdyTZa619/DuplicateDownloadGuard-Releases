const test = require('node:test');
const assert = require('node:assert/strict');
const {render} = require('../duplicate_evidence_v90.js');
test('unknown evidence keeps a missing score and escapes source text', () => {
  const html = render({classification:'NECUNOSCUT / DATE INSUFICIENTE', score:null, signals:['<img src=x onerror=alert(1)>']});
  assert.ok(html.includes('scor date insuficiente'));
  assert.ok(!html.includes('<img'));
  assert.ok(!html.includes('scor 0/100'));
});
test('measured evidence shows both technical versions and cache use', () => {
  const html = render({classification:'PROBABIL', score:93, remote:{width:1280,height:720}, local:{width:1920,height:1080}, remoteCacheHit:true, remoteBytes:0});
  for (const text of ['93/100','ONLINE','LOCAL','1280','1920','cache']) assert.ok(html.includes(text));
});
