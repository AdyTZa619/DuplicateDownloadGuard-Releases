const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function loadFastJD() {
  const source = fs.readFileSync(path.join(__dirname, '..', 'jdownloader_fast_v8567.js'), 'utf8');
  const context = {
    URL,
    URLSearchParams,
    clearTimeout,
    confirm: () => true,
    console,
    document: {
      readyState: 'loading',
      addEventListener() {},
    },
    Math,
    setTimeout: () => 0,
    window: {cfg: {}},
  };
  vm.runInNewContext(source, context, {filename: 'jdownloader_fast_v8567.js'});
  return context.window.ddgJDownloaderFastV8567;
}

function loadMediaPicker() {
  const source = fs.readFileSync(path.join(__dirname, '..', 'media_picker_v8566.js'), 'utf8');
  const context = {
    URL,
    console,
    document: {
      readyState: 'loading',
      addEventListener() {},
    },
    setTimeout: () => 0,
    window: {},
  };
  vm.runInNewContext(source, context, {filename: 'media_picker_v8566.js'});
  return context.window.ddgMediaPickerV8566;
}

function loadCanonicalJDGuard() {
  const source = fs.readFileSync(path.join(__dirname, '..', 'jdownloader_window_capture_v8566.js'), 'utf8');
  const calls = [];
  const remembered = [];
  const listeners = {};
  const elements = {
    downloadMethod: {value: 'jdownloader'},
    downloadDir: {value: 'H:\\Downloads'},
    downloadGuardMode: {value: 'smart'},
  };
  const window = {
    api: async (url, options) => {
      calls.push({url, options});
      return {externalAdded: 1, externalItems: [{resultId: 8, url: 'https://bunkr.example/d/eight', name: 'eight.mp4'}], message: '1 fișier trimis', guard: {decisions: []}};
    },
    addEventListener: (name, fn) => { listeners[name] = fn; },
    cfg: {},
    idsForAction: () => [7, 8, 7],
    loadResults: async () => {},
    toast: () => {},
    ddgSmartStateEngineV8569: {rememberBackendHandoff: async items => remembered.push(...items)},
  };
  const context = {
    console,
    document: {getElementById: id => elements[id] || null},
    JSON,
    Number,
    Object,
    Set,
    String,
    window,
  };
  vm.runInNewContext(source, context, {filename: 'jdownloader_window_capture_v8566.js'});
  return {guard:window.ddgJDownloaderGuardV901, calls, listeners, remembered};
}

function loadSmartState() {
  const source = fs.readFileSync(path.join(__dirname, '..', 'smart_state_engine_v8569.js'), 'utf8');
  const values = new Map();
  const row = {id: 8, status: 'MISSING', autoStatus: 'MISSING', guardVerdict: 'DOWNLOAD', detector: {classification: 'LIPSĂ'}, remote: {source: 'BUNKR', handle: 'eight', size: 100, name: 'eight.mp4'}};
  const window = {api: async url => url.startsWith('/api/results?') ? {rows:[row], total:1} : {}};
  const context = {
    URL,
    CustomEvent: class {},
    MutationObserver: class { observe() {} },
    clearTimeout,
    console,
    document: {readyState:'loading', addEventListener() {}},
    localStorage: {
      getItem: key => values.has(key) ? values.get(key) : null,
      setItem: (key, value) => values.set(key, String(value)),
      removeItem: key => values.delete(key),
    },
    setTimeout,
    window,
  };
  vm.runInNewContext(source, context, {filename: 'smart_state_engine_v8569.js'});
  return {smart:window.ddgSmartStateEngineV8569, row};
}

test('JD keeps only current missing rows and never stale local verdicts', () => {
  const jd = loadFastJD();
  const rows = [
    {id: 1, guardVerdict: 'DOWNLOAD', detector: {classification: 'LIPSĂ'}, localPresent: false, remote: {url: 'https://example.test/1.jpg', name: '1.jpg'}},
    {id: 2, guardVerdict: 'DUPLICATE', localPresent: true, localPath: 'H:\\2.jpg', remote: {url: 'https://example.test/2.jpg', name: '2.jpg'}},
    {id: 3, guardVerdict: 'DUPLICATE', localPresent: false, localPath: 'H:\\gone.jpg', remote: {url: 'https://example.test/3.jpg', name: '3.jpg'}},
    {id: 4, status: 'MISSING', localPresent: false, remote: {url: 'https://example.test/4.jpg', name: '4.jpg'}},
    {id: 5, status: 'SAMPLED', localPresent: true, localPath: 'H:\\5.jpg', remote: {url: 'https://example.test/5.jpg', name: '5.jpg'}},
    {id: 6, guardVerdict: 'DOWNLOAD', detector: {classification: 'DE VERIFICAT'}, remote: {url: 'https://example.test/6.jpg', name: '6.jpg'}},
  ];
  assert.deepEqual(Array.from(jd.downloadRows(rows), row => row.id), [1]);
  assert.equal(jd.classify(rows[1]), 'DUPLICATE');
  assert.equal(jd.classify(rows[2]), 'REVIEW');
  assert.equal(jd.classify(rows[4]), 'REVIEW');
  assert.equal(jd.classify(rows[5]), 'REVIEW');
});

test('JD builds one FlashGot payload and one package for missing rows', () => {
  const jd = loadFastJD();
  const rows = [
    {id: 10, status: 'MISSING', guardVerdict: 'DOWNLOAD', detector: {classification: 'LIPSĂ'}, remote: {source: 'WEB', url: 'https://example.test/gallery', directUrl: 'https://cdn.test/a.jpg', path: 'album/a.jpg', name: 'a.jpg'}},
    {id: 11, status: 'MISSING', guardVerdict: 'DOWNLOAD', detector: {classification: 'LIPSĂ'}, remote: {source: 'WEB', url: 'https://example.test/gallery', directUrl: 'https://cdn.test/b.jpg', path: 'album/b.jpg', name: 'b.jpg'}},
  ];
  const submission = jd.buildSubmission(jd.downloadRows(rows));
  assert.equal(submission.count, 2);
  assert.equal(submission.packageName, 'album');
  assert.equal(submission.params.get('urls'), 'https://cdn.test/a.jpg\nhttps://cdn.test/b.jpg');
  assert.equal(submission.params.get('package'), 'album');
  assert.equal(submission.params.get('referer'), 'https://example.test/gallery');
});

test('legacy JD payload uses Bunkr download endpoint', () => {
  const jd = loadFastJD();
  const submission = jd.buildSubmission([{
    id: 12,
    guardVerdict: 'DOWNLOAD',
    detector: {classification: 'LIPSĂ'},
    remote: {source: 'BUNKR', url: 'https://bunkr.example/a/album', handle: 'file-handle', name: 'clip.mp4'},
  }]);
  assert.equal(submission.params.get('urls'), 'https://bunkr.example/d/file-handle');
});

test('earliest JD click guard sends IDs only to the guarded backend route', async () => {
  const {guard, calls, listeners, remembered} = loadCanonicalJDGuard();
  assert.ok(guard);
  assert.equal(typeof listeners.click, 'function');
  let prevented = false;
  let stopped = false;
  listeners.click({
    target: {closest: () => ({id: 'jdSelectedBtn', getAttribute: () => null})},
    preventDefault: () => { prevented = true; },
    stopImmediatePropagation: () => { stopped = true; },
  });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(prevented, true);
  assert.equal(stopped, true);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, '/api/download/jdownloader-direct');
  assert.deepEqual(JSON.parse(calls[0].options.body), {
    ids: [7, 8],
    destination: 'H:\\Downloads',
    guardMode: 'smart',
  });
  assert.deepEqual(remembered, [{resultId: 8, url: 'https://bunkr.example/d/eight', name: 'eight.mp4'}]);
});

test('Smart State never promotes provisional MISSING to final LIPSĂ', async () => {
  const {smart, row} = loadSmartState();
  assert.equal(smart.finalState({status:'POSSIBLE', detector:{classification:'DE VERIFICAT'}, guardAt:0}), 'ANALYZING');
  assert.equal(smart.finalState({status:'POSSIBLE', detector:{classification:'DE VERIFICAT'}, guardAt:123, guardVerdict:'REVIEW'}), 'REVIEW');
  assert.equal(smart.finalState({status:'MISSING', detector:{classification:'DE VERIFICAT'}}), 'ANALYZING');
  assert.equal(smart.finalState({status:'MISSING'}), 'REVIEW');
  assert.equal(smart.finalState({guardVerdict:'DOWNLOAD', detector:{classification:'DE VERIFICAT'}, guardAt:123}), 'REVIEW');
  assert.equal(smart.finalState(row), 'DOWNLOAD');
  assert.equal(await smart.rememberBackendHandoff([{resultId:8, url:'https://bunkr.example/d/eight', name:'eight.mp4'}]), 1);
  assert.equal(smart.finalState(row), 'IN_JD');
});

test('generic Media Picker refuses every dedicated provider', () => {
  const picker = loadMediaPicker();
  assert.equal(picker.genericPickerAllowed('https://example.test/gallery/123'), true);
  assert.equal(picker.genericPickerAllowed('https://www.erome.com/a/G4szqJSJ'), false);
  assert.equal(picker.genericPickerAllowed('https://gofile.io/d/example'), false);
  assert.equal(picker.genericPickerAllowed('https://bunkr.cr/a/example'), false);
  assert.equal(picker.genericPickerAllowed('https://cyberdrop.me/a/example'), false);
  assert.equal(picker.genericPickerAllowed('https://mega.nz/folder/example#key'), false);
});

test('generic Media Picker compares logical selected sources, not preview/thumbnail URLs', () => {
  const picker = loadMediaPicker();
  const urls = picker.candidateActionURLs([
    {url: 'https://example.test/watch/one', previewUrl: 'https://cdn.test/one-1080.mp4', thumbnail: 'https://img.test/one.jpg'},
    {url: 'https://example.test/watch/one', previewUrl: 'https://cdn.test/one-720.mp4'},
    {url: 'https://example.test/watch/two', previewUrl: 'https://cdn.test/two.mp4'},
  ]);
  assert.deepEqual(Array.from(urls), ['https://example.test/watch/one', 'https://example.test/watch/two']);
});
