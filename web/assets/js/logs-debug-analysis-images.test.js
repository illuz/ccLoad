const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const logsSource = fs.readFileSync(path.join(__dirname, 'logs.js'), 'utf8');

function extractFunction(source, name) {
  const start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `缺少函数 ${name}`);
  const bodyStart = source.indexOf('{', start);
  let depth = 0;
  for (let i = bodyStart; i < source.length; i++) {
    if (source[i] === '{') depth++;
    if (source[i] === '}') {
      depth--;
      if (depth === 0) return source.slice(start, i + 1);
    }
  }
  assert.fail(`函数 ${name} 大括号未闭合`);
}

function getHelper() {
  const sandbox = {};
  vm.createContext(sandbox);
  vm.runInContext(`${extractFunction(logsSource, 'getDebugAnalysisImageData')}\nthis.helper = getDebugAnalysisImageData;`, sandbox);
  return sandbox.helper;
}

test('分析图片预览仅接受受支持 MIME 和规范 base64', () => {
  const helper = getHelper();
  const image = helper({ mime_type: 'image/png', data: 'cG5nLWJ5dGVz', bytes: 9, source: 'output' });

  assert.deepEqual(JSON.parse(JSON.stringify(image)), {
    mimeType: 'image/png',
    data: 'cG5nLWJ5dGVz',
    bytes: 9,
    source: 'output',
  });
  assert.equal(helper({ mime_type: 'image/svg+xml', data: 'PHN2Zz4=' }), null);
  assert.equal(helper({ mime_type: 'image/png', data: 'not-valid!' }), null);
});

test('分析页保留图片区域、来源标记与受限预览样式', () => {
  const css = fs.readFileSync(path.join(__dirname, '..', 'css', 'logs.css'), 'utf8');
  const zhLocale = fs.readFileSync(path.join(__dirname, '..', 'locales', 'zh-CN.js'), 'utf8');

  assert.match(logsSource, /debugAnalysisImages/);
  assert.match(logsSource, /data:\$\{image\.mimeType\};base64,\$\{image\.data\}/);
  assert.match(css, /\.debug-analysis-image img\s*\{[\s\S]*?max-height:\s*min\(50vh,\s*480px\);[\s\S]*?object-fit:\s*contain;/);
  assert.match(zhLocale, /'logs\.debugAnalysisImages': '图片'/);
});
