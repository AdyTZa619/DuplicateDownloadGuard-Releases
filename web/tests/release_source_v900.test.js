const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '..', '..');
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8');

test('release source contains the routes previously injected only in TEST builds', () => {
  const main = read('main.go');
  const extra = read('v8_extra.go');
  const gallery = read('provider_gallery_scan.go');
  const transport = read('provider_http_transport.go');
  const index = read('web/index.html');

  assert.match(main, /mux\.HandleFunc\("\/api\/queue\/add", a\.handleQueueAddRoutedV8550\)/);
  assert.match(main, /mux\.HandleFunc\("\/api\/download\/jdownloader-direct", a\.handleJDownloaderDirectEndpointV8551\)/);
  assert.match(main, /mux\.HandleFunc\("\/api\/update\/native-notify", a\.handleUpdateNativeNotifyV8554\)/);
  assert.match(main, /registerLocalPreviewDiagnosticsV8599\(mux, a\)/);
  assert.doesNotMatch(main, /mux\.HandleFunc\("\/api\/queue\/add", a\.handleQueueAdd\)/);
  assert.doesNotMatch(main, /mux\.HandleFunc\("\/api\/local-preview", a\.handleLocalPreview\)/);

  assert.match(extra, /func portableDownloadsDir\(\) string \{ return executableDir\(\) \}/);
  assert.match(extra, /galleryDLDownloadBunkrV8560/);
  assert.match(gallery, /officialGalleryDLForBunkrV8560/);
  assert.match(transport, /preferredGalleryDLProviderV8560\(\)/);
  assert.match(transport, /extractor\.bunkr\.tlds=true/);

  assert.match(index, /option value="jdownloader"/);
  assert.match(index, /jdownloader_final_v8551\.js/);
  assert.match(index, /let engine=\$\("downloadMethod"\)\?\.value\|\|cfg\.downloadMethod\|\|"auto";/);
});

test('TEST updater keeps pinned Git revision safety and recovery fallback together', () => {
  const updater = read('web/update_channels_v8546.js');

  assert.match(updater, /const REPO_API = 'https:\/\/api\.github\.com\/repos\/AdyTZa619\/DuplicateDownloadGuard-Releases'/);
  assert.match(updater, /const TEST_BRANCH_REF_API = `\$\{REPO_API\}\/git\/ref\/heads\/testing`/);
  assert.match(updater, /contentsBytes\('update-test\.json', ref\)/);
  assert.match(updater, /contentsBytes\('test-releases\/DuplicateDownloadGuard_PRO_TEST\.exe', state\.ref\)/);
  assert.match(updater, /const RECOVERY_PORTS = \[51289, 51290, 51291, 51292\]/);
  assert.match(updater, /async function backendAliveForUpdate\(\)/);
  assert.match(updater, /async function applyTestViaRecovery\(manifest\)/);
  assert.doesNotMatch(updater, /const TEST_MANIFEST_URL/);
});
