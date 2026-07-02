const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const tokensSource = fs.readFileSync(path.join(__dirname, 'tokens.js'), 'utf8');

function extractFunction(source, name) {
  const signature = `function ${name}`;
  const start = source.indexOf(signature);
  assert.ok(start >= 0, `缺少函数 ${name}`);
  const paramsEnd = source.indexOf(')', start);
  const braceStart = source.indexOf('{', paramsEnd);
  let depth = 0;
  for (let i = braceStart; i < source.length; i++) {
    const char = source[i];
    if (char === '{') depth++;
    if (char === '}') depth--;
    if (depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(`函数 ${name} 大括号未闭合`);
}

function joinFunctions(names) {
  return names.map((name) => extractFunction(tokensSource, name)).join('\n\n');
}

test('tokens 页费用摘要按 0 位小数显示', () => {
  const sandbox = {
    escapeHtml(value) { return String(value ?? ''); },
    t(key, params = {}) {
      if (key === 'tokens.table.totalCost') return '总费用';
      if (key === 'tokens.table.dailyCost') return '当日费用';
      if (key === 'tokens.batteryUnlimited') return '无限额';
      if (key === 'tokens.batteryDailyRemaining') return `${params.remaining}/${params.limit} (${params.percent}%)`;
      if (key === 'tokens.batteryTotalRemaining') return `${params.remaining}/${params.limit} (${params.percent}%)`;
      return key;
    }
  };

  vm.runInNewContext(joinFunctions([
    'formatCostDisplay',
    'buildCostMetricRow',
    'getTokenEffectiveDailyCostLimit',
    'buildCostSummaryHtml'
  ]), sandbox);

  const html = sandbox.buildCostSummaryHtml({ total_cost_usd: 12.6, daily_cost_used_usd: 0.7, daily_cost_limit_usd: 10.2 });
  assert.match(html, /\$13/);
  assert.match(html, /\$1\/\$10/);
  assert.doesNotMatch(html, /\$12\.6/);
  assert.doesNotMatch(html, /\$0\.7/);
});

test('tokens 页电池图标根据剩余额度显示多档颜色和百分比', () => {
  const sandbox = {
    escapeHtml(value) { return String(value ?? ''); },
    t(key, params = {}) {
      if (key === 'tokens.batteryUnlimited') return '无限额';
      if (key === 'tokens.batteryDailyRemaining') return `日剩余 ${params.remaining}/${params.limit} (${params.percent}%)`;
      if (key === 'tokens.batteryTotalRemaining') return `总剩余 ${params.remaining}/${params.limit} (${params.percent}%)`;
      return key;
    }
  };

  vm.runInNewContext(joinFunctions([
    'formatCostDisplay',
    'getTokenEffectiveDailyCostLimit',
    'getTokenEffectiveCostLimit',
    'getTokenBatteryState',
    'buildTokenBatteryHtml'
  ]), sandbox);

  const full = sandbox.buildTokenBatteryHtml({ daily_cost_used_usd: 2, daily_cost_limit_usd: 10 });
  const high = sandbox.buildTokenBatteryHtml({ daily_cost_used_usd: 3.5, daily_cost_limit_usd: 10 });
  const medium = sandbox.buildTokenBatteryHtml({ daily_cost_used_usd: 5.5, daily_cost_limit_usd: 10 });
  const low = sandbox.buildTokenBatteryHtml({ daily_cost_used_usd: 7.5, daily_cost_limit_usd: 10 });
  const critical = sandbox.buildTokenBatteryHtml({ daily_cost_used_usd: 8.5, daily_cost_limit_usd: 10 });

  assert.match(full, /token-battery--full/);
  assert.match(full, /width: 80%/);
  assert.match(full, />80%<\/span>/);
  assert.match(high, /token-battery--high/);
  assert.match(high, />65%<\/span>/);
  assert.match(medium, /token-battery--medium/);
  assert.match(medium, />45%<\/span>/);
  assert.match(low, /token-battery--low/);
  assert.match(low, />25%<\/span>/);
  assert.match(critical, /token-battery--critical/);
  assert.match(critical, /width: 15%/);
  assert.match(critical, />15%<\/span>/);
});

test('tokens 页当日翻倍开关会把每日限额按 2 倍展示', () => {
  const sandbox = {};
  vm.runInNewContext(joinFunctions([
    'getTokenEffectiveDailyCostLimit'
  ]), sandbox);

  assert.equal(
    sandbox.getTokenEffectiveDailyCostLimit({ daily_cost_limit_usd: 3, daily_limit_double_enabled: true }),
    6
  );
});
