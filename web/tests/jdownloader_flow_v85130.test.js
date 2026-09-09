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

test('JD keeps only current missing rows and never stale local verdicts', () => {
  const jd = loadFastJD();
  const rows = [
    {id: 1, guardVerdict: 'DOWNLOAD', localPresent: false, remote: {url: 'https://example.test/1.jpg', name: '1.jpg'}},
    {id: 2, guardVerdict: 'DUPLICATE', localPresent: true, localPath: 'H:\\2.jpg', remote: {url: 'https://example.test/2.jpg', name: '2.jpg'}},
    {id: 3, guardVerdict: 'DUPLICATE', localPresent: false, localPath: 'H:\\gone.jpg', remote: {url: 'https://example.test/3.jpg', name: '3.jpg'}},
    {id: 4, status: 'MISSING', localPresent: false, remote: {url: 'https://example.test/4.jpg', name: '4.jpg'}},
    {id: 5, status: 'SAMPLED', localPresent: true, localPath: 'H:\\5.jpg', remote: {url: 'https://example.test/5.jpg', name: '5.jpg'}},
  ];
  assert.deepEqual(Array.from(jd.downloadRows(rows), row => row.id), [1, 4]);
  assert.equal(jd.classify(rows[1]), 'DUPLICATE');
  assert.equal(jd.classify(rows[2]), 'REVIEW');
  assert.equal(jd.classify(rows[4]), 'REVIEW');
});

test('JD builds one FlashGot payload and one package for missing rows', () => {
  const jd = loadFastJD();
  const rows = [
    {id: 10, status: 'MISSING', remote: {source: 'WEB', url: 'https://example.test/gallery', directUrl: 'https://cdn.test/a.jpg', path: 'album/a.jpg', name: 'a.jpg'}},
    {id: 11, status: 'MISSING', remote: {source: 'WEB', url: 'https://example.test/gallery', directUrl: 'https://cdn.test/b.jpg', path: 'album/b.jpg', name: 'b.jpg'}},
  ];
  const submission = jd.buildSubmission(jd.downloadRows(rows));
  assert.equal(submission.count, 2);
  assert.equal(submission.packageName, 'album');
  assert.equal(submission.params.get('urls'), 'https://cdn.test/a.jpg\nhttps://cdn.test/b.jpg');
  assert.equal(submission.params.get('package'), 'album');
  assert.equal(submission.params.get('referer'), 'https://example.test/gallery');
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
