const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'model-test.html'), 'utf8');
const source = fs.readFileSync(path.join(__dirname, 'model-test.js'), 'utf8');

function extractFunction(name) {
  const marker = `function ${name}`;
  let start = source.indexOf(`async ${marker}`);
  if (start < 0) start = source.indexOf(marker);
  assert.ok(start >= 0, `missing ${name}`);
  const open = source.indexOf('{', start);
  let depth = 0;
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++;
    if (source[i] === '}') depth--;
    if (depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(`unterminated ${name}`);
}

test('模型测试页提供渠道 Key 选择器', () => {
  assert.match(html, /id="keySelectorLabel"[\s\S]*?id="testChannelKeySelectContainer"/);
  assert.match(source, /payload\.key_index = target\.keyIndex/);
  assert.match(source, /requestId !== channelKeyLoadRequestId/);
});

test('渠道模式默认选择首个启用 Key，全部禁用时保持未选择', () => {
  const sandbox = {
    keys: [
      { key_index: 0, disabled: true },
      { key_index: 4, disabled: false },
      { key_index: 9, disabled: false }
    ]
  };
  vm.runInNewContext([
    extractFunction('getFirstEnabledModelTestKey'),
    'result = getFirstEnabledModelTestKey(keys);'
  ].join('\n'), sandbox);
  assert.equal(sandbox.result.key_index, 4);

  sandbox.keys = [{ key_index: 0, disabled: true }];
  vm.runInNewContext(`result = (${extractFunction('getFirstEnabledModelTestKey')})(keys);`, sandbox);
  assert.equal(sandbox.result, null);
});

test('按模型批测在全部禁用时仍明确指定首个 Key', () => {
  const sandbox = {
    keys: [{ key_index: 5, disabled: true }]
  };
  vm.runInNewContext([
    extractFunction('getFirstEnabledModelTestKey'),
    extractFunction('getPreferredModelTestKey'),
    'result = getPreferredModelTestKey(keys);'
  ].join('\n'), sandbox);
  assert.equal(sandbox.result.key_index, 5);
});
