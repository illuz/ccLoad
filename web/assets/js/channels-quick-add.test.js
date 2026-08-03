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
      discoverQuickAddModels,
      normalizeQuickAddURL,
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

test('快速添加支持带业务前缀的环境变量和 Bearer 密钥', () => {
  const { parseQuickAddInput } = loadQuickAddForTest();
  const parsed = parseQuickAddInput(`
    export OPENAI_BASE_URL="https://gateway.example.com/openai/v1/"
    Authorization: Bearer oauth-token-value
  `);

  assert.equal(parsed.url, 'https://gateway.example.com/openai');
  assert.deepEqual(Array.from(parsed.keys), ['oauth-token-value']);
});

test('快速添加支持宽松 JSON 字段名和多 Key', () => {
  const { parseQuickAddInput } = loadQuickAddForTest();
  const parsed = parseQuickAddInput(JSON.stringify({
    config: {
      ANTHROPIC_BASE_URL: 'https://gateway.example.com/claude/v1/models',
      ANTHROPIC_AUTH_TOKEN: ['token-a', 'token-b', 'token-a']
    }
  }));

  assert.equal(parsed.url, 'https://gateway.example.com/claude');
  assert.deepEqual(Array.from(parsed.keys), ['token-a', 'token-b']);
});

test('快速添加只去除完整 v1 路径段', () => {
  const { normalizeQuickAddURL } = loadQuickAddForTest();

  assert.equal(normalizeQuickAddURL('https://gateway.example.com/api/v1'), 'https://gateway.example.com/api');
  assert.equal(normalizeQuickAddURL('https://gateway.example.com/v1/models'), 'https://gateway.example.com');
  assert.equal(normalizeQuickAddURL('https://gateway.example.com/v1beta'), 'https://gateway.example.com/v1beta');
});

test('快速添加按所选渠道类型探测模型并去重', async () => {
  const { discoverQuickAddModels } = loadQuickAddForTest();
  let request;
  const models = await discoverQuickAddModels(
    'https://gateway.example.com',
    'sk-probe',
    'openai',
    async (url, options) => {
      request = { url, body: JSON.parse(options.body) };
      return {
        success: true,
        data: { models: [{ model: 'gpt-test' }, { model: 'GPT-TEST' }, { model: 'gpt-next' }] }
      };
    }
  );

  assert.deepEqual(request, {
    url: '/admin/channels/models/fetch',
    body: {
      channel_type: 'openai',
      url: 'https://gateway.example.com',
      api_key: 'sk-probe'
    }
  });
  assert.deepEqual(Array.from(models), ['gpt-test', 'gpt-next']);
});

test('快速添加不会在模型探测失败后继续创建', async () => {
  const { discoverQuickAddModels } = loadQuickAddForTest();

  await assert.rejects(
    discoverQuickAddModels(
      'https://gateway.example.com',
      'sk-invalid',
      'anthropic',
      async () => ({ success: false, error: 'unauthorized' })
    ),
    /unauthorized/
  );
});
