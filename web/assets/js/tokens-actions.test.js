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

  assert.match(template, /class="token-enable-switch \{\{tokenEnableSwitchClass\}\}"[\s\S]*?data-action="toggle-token-active"[\s\S]*?role="switch"[\s\S]*?aria-checked="\{\{isActive\}\}"/);
  assert.match(template, /class="token-select-checkbox" data-token-id="\{\{id\}\}" \{\{selectedAttr\}\} aria-label="\{\{selectionLabel\}\}"/);
  assert.match(template, /class="btn-icon btn-edit token-row-action-btn"/);
  assert.match(template, /class="btn-icon btn-danger btn-delete token-row-action-btn"/);
  assert.match(template, /data-i18n-title="common\.edit" data-i18n-aria-label="common\.edit" title="编辑" aria-label="编辑"/);
  assert.match(template, /data-i18n-title="common\.delete" data-i18n-aria-label="common\.delete" title="删除" aria-label="删除"/);
  assert.match(template, /<button[^>]*class="btn-icon btn-edit[\s\S]*?<svg[\s\S]*?aria-hidden="true"[\s\S]*?<\/button>/);
  assert.match(template, /<button[^>]*class="btn-icon btn-danger btn-delete[\s\S]*?<svg[\s\S]*?aria-hidden="true"[\s\S]*?<\/button>/);

  assert.doesNotMatch(template, /btn-copy-token/);
  assert.doesNotMatch(template, /<button[^>]*data-i18n="common\.(?:edit|delete)"[^>]*>/);
  assert.doesNotMatch(template, />\s*(?:编辑|删除)\s*<\/button>/);
});

test('tokens 图标按钮保持固定尺寸并支持图标内部点击', () => {
  assert.match(tokensCss, /\.token-row-action-btn\s*\{[\s\S]*?display:\s*inline-flex;[\s\S]*?width:\s*28px;[\s\S]*?height:\s*28px;[\s\S]*?padding:\s*0;/);
  assert.match(tokensCss, /\.token-enable-switch\s*\{[\s\S]*?width:\s*40px;[\s\S]*?height:\s*22px;/);
  assert.match(tokensCss, /\.token-enable-switch--on\s*\{[\s\S]*?background:\s*#22c55e;/);
  assert.match(tokensCss, /\.token-enable-switch--off\s*\{[\s\S]*?background:\s*#cbd5e1;/);
  assert.match(tokensCss, /\.token-row-action-btn\.btn-danger\s*\{[\s\S]*?color:\s*var\(--error-600\);/);
  assert.match(tokensCss, /\.token-select-checkbox\s*\{[\s\S]*?width:\s*16px;[\s\S]*?height:\s*16px;/);
  assert.match(tokensScript, /const target = e\.target\.closest\('\.btn-edit, \.btn-delete, \.token-enable-switch'\);/);
  assert.match(tokensScript, /body:\s*JSON\.stringify\(\{\s*is_active:\s*isActive\s*\}\)/);
});

test('tokens 页仅保留名称搜索并改为本地过滤', () => {
  assert.match(tokensHtml, /class="tokens-search-row"[\s\S]*id="tokenSearchInput"[\s\S]*data-i18n-placeholder="tokens\.searchPlaceholder"/);
  assert.match(tokensHtml, /class="tokens-search-control"[\s\S]*id="tokenSearchInput"[\s\S]*data-action="clear-token-search"/);
  assert.doesNotMatch(tokensHtml, /data-action="filter-tokens"/);
  assert.match(tokensHtml, /class="tokens-view-switch"[\s\S]*data-i18n-aria-label="tokens\.viewSwitcher"/);
  assert.match(tokensHtml, /<h3 class="token-edit-section-title" data-i18n="tokens\.basicInfo">基础信息<\/h3>/);
  assert.match(tokensHtml, /<h3 class="token-edit-section-title" data-i18n="tokens\.quotaInfo">配额信息<\/h3>/);
  assert.doesNotMatch(tokensHtml, /tokens-page-subtitle/);
  assert.doesNotMatch(tokensHtml, /tokens-pagination-card/);
  assert.doesNotMatch(tokensHtml, /id="tokens_page_size"/);
  assert.doesNotMatch(tokensScript, /tokensPageSize/);
  assert.doesNotMatch(tokensScript, /tokensCurrentPage/);
  assert.doesNotMatch(tokensScript, /params\.set\('search', tokenSearch\);/);
  assert.doesNotMatch(tokensScript, /params\.set\('limit'/);
  assert.doesNotMatch(tokensScript, /params\.set\('offset'/);
  assert.match(tokensScript, /function getVisibleTokens\(\)/);
  assert.match(tokensScript, /const visibleTokens = getVisibleTokens\(\);/);
});

test('tokens 页搜索区域与渠道页一致为单行输入+按钮布局', () => {
  assert.match(tokensCss, /\.tokens-filter-controls\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-columns:\s*minmax\(280px,\s*1\.35fr\)\s+minmax\(0,\s*1fr\);/);
  assert.match(tokensCss, /\.tokens-search-row\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-columns:\s*80px\s+minmax\(0,\s*1fr\);/);
  assert.match(tokensCss, /\.tokens-search-control\s*\{[\s\S]*?display:\s*flex;[\s\S]*?flex:\s*1\s+1\s+auto;[\s\S]*?min-width:\s*0;/);
  assert.match(tokensCss, /\.tokens-search-input\s*\{[\s\S]*?flex:\s*1\s+1\s+auto;[\s\S]*?min-width:\s*0;/);
  assert.match(tokensCss, /@media\s*\(max-width:\s*768px\)\s*\{[\s\S]*?\.tokens-filter-controls\s*\{[\s\S]*?grid-template-columns:\s*1fr;/);
  assert.match(tokensCss, /@media\s*\(max-width:\s*768px\)\s*\{[\s\S]*?\.tokens-search-control\s*\{[\s\S]*?width:\s*100%;[\s\S]*?flex-wrap:\s*wrap;/);
});

test('tokens 暗色主题提示块与批量操作文案存在', () => {
  assert.match(tokensCss, /--token-info-bg:\s*var\(--info-50\)/);
  assert.match(tokensCss, /html\[data-theme="dark"\],[\s\S]*?--token-info-bg:\s*rgba\(14,\s*165,\s*233,\s*0\.16\)/);
  assert.match(tokensCss, /\.token-model-import-tip\s*\{[\s\S]*?background:\s*var\(--token-info-bg\);[\s\S]*?border:\s*1px solid var\(--token-info-border\);/);
  assert.match(tokensCss, /\.token-model-import-code\s*\{[\s\S]*?background:\s*var\(--token-inline-code-bg\);/);
  assert.match(tokensHtml, /id="tokenBatchFloatingMenu"[\s\S]*id="selectedTokensCountBadge"[\s\S]*id="batchEnableTokensBtn"[\s\S]*id="batchDisableTokensBtn"[\s\S]*id="batchDeleteTokensBtn"/);
});

test('tokens 列表支持默认按最后使用排序与批量选择', () => {
  assert.match(tokensScript, /function sortTokensByUsage\(tokens\)/);
  assert.match(tokensScript, /if \(!tokenSearch\) return sortTokensByUsage\(allTokens\);/);
  assert.match(tokensScript, /const visibleTokens = getVisibleTokens\(\);/);
  assert.match(tokensScript, /className = 'mobile-card-table mobile-card-table--selectable tokens-table'/);
  assert.match(tokensScript, /class="tokens-visible-selection-checkbox" data-group-key="\$\{escapeHtml\(String\(selectionGroupKey \|\| ''\)\)\}"/);
  assert.match(tokensScript, /selectedTokenIds = new Set|let selectedTokenIds = new Set\(\);/);
  assert.match(tokensScript, /function updateBatchTokenSelectionUI\(\)/);
  assert.match(tokensScript, /function batchSetSelectedTokensEnabled\(isActive\)/);
  assert.match(tokensScript, /function batchDeleteSelectedTokens\(\)/);
  assert.match(tokensScript, /function toggleTokenGroupSelection\(groupKey\)/);
});

test('tokens 分组管理弹窗改为左窄右宽，右侧渠道和模型使用双主区块布局', () => {
  assert.match(tokensHtml, /<div class="modal-body token-group-manager-body">[\s\S]*?class="token-group-list-panel"[\s\S]*?class="token-group-form-panel"/);
  assert.match(tokensHtml, /class="token-group-list-header"[\s\S]*?data-i18n="tokens\.groupList"[\s\S]*?data-action="create-token-group-draft"/);
  assert.match(tokensHtml, /class="token-edit-section token-group-basic-section"[\s\S]*?class="token-group-basic-grid"[\s\S]*?class="form-group token-group-basic-field"[\s\S]*?id="tokenGroupName"[\s\S]*?id="tokenGroupDescription"[\s\S]*?class="form-group form-row-inline token-group-basic-field token-group-basic-field--limit"[\s\S]*?id="tokenGroupMaxConcurrency"/);
  assert.match(tokensHtml, /class="token-edit-main token-group-main"[\s\S]*?class="token-edit-section token-edit-section--channels token-group-restrictions token-group-restrictions--channels"[\s\S]*?id="tokenGroupAllowedChannelsSummary"[\s\S]*?class="token-edit-section token-edit-section--models token-group-restrictions token-group-restrictions--models"[\s\S]*?id="tokenGroupAllowedModelsSummary"/);
  assert.match(tokensCss, /\.token-group-modal\s*\{[\s\S]*?width:\s*88%;[\s\S]*?max-width:\s*1440px;/);
  assert.match(tokensCss, /\.token-group-manager-body\s*\{[\s\S]*?grid-template-columns:\s*260px\s+minmax\(0,\s*1fr\);[\s\S]*?min-height:\s*min\(72vh,\s*720px\);/);
  assert.match(tokensCss, /\.token-group-form-panel\s*\{[\s\S]*?grid-template-rows:\s*auto\s+minmax\(0,\s*1fr\);/);
  assert.match(tokensCss, /\.token-group-list-header\s*\{[\s\S]*?display:\s*flex;[\s\S]*?justify-content:\s*space-between;/);
  assert.match(tokensCss, /\.token-group-basic-grid\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0,\s*1fr\)\s+minmax\(0,\s*1fr\);/);
  assert.match(tokensCss, /\.token-group-basic-field\s*\{[\s\S]*?grid-template-columns:\s*76px\s+minmax\(0,\s*1fr\);[\s\S]*?align-items:\s*center;/);
  assert.match(tokensCss, /\.token-group-main\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0,\s*1fr\)\s+minmax\(0,\s*1fr\);/);
  assert.match(tokensCss, /\.token-group-restrictions__list\s*\{[\s\S]*?flex:\s*1\s+1\s+auto;[\s\S]*?min-height:\s*200px;/);
});

test('tokens 分组管理左侧摘要压成单行，整行可编辑且子弹窗层级高于父弹窗', () => {
  assert.match(tokensHtml, /id="channelSelectModal" class="modal token-stacked-modal"/);
  assert.match(tokensHtml, /id="modelSelectModal" class="modal token-stacked-modal"/);
  assert.match(tokensHtml, /id="modelImportModal" class="modal token-model-import-modal token-stacked-modal"/);
  assert.match(tokensHtml, /data-i18n="tokens\.groupDescriptionLabel">描述<\/label>/);
  assert.match(tokensHtml, /id="tokenGroupSaveBtn"[^>]*data-i18n="common\.save"/);
  assert.match(tokensScript, /<div class="token-group-list-item" data-action="edit-token-group" data-group-id="\$\{group\.id\}">/);
  assert.doesNotMatch(tokensScript, /data-action="edit-token-group"[^>]*>\$\{t\('common\.edit'\)\}/);
  assert.match(tokensScript, /class="btn-icon btn-danger token-group-delete-btn" data-action="delete-token-group" data-group-id="\$\{group\.id\}"/);
  assert.match(tokensScript, /'create-token-group-draft': \(\) => createTokenGroupDraft\(\)/);
  assert.match(tokensScript, /async function createTokenGroupDraft\(\)/);
  assert.match(tokensScript, /name:\s*defaultName,\s*[\s\S]*description:\s*''/);
  assert.match(tokensScript, /const defaultName = t\('tokens\.untitledGroup'\)/);
  assert.match(tokensCss, /\.token-group-list-desc\s*\{[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?white-space:\s*nowrap;/);
  assert.match(tokensCss, /\.token-group-list-meta\s*\{[\s\S]*?display:\s*block;[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?white-space:\s*nowrap;/);
  assert.match(tokensCss, /\.token-group-list-meta\s+span \+ span::before\s*\{[\s\S]*?content:\s*" · ";/);
  assert.match(tokensCss, /\.token-group-list-item\s*\{[\s\S]*?cursor:\s*pointer;/);
  assert.match(tokensCss, /\.token-group-delete-btn\s*\{[\s\S]*?width:\s*28px;[\s\S]*?height:\s*28px;/);
  assert.match(tokensCss, /\.token-stacked-modal\s*\{[\s\S]*?z-index:\s*1100;/);
});

test('tokens 页列表/分组视图切换会切到分组渲染并切换 active 状态', () => {
  assert.match(tokensScript, /function setTokenViewMode\(mode\)/);
  assert.match(tokensScript, /if \(tokenViewMode === 'group'\) \{[\s\S]*?renderGroupedTokens\(container,\s*visibleTokens\);[\s\S]*?\} else \{[\s\S]*?container\.appendChild\(createTokensTable\(visibleTokens\)\);[\s\S]*?\}/);
  assert.match(tokensScript, /if \(listBtn\) listBtn\.classList\.toggle\('active', tokenViewMode !== 'group'\);/);
  assert.match(tokensScript, /if \(groupBtn\) groupBtn\.classList\.toggle\('active', tokenViewMode === 'group'\);/);
  assert.match(tokensHtml, /data-action="set-token-view-list"/);
  assert.match(tokensHtml, /data-action="set-token-view-group"/);
});

test('tokens 增删改后等待列表刷新完成', () => {
  assert.match(tokensScript, /document\.getElementById\('tokenResultModal'\)\.style\.display = 'block';\s*await loadTokens\(\);/);
  assert.match(tokensScript, /closeEditModal\(\);\s*await loadTokens\(\);/);
  assert.match(tokensScript, /method: 'DELETE'[\s\S]*await loadTokens\(\);/);
});
