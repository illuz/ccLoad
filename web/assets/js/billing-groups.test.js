const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'billing-groups.html'), 'utf8');
const script = fs.readFileSync(path.join(__dirname, 'billing-groups.js'), 'utf8');
const styles = fs.readFileSync(path.join(__dirname, '..', 'css', 'billing-groups.css'), 'utf8');
const ui = fs.readFileSync(path.join(__dirname, 'ui.js'), 'utf8');
const channelsHTML = fs.readFileSync(path.join(__dirname, '..', '..', 'channels.html'), 'utf8');
const channelKeys = fs.readFileSync(path.join(__dirname, 'channels-keys.js'), 'utf8');
const channelModals = fs.readFileSync(path.join(__dirname, 'channels-modals.js'), 'utf8');

test('计费分组作为 API 令牌后的独立导航页并支持局部路由', () => {
  assert.match(ui, /\{ key: 'tokens',[\s\S]*?\{ key: 'billing-groups'/);
  assert.match(ui, /dashboardPageKeys = \[[^\]]*'tokens', 'billing-groups', 'stats'/);
  assert.match(ui, /\(\?:index\|channels\|tokens\|billing-groups\|stats/);
  assert.match(html, /id="billingGroupForm"/);
  assert.match(html, /id="billingGroupChannelOptions"/);
  assert.match(html, /\/web\/assets\/js\/billing-groups\.js/);
  assert.match(script, /topbarKey: 'billing-groups'/);
});

test('计费分组按渠道分区展示具体 Key，并提交 key 级选择', () => {
  assert.match(script, /class="billing-groups-channel-section"/);
  assert.match(script, /class="billing-groups-channel-section__keys"/);
  assert.match(script, /key\.alias \|\| ''/);
  assert.match(script, /alias \? `\$\{alias\}（Key \$\{index \+ 1\}）` : fallback/);
  assert.match(script, /channel_keys: channelKeysPayload/);
  assert.match(script, /billingGroups\.selectionSummary/);
  assert.match(script, /billingGroups\.listMeta/);
  assert.match(styles, /\.billing-groups-channel-options\s*\{[\s\S]*?flex-direction: column/);
  assert.match(styles, /\.billing-groups-channel-section__keys\s*\{[\s\S]*?grid-template-columns/);
  assert.match(styles, /@media \(min-width: 1281px\)[\s\S]*?\.billing-groups-page\s*\{[\s\S]*?padding-top: 18px/);
});

test('渠道 Key 别名可编辑、回填并随渠道保存', () => {
  assert.match(channelsHTML, /class="inline-key-alias-input modal-inline-input"/);
  assert.match(channelsHTML, /maxlength="191"/);
  assert.match(channelKeys, /item\.alias \|\| ''/);
  assert.match(channelKeys, /function updateInlineKeyAlias\(index, value\)/);
  assert.match(channelModals, /\.\.\.\(row\.alias \? \{ alias: row\.alias \} : \{\}\)/);
});
