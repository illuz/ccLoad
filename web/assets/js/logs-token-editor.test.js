const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'logs.html'), 'utf8');
const logsScript = fs.readFileSync(path.join(__dirname, 'logs.js'), 'utf8');
const tokenEditorScript = fs.readFileSync(path.join(__dirname, 'logs-token-editor.js'), 'utf8');

test('日志页接入令牌编辑器桥接脚本', () => {
  assert.match(html, /<script defer src="\/web\/assets\/js\/logs-token-editor\.js\?v=__VERSION__"><\/script>/);
});

test('日志页令牌列渲染为可点击的令牌编辑按钮', () => {
  assert.match(logsScript, /function buildLogTokenDescDisplay\(label,\s*tokenId = 0\)/);
  assert.match(logsScript, /class="channel-link token-link logs-token-desc-text"/);
  assert.match(logsScript, /data-token-id="\$\{numericTokenID\}"/);
  assert.match(logsScript, /buildLogTokenDescDisplay\(entry\.auth_token_description,\s*entry\.auth_token_id\)/);
});

test('日志页令牌按钮点击事件委托到令牌编辑弹窗', () => {
  assert.match(logsScript, /const tokenBtn = e\.target\.closest\('\.token-link\[data-token-id\]'\);/);
  assert.match(logsScript, /openLogTokenEditor\(tokenId\)/);
});

test('日志页令牌编辑器复用 tokens.html 编辑弹窗标记但不加载 tokens.js，避免顶层变量冲突', () => {
  assert.match(tokenEditorScript, /fetch\(versioned\('\/web\/tokens\.html'\)/);
  assert.match(tokenEditorScript, /'editModal'/);
  assert.match(tokenEditorScript, /'channelSelectModal'/);
  assert.match(tokenEditorScript, /'modelSelectModal'/);
  assert.match(tokenEditorScript, /'tpl-token-expiry-options'/);
  assert.doesNotMatch(tokenEditorScript, /tokens\.js/);
});

test('日志页令牌编辑器同步并保存 Codex Guard 开关', () => {
  assert.match(tokenEditorScript, /const editCodexGuardInput = document\.getElementById\('editCodexGuardEnabled'\);/);
  assert.match(tokenEditorScript, /editCodexGuardInput\.checked = !!token\.codex_guard_enabled;/);
  assert.match(tokenEditorScript, /const codexGuardEnabled = !!document\.getElementById\('editCodexGuardEnabled'\)\?\.checked;/);
  assert.match(tokenEditorScript, /codex_guard_enabled:\s*codexGuardEnabled,/);
});

test('日志页令牌编辑保存后刷新日志令牌筛选和列表', () => {
  assert.match(tokenEditorScript, /window\.loadAuthTokensIntoSelect\('f_auth_token'/);
  assert.match(tokenEditorScript, /if \(typeof authTokens !== 'undefined'\)/);
  assert.match(tokenEditorScript, /authTokens = tokens;/);
  assert.match(tokenEditorScript, /if \(typeof load === 'function'\)/);
  assert.match(tokenEditorScript, /await load\(true\);/);
});

test('日志页令牌编辑器保存支持令牌页同款预设过期时间', () => {
  assert.match(tokenEditorScript, /const days = parseInt\(expiryType,\s*10\);/);
  assert.match(tokenEditorScript, /expiresAt = Date\.now\(\) \+ days \* 24 \* 60 \* 60 \* 1000;/);
});
