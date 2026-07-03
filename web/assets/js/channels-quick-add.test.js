const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const quickAddSource = fs.readFileSync(path.join(__dirname, 'channels-quick-add.js'), 'utf8');

function loadQuickAddForTest() {
  const context = {
    URL,
    window: {}
  };
  vm.createContext(context);
  vm.runInContext(`
    ${quickAddSource}
    window.__quickAddTest = {
      QUICK_ADD_TYPE_DEFAULT,
      detectTypeFromURL,
      parseQuickAddInput
    };
  `, context);
  return context.window.__quickAddTest;
}

test('快速添加可从 newapi_channel_conn JSON 中提取 URL 和 sk', () => {
  const { parseQuickAddInput } = loadQuickAddForTest();
  const parsed = parseQuickAddInput('{"_type":"newapi_channel_conn","key":"sk-test-quickadd-json-key-1234567890","url":"https://muyuan.do"}');

  assert.equal(parsed.url, 'https://muyuan.do');
  assert.deepEqual(Array.from(parsed.keys), ['sk-test-quickadd-json-key-1234567890']);
});

test('快速添加未知 URL 默认识别为 codex', () => {
  const { QUICK_ADD_TYPE_DEFAULT, detectTypeFromURL } = loadQuickAddForTest();

  assert.equal(QUICK_ADD_TYPE_DEFAULT, 'codex');
  assert.equal(detectTypeFromURL('https://muyuan.do'), 'codex');
});

test('快速添加仍兼容首行 URL 后续行 Key 并去重', () => {
  const { parseQuickAddInput } = loadQuickAddForTest();
  const parsed = parseQuickAddInput('https://codex.hiyo.top\nsk-aaa\nsk-bbb\nsk-aaa');

  assert.equal(parsed.url, 'https://codex.hiyo.top');
  assert.deepEqual(Array.from(parsed.keys), ['sk-aaa', 'sk-bbb']);
});

test('快速添加在 JSON 和首行 URL 格式都不符合时使用正则兜底', () => {
  const { parseQuickAddInput } = loadQuickAddForTest();
  const parsed = parseQuickAddInput('请添加这个渠道，地址是 https://regex.example.com/api；密钥为 sk-regex-fallback-key-123456。');

  assert.equal(parsed.url, 'https://regex.example.com/api');
  assert.deepEqual(Array.from(parsed.keys), ['sk-regex-fallback-key-123456']);
});
