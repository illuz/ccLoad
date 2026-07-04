const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const logsSource = fs.readFileSync(path.join(__dirname, 'logs.js'), 'utf8');
const logsCss = fs.readFileSync(path.join(__dirname, '..', 'css', 'logs.css'), 'utf8');

test('日志页渠道和令牌链接都带用量浮层标记', () => {
  assert.match(logsSource, /data-usage-popover="channel"/);
  assert.match(logsSource, /data-usage-popover="token"/);
  assert.match(logsSource, /data-channel-id="\$\{channelId\}"/);
  assert.match(logsSource, /data-token-id="\$\{numericTokenID\}"/);
});

test('日志页用量浮层按当前时间范围加载渠道统计和令牌统计，并按渠道懒加载余额', () => {
  assert.match(logsSource, /function getLogsUsageCacheKey\(\)/);
  assert.match(logsSource, /appendLogsTimeRangeParams\(params, filters\)/);
  assert.match(logsSource, /fetchDataWithAuth\(`\/admin\/stats\?\$\{params\.toString\(\)\}`\)/);
  assert.match(logsSource, /fetchDataWithAuth\(`\/admin\/auth-tokens\?\$\{params\.toString\(\)\}`\)/);
  assert.match(logsSource, /fetchDataWithAuth\(`\/admin\/channels\/\$\{key\}`\)/);
  assert.match(logsSource, /logsUsageStatsCache/);
  assert.match(logsSource, /logsUsageTokensCache/);
  assert.match(logsSource, /logsUsageChannelsCache/);
});

test('日志页渠道浮层包含渠道管理页耗时、消耗、成本和余额语义', () => {
  assert.match(logsSource, /t\('channels\.table\.duration'\)/);
  assert.match(logsSource, /t\('channels\.table\.usage'\)/);
  assert.match(logsSource, /t\('channels\.stats\.firstByte'\)/);
  assert.match(logsSource, /t\('stats\.tooltipDuration'\)/);
  assert.match(logsSource, /t\('channels\.stats\.input'\)/);
  assert.match(logsSource, /t\('channels\.stats\.output'\)/);
  assert.match(logsSource, /t\('channels\.stats\.cacheHitRate'\)/);
  assert.match(logsSource, /t\('channels\.stats\.cost'\)/);
  assert.match(logsSource, /formatCostPair\(totalCost,\s*effectiveCost\)/);
  assert.match(logsSource, /t\('channels\.upstreamBalance\.title'\)/);
  assert.match(logsSource, /t\('channels\.upstreamBalance\.limit'\)/);
  assert.match(logsSource, /t\('channels\.upstreamBalance\.usedToday'\)/);
  assert.match(logsSource, /t\('channels\.upstreamBalance\.plan'\)/);
});

test('日志页令牌浮层包含令牌管理页 Token 用量和费用语义', () => {
  assert.match(logsSource, /t\('tokens\.table\.tokenUsage'\)/);
  assert.match(logsSource, /t\('tokens\.costSummary'\)/);
  assert.match(logsSource, /t\('tokens\.input'\)/);
  assert.match(logsSource, /t\('tokens\.output'\)/);
  assert.match(logsSource, /t\('tokens\.table\.totalCost'\)/);
  assert.match(logsSource, /t\('tokens\.table\.dailyCost'\)/);
});

test('日志页用量浮层有独立主题样式', () => {
  assert.match(logsCss, /\.logs-usage-popover\s*\{/);
  assert.match(logsCss, /background:\s*var\(--surface-bg-strong\)/);
  assert.match(logsCss, /\.logs-usage-popover__row\s*\{/);
  assert.match(logsCss, /\.logs-usage-popover__hint\s*\{/);
});
