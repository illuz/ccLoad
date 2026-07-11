const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const statsHtml = fs.readFileSync(path.join(__dirname, '..', '..', 'stats.html'), 'utf8');
const statsSource = fs.readFileSync(path.join(__dirname, 'stats.js'), 'utf8');
const zhLocale = fs.readFileSync(path.join(__dirname, '..', 'locales', 'zh-CN.js'), 'utf8');
const enLocale = fs.readFileSync(path.join(__dirname, '..', 'locales', 'en.js'), 'utf8');

function extractFunction(source, name) {
  const signature = `function ${name}`;
  const start = source.indexOf(signature);
  assert.ok(start >= 0, `missing ${name}`);

  const braceStart = source.indexOf('{', start);
  let depth = 0;
  for (let index = braceStart; index < source.length; index++) {
    if (source[index] === '{') depth++;
    if (source[index] === '}') depth--;
    if (depth === 0) return source.slice(start, index + 1);
  }

  assert.fail(`${name} has an unclosed body`);
}

test('stats page mounts token and cost trend charts below the statistics table', () => {
  assert.match(statsHtml, /id="chart-stats-token-trend" class="stats-trend-chart"/);
  assert.match(statsHtml, /id="chart-stats-cost-trend" class="stats-trend-chart"/);
  assert.match(statsHtml, /id="stats-token-trend-total" class="stats-trend-total"/);
  assert.match(statsHtml, /id="stats-cost-trend-total" class="stats-trend-total"/);
  assert.match(zhLocale, /'stats\.tokenUsageTrend': 'Token用量趋势'/);
  assert.match(enLocale, /'stats\.costTrend': 'Cost Trend'/);
});

test('stats trends use daily buckets only for ranges longer than one day', () => {
  const context = {
    window: {
      getRangeHours(range) {
        return range === 'this_week' ? 168 : 24;
      }
    }
  };

  vm.runInNewContext(`
    ${extractFunction(statsSource, 'getStatsTrendRangeDurationMs')}
    ${extractFunction(statsSource, 'getStatsTrendBucketMinutes')}
    this.bucketFor = getStatsTrendBucketMinutes;
  `, context);

  assert.equal(context.bucketFor({ range: 'today' }), 60);
  assert.equal(context.bucketFor({ range: 'this_week' }), 1440);
  assert.equal(context.bucketFor({
    range: 'custom',
    customStartTime: '0',
    customEndTime: String(24 * 60 * 60 * 1000)
  }), 60);
  assert.equal(context.bucketFor({
    range: 'custom',
    customStartTime: '0',
    customEndTime: String(24 * 60 * 60 * 1000 + 1)
  }), 1440);
});

test('stats trend totals include all token categories and effective cost', () => {
  const context = {};
  vm.runInNewContext(`
    ${extractFunction(statsSource, 'getStatsTrendTokenTotal')}
    ${extractFunction(statsSource, 'getStatsTrendEffectiveCost')}
    this.totalTokens = getStatsTrendTokenTotal;
    this.effectiveCost = getStatsTrendEffectiveCost;
  `, context);

  const point = {
    input_tokens: 100,
    output_tokens: 50,
    cache_read_tokens: 25,
    cache_creation_tokens: 5,
    total_cost: 0.4,
    effective_cost: 0.8
  };
  assert.equal(context.totalTokens(point), 180);
  assert.equal(context.effectiveCost(point), 0.8);
  assert.equal(context.effectiveCost({ total_cost: 0.4 }), 0.4);
});
