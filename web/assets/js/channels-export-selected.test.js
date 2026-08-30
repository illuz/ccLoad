const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'channels.html'), 'utf8');
const initSource = fs.readFileSync(path.join(__dirname, 'channels-init.js'), 'utf8');
const modalsSource = fs.readFileSync(path.join(__dirname, 'channels-modals.js'), 'utf8');

function loadExportModule(selectedIDs = []) {
  const fetchCalls = [];
  const warnings = [];
  const downloads = [];
  const button = { disabled: false };

  global.window = {
    t: (key) => key,
    showWarning: (message) => warnings.push(message),
    showError() {},
    showSuccess() {}
  };
  global.document = {
    getElementById(id) {
      return id === 'batchExportChannelsBtn' ? button : null;
    },
    createElement() {
      return {
        click() {
          downloads.push({ href: this.href, download: this.download });
        },
        remove() {}
      };
    },
    body: { appendChild() {}, removeChild() {} }
  };
  global.getSelectedChannelIDs = () => selectedIDs;
  global.fetchWithAuth = async (url) => {
    fetchCalls.push(url);
    return {
      ok: true,
      headers: { get: () => null },
      blob: async () => new Blob(['csv'])
    };
  };
  global.formatTimestampForFilename = () => '20260817-000000';
  global.URL = { createObjectURL: () => 'blob:test', revokeObjectURL() {} };

  const modulePath = require.resolve('./channels-import-export.js');
  delete require.cache[modulePath];
  const api = require(modulePath);

  return {
    api,
    fetchCalls,
    downloads,
    warnings,
    button,
    cleanup() {
      delete require.cache[modulePath];
      delete global.window;
      delete global.document;
      delete global.getSelectedChannelIDs;
      delete global.fetchWithAuth;
      delete global.formatTimestampForFilename;
      delete global.URL;
    }
  };
}

test('渠道批量菜单提供导出所选动作并参与禁用状态同步', () => {
  assert.match(html, /id="batchExportChannelsBtn"[\s\S]*?data-action="batch-export-channels"/);
  assert.match(initSource, /'batch-export-channels': \(\) => exportSelectedChannelsCSV\(\)/);
  assert.match(modalsSource, /'batchExportChannelsBtn'/);
});

test('exportSelectedChannelsCSV 只把所选渠道 ID 传给导出接口', async () => {
  const harness = loadExportModule([7, 3, 11]);
  try {
    await harness.api.exportSelectedChannelsCSV();
    assert.deepEqual(harness.fetchCalls, ['/admin/channels/export?ids=7,3,11']);
    assert.deepEqual(harness.downloads, [{ href: 'blob:test', download: 'channels-20260817-000000.csv' }]);
    assert.equal(harness.button.disabled, false);
    assert.deepEqual(harness.warnings, []);
  } finally {
    harness.cleanup();
  }
});

test('exportSelectedChannelsCSV 在没有选择时不请求全量导出', async () => {
  const harness = loadExportModule([]);
  try {
    await harness.api.exportSelectedChannelsCSV();
    assert.deepEqual(harness.fetchCalls, []);
    assert.deepEqual(harness.downloads, []);
    assert.deepEqual(harness.warnings, ['channels.batchNoSelection']);
  } finally {
    harness.cleanup();
  }
});
