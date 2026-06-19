const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const tokensHtml = fs.readFileSync(path.join(__dirname, '..', '..', 'tokens.html'), 'utf8');
const tokensCss = fs.readFileSync(path.join(__dirname, '..', 'css', 'tokens.css'), 'utf8');
const tokensScript = fs.readFileSync(path.join(__dirname, 'tokens.js'), 'utf8');

function tokenRowTemplate() {
  const match = tokensHtml.match(/<template id="tpl-token-row">[\s\S]*?<\/template>/);
  assert.ok(match, '缺少 tpl-token-row 模板');
  return match[0];
}

test('tokens 操作列使用图标按钮而不是文字按钮', () => {
  const template = tokenRowTemplate();

  assert.match(template, /class="btn-copy-token btn-icon token-row-action-btn"/);
  assert.match(template, /class="btn-icon btn-edit token-row-action-btn"/);
  assert.match(template, /class="btn-icon btn-danger btn-delete token-row-action-btn"/);
  assert.match(template, /data-i18n-title="common\.copy" data-i18n-aria-label="common\.copy" title="复制" aria-label="复制"/);
  assert.match(template, /data-i18n-title="common\.edit" data-i18n-aria-label="common\.edit" title="编辑" aria-label="编辑"/);
  assert.match(template, /data-i18n-title="common\.delete" data-i18n-aria-label="common\.delete" title="删除" aria-label="删除"/);
  assert.match(template, /<button[^>]*class="btn-copy-token[\s\S]*?<svg[\s\S]*?aria-hidden="true"[\s\S]*?<\/button>/);
  assert.match(template, /<button[^>]*class="btn-icon btn-edit[\s\S]*?<svg[\s\S]*?aria-hidden="true"[\s\S]*?<\/button>/);
  assert.match(template, /<button[^>]*class="btn-icon btn-danger btn-delete[\s\S]*?<svg[\s\S]*?aria-hidden="true"[\s\S]*?<\/button>/);

  assert.doesNotMatch(template, /<button[^>]*data-i18n="common\.(?:copy|edit|delete)"[^>]*>/);
  assert.doesNotMatch(template, />\s*(?:复制|编辑|删除)\s*<\/button>/);
});

test('tokens 图标按钮保持固定尺寸并支持图标内部点击', () => {
  assert.match(tokensCss, /\.token-row-action-btn\s*\{[\s\S]*?display:\s*inline-flex;[\s\S]*?width:\s*28px;[\s\S]*?height:\s*28px;[\s\S]*?padding:\s*0;/);
  assert.match(tokensCss, /\.token-row-action-btn\.btn-danger\s*\{[\s\S]*?color:\s*var\(--error-600\);/);
  assert.match(tokensScript, /const target = e\.target\.closest\('\.btn-copy-token, \.btn-edit, \.btn-delete'\);/);
});

test('tokens 页提供名称搜索、分页并默认每页 200 条', () => {
  assert.match(tokensHtml, /class="tokens-search-row"[\s\S]*id="tokenSearchInput"[\s\S]*data-i18n-placeholder="tokens\.searchPlaceholder"/);
  assert.match(tokensHtml, /class="tokens-search-control"[\s\S]*data-action="filter-tokens"[\s\S]*data-action="clear-token-search"/);
  assert.match(tokensHtml, /data-action="filter-tokens"/);
  assert.match(tokensHtml, /class="tokens-view-switch"[\s\S]*data-i18n-aria-label="tokens\.viewSwitcher"/);
  assert.match(tokensHtml, /<h3 class="token-edit-section-title" data-i18n="tokens\.basicInfo">基础信息<\/h3>/);
  assert.match(tokensHtml, /<h3 class="token-edit-section-title" data-i18n="tokens\.quotaInfo">配额信息<\/h3>/);
  assert.match(tokensHtml, /id="tokens_page_size"[\s\S]*<option value="200" selected>200<\/option>/);
  assert.match(tokensScript, /let tokensPageSize = parseInt\(localStorage\.getItem\('tokens\.pageSize'\), 10\) \|\| 200;/);
  assert.match(tokensScript, /params\.set\('search', tokenSearch\);/);
  assert.match(tokensScript, /params\.set\('limit', String\(tokensPageSize\)\);/);
  assert.match(tokensScript, /params\.set\('offset', String\(\(tokensCurrentPage - 1\) \* tokensPageSize\)\);/);
});

test('tokens 页搜索区域与渠道页一致为单行输入+按钮布局', () => {
  assert.match(tokensCss, /\.tokens-search-row\s*\{[\s\S]*?display:\s*flex;[\s\S]*?align-items:\s*center;[\s\S]*?flex-wrap:\s*wrap;/);
  assert.match(tokensCss, /\.tokens-search-control\s*\{[\s\S]*?display:\s*flex;[\s\S]*?flex:\s*1\s+1\s+auto;[\s\S]*?min-width:\s*0;/);
  assert.match(tokensCss, /\.tokens-search-input\s*\{[\s\S]*?flex:\s*1\s+1\s+auto;[\s\S]*?min-width:\s*0;/);
  assert.match(tokensCss, /@media\s*\(max-width:\s*768px\)\s*\{[\s\S]*?\.tokens-search-control\s*\{[\s\S]*?width:\s*100%;[\s\S]*?flex-wrap:\s*wrap;/);
});

test('tokens 页列表/分组视图切换会切到分组渲染并切换 active 状态', () => {
  assert.match(tokensScript, /function setTokenViewMode\(mode\)/);
  assert.match(tokensScript, /if \(tokenViewMode === 'group'\) \{[\s\S]*?renderGroupedTokens\(container\);[\s\S]*?\} else \{[\s\S]*?container\.appendChild\(createTokensTable\(allTokens\)\);[\s\S]*?\}/);
  assert.match(tokensScript, /if \(listBtn\) listBtn\.classList\.toggle\('active', tokenViewMode !== 'group'\);/);
  assert.match(tokensScript, /if \(groupBtn\) groupBtn\.classList\.toggle\('active', tokenViewMode === 'group'\);/);
  assert.match(tokensHtml, /data-action="set-token-view-list"/);
  assert.match(tokensHtml, /data-action="set-token-view-group"/);
});

test('tokens 增删改后等待列表刷新完成', () => {
  assert.match(tokensScript, /tokensCurrentPage = 1;\s*await loadTokens\(\);/);
  assert.match(tokensScript, /closeEditModal\(\);\s*await loadTokens\(\);/);
  assert.match(tokensScript, /method: 'DELETE'[\s\S]*await loadTokens\(\);/);
});
