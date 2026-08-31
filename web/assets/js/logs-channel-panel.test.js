const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const scriptPath = path.join(__dirname, 'logs-channel-panel.js');
const htmlPath = path.join(__dirname, '..', '..', 'logs.html');
const cssPath = path.join(__dirname, '..', 'css', 'logs-channel-panel.css');
const zhPath = path.join(__dirname, '..', 'locales', 'zh-CN.js');
const enPath = path.join(__dirname, '..', 'locales', 'en.js');

const source = fs.readFileSync(scriptPath, 'utf8');
const html = fs.readFileSync(htmlPath, 'utf8');
const css = fs.readFileSync(cssPath, 'utf8');
const zhLocale = fs.readFileSync(zhPath, 'utf8');
const enLocale = fs.readFileSync(enPath, 'utf8');

function loadTestAPI(windowOverrides = {}) {
  const sandbox = {
    console,
    clearTimeout,
    setTimeout,
    window: { ...windowOverrides }
  };
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox);
  return sandbox.window.LogsChannelQuickPanel.__test;
}

function plain(value) {
  return JSON.parse(JSON.stringify(value));
}

test('日志页接入默认折叠的渠道快捷浮窗', () => {
  assert.match(html, /logs-channel-panel\.css/);
  assert.match(html, /logs-channel-panel\.js/);
  assert.match(html, /id="logsChannelPanel"[^>]*data-state="collapsed"/);
  assert.match(html, /id="logsChannelPanelTrigger"[\s\S]*aria-expanded="false"/);
  assert.match(html, /id="logsChannelPanelSurface"[^>]*hidden/);
  assert.doesNotMatch(html, /id="logsChannelPanel"[\s\S]*?onclick=/);
});

test('日志快捷浮窗提供渠道和令牌两个 tab，并有独立令牌入口', () => {
  assert.match(html, /id="logsTokenPanelTrigger"[\s\S]*data-channel-panel-action="open-token"/);
  assert.ok(
    html.indexOf('id="logsTokenPanelTrigger"') < html.indexOf('id="logsChannelPanelTrigger"'),
    '令牌入口应位于渠道入口上方'
  );
  assert.match(css, /\.logs-channel-panel\s*\{[\s\S]*?flex-direction:\s*column;/);
  assert.match(html, /id="logsChannelPanelChannelTab"[\s\S]*role="tab"/);
  assert.match(html, /id="logsChannelPanelTokenTab"[\s\S]*data-panel-tab="tokens"/);
  assert.match(html, /data-channel-panel-action="select-tab"/);
  assert.match(source, /setActiveTab\(TOKEN_TAB/);
  assert.match(source, /\/admin\/auth-tokens/);
});

test('浮窗使用固定右下角工具布局并适配窄屏', () => {
  assert.match(css, /\.logs-channel-panel\s*\{[\s\S]*?position:\s*fixed;[\s\S]*?right:[\s\S]*?bottom:/);
  assert.match(css, /\.logs-channel-panel__surface\s*\{[\s\S]*?width:\s*min\(380px,[\s\S]*?max-height:/);
  assert.match(css, /@media\s*\(max-width:\s*640px\)[\s\S]*?width:\s*calc\(100vw\s*-\s*24px\)/);
  assert.match(css, /\.logs-channel-panel__name\s*\{[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?white-space:\s*nowrap;/);
  assert.match(css, /\.logs-channel-panel__drag\s*\{[\s\S]*?touch-action:\s*none;/);
});

test('分组构建按优先级排列渠道并统计启用数量', () => {
  const api = loadTestAPI();
  const grouped = plain(api.buildChannelGroups([
    { id: 4, name: 'Loose', group_id: 0, priority: 9, enabled: true },
    { id: 2, name: 'Second', group_id: 7, priority: 10, enabled: false },
    { id: 1, name: 'First', group_id: 7, priority: 30, enabled: true },
    { id: 3, name: 'Other', group_id: 8, priority: 20, enabled: true }
  ], [
    { id: 8, name: 'Beta', color: '#not-a-color' },
    { id: 7, name: 'Alpha', color: '#22c55e' }
  ], 'No group'));

  assert.deepEqual(grouped.map((group) => ({
    key: group.key,
    name: group.name,
    color: group.color,
    ids: group.channels.map((channel) => channel.id),
    enabled: group.enabledCount,
    total: group.totalCount
  })), [
    { key: '7', name: 'Alpha', color: '#22c55e', ids: [1, 2], enabled: 1, total: 2 },
    { key: '8', name: 'Beta', color: '#64748b', ids: [3], enabled: 1, total: 1 },
    { key: '0', name: 'No group', color: '#64748b', ids: [4], enabled: 1, total: 1 }
  ]);
});

test('令牌快捷控制按分组展示启用统计并提供编辑/启停操作', () => {
  const api = loadTestAPI();
  const grouped = plain(api.buildTokenGroups([
    { id: 2, description: 'Backup', group_id: 9, is_active: false, last_used_at: 100 },
    { id: 1, description: 'Primary', group_id: 9, is_active: true, last_used_at: 200 },
    { id: 3, description: 'Loose', group_id: 0, is_active: true }
  ], [{ id: 9, name: 'Production', color: '#22c55e' }], 'No group'));
  assert.deepEqual(grouped.map((group) => ({
    key: group.key,
    name: group.name,
    ids: group.tokens.map((token) => token.id),
    enabled: group.enabledCount,
    total: group.totalCount
  })), [
    { key: '9', name: 'Production', ids: [1, 2], enabled: 1, total: 2 },
    { key: '0', name: 'No group', ids: [3], enabled: 1, total: 1 }
  ]);
  const row = api.renderTokenRow({
    id: 4,
    description: 'Customer token',
    is_active: true,
    plain_token: 'sk-1234567890',
    success_count: 4,
    failure_count: 1,
    daily_cost_used_usd: 0.125
  }, '9');
  assert.match(row, /data-channel-panel-action="edit-token"/);
  assert.match(row, /data-channel-panel-action="toggle-token"[^>]*aria-checked="true"/);
  assert.match(row, /sk-1\*\*\*\*7890/);
  assert.doesNotMatch(row, /sk-1234567890[^<]/);
});

test('同组重排保留其他分组的相对位置并生成全局递减优先级', () => {
  const api = loadTestAPI();
  const channels = [
    { id: 1, name: 'A1', group_id: 1, priority: 50 },
    { id: 2, name: 'B1', group_id: 2, priority: 40 },
    { id: 3, name: 'A2', group_id: 1, priority: 30 },
    { id: 4, name: 'B2', group_id: 2, priority: 20 }
  ];
  const updates = plain(api.buildPriorityUpdatesAfterGroupReorder(channels, 1, [3, 1]));

  assert.deepEqual(updates, [
    { id: 3, priority: 40 },
    { id: 2, priority: 30 },
    { id: 1, priority: 20 },
    { id: 4, priority: 10 }
  ]);
  assert.deepEqual(updates.filter((item) => [2, 4].includes(item.id)).map((item) => item.id), [2, 4]);
});

test('排序请求拒绝缺失、重复或跨组的渠道 ID', () => {
  const api = loadTestAPI();
  const channels = [
    { id: 1, group_id: 1, priority: 30 },
    { id: 2, group_id: 1, priority: 20 },
    { id: 3, group_id: 2, priority: 10 }
  ];

  assert.deepEqual(plain(api.buildPriorityUpdatesAfterGroupReorder(channels, 1, [1])), []);
  assert.deepEqual(plain(api.buildPriorityUpdatesAfterGroupReorder(channels, 1, [1, 1])), []);
  assert.deepEqual(plain(api.buildPriorityUpdatesAfterGroupReorder(channels, 1, [1, 3])), []);
});

test('快捷启停复用批量接口并在失败时回滚本地状态', () => {
  assert.match(source, /channel\.enabled\s*=\s*nextEnabled;[\s\S]*?\/admin\/channels\/batch-enabled/);
  assert.match(source, /catch\s*\(error\)[\s\S]*?channel\.enabled\s*=\s*previousEnabled;/);
  assert.match(source, /role="switch"[\s\S]*?aria-checked=/);
  assert.match(source, /\/admin\/auth-tokens\/\$\{encodeURIComponent\(id\)\}/);
  assert.match(source, /token\.is_active\s*=\s*previousActive/);
});

test('渠道行提供快速编辑图标并复用日志页渠道编辑器', () => {
  assert.match(source, /data-channel-panel-action="edit-channel"/);
  assert.match(source, /\$\{ICONS\.edit\}/);
  assert.match(source, /window\.openLogChannelEditor\(actionTarget\.dataset\.channelId\)/);
  assert.match(css, /\.logs-channel-panel__row-actions\s*\{[\s\S]*?display:\s*flex;/);
});

test('渠道行在优先级旁展示格式化的当日使用费用', () => {
  const api = loadTestAPI();
  assert.equal(api.formatDailyCost(0), '$0');
  assert.equal(api.formatDailyCost(1.23456), '$1.235');
  assert.equal(api.formatDailyCost(undefined), '$0');

  const customAPI = loadTestAPI({ formatCost: (value) => `USD ${value.toFixed(2)}` });
  assert.equal(customAPI.formatDailyCost(1.23456), 'USD 1.23');

  const row = api.renderChannelRow({
    id: 7,
    name: 'Test channel',
    channel_type: 'openai',
    priority: 30,
    daily_cost_used: 0.125,
    enabled: true
  }, '1');
  assert.match(row, /logs-channel-panel__priority[^>]*>Priority 30<\/span>[\s\S]*?logs-channel-panel__daily-cost[^>]*>Today \$0\.125<\/span>/);
  assert.match(css, /\.logs-channel-panel__daily-cost\s*\{[\s\S]*?overflow:\s*hidden;[\s\S]*?white-space:\s*nowrap;/);
  assert.match(css, /@media\s*\(max-width:\s*640px\)[\s\S]*?grid-template-areas:\s*"priority daily-cost"\s*"type cooldown";/);
});

test('排序复用批量优先级接口且限制为同组拖放', () => {
  assert.match(source, /\/admin\/channels\/batch-priority/);
  assert.match(source, /list\s*!==\s*state\.nativeDrag\.list/);
  assert.match(source, /\['ArrowUp',\s*'ArrowDown'\]/);
  assert.match(source, /event\.pointerType\s*===\s*'mouse'/);
  assert.match(source, /setPointerCapture/);
});

test('渠道浮窗文案同时提供中英文键值', () => {
  const requiredKeys = [
    'title', 'open', 'collapse', 'refresh', 'loading', 'loadFailed',
    'enabledSummary', 'dailyCost', 'dragHandle', 'editChannel', 'toggleFailed', 'orderSaved', 'orderFailed'
  ];
  for (const suffix of requiredKeys) {
    const key = `logs.channelPanel.${suffix}`;
    assert.ok(zhLocale.includes(`'${key}'`), `中文缺少 ${key}`);
    assert.ok(enLocale.includes(`'${key}'`), `英文缺少 ${key}`);
  }
  for (const suffix of [
    'openTokens', 'collapseTokens', 'refreshTokens', 'channelTab', 'tokenTab', 'loadingTokens',
    'tokenLoadFailed', 'emptyTokens', 'tokenEnabledSummary', 'tokenUsage',
    'tokenDailyCost', 'tokenLastUsed', 'tokenID', 'editToken',
    'tokenToggleEnabled', 'tokenToggleDisabled', 'tokenToggleFailed'
  ]) {
    const key = `logs.channelPanel.${suffix}`;
    assert.ok(zhLocale.includes(`'${key}'`), `中文缺少 ${key}`);
    assert.ok(enLocale.includes(`'${key}'`), `英文缺少 ${key}`);
  }
});
