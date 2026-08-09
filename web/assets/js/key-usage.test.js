const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'key-usage.html'), 'utf8');
const script = fs.readFileSync(path.join(__dirname, 'key-usage.js'), 'utf8');
const styles = fs.readFileSync(path.join(__dirname, '..', 'css', 'key-usage.css'), 'utf8');

test('公开 Key 页面展示今日实时汇总、模型占比和趋势图', () => {
  assert.match(html, /id="todayMetrics"/);
  assert.match(html, /id="modelTokenChart"/);
  assert.match(html, /id="modelCostChart"/);
  assert.match(html, /id="usageChart"/);
  assert.match(html, /echarts\.min\.js/);
  assert.doesNotMatch(html, /historyRange|historyMetrics|totalMetrics|历史统计|累计总量|历史区间/);

  assert.match(script, /\{ label: '请求次数', value: number\(today\.request_count\) \}/);
  assert.match(script, /\{ label: '总 Token', value: number\(today\.total_tokens\) \}/);
  assert.match(script, /费用上限/);
  assert.match(script, /usage_percentage/);
  assert.doesNotMatch(script, /总费用上限|无限制/);
  assert.doesNotMatch(script, /成功率|输入 Token|输出 Token|缓存读取|缓存写入|最近一分钟 RPM/);
});

test('模型 Token 和费用占比使用响应式饼图', () => {
  assert.match(script, /data\.model_usage/);
  assert.match(script, /field: 'total_tokens'/);
  assert.match(script, /field: 'effective_cost'/);
  assert.match(script, /type: 'pie'/);
  assert.match(script, /占比:/);
  assert.match(styles, /\.public-usage-pie-grid\s*\{[\s\S]*?grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\)/);
  assert.match(styles, /@media \(max-width: 760px\)[\s\S]*?\.public-usage-pie-grid\s*\{[\s\S]*?grid-template-columns:\s*1fr/);
});

test('今日趋势图使用费用和 Token 双轴序列', () => {
  assert.match(script, /window\.echarts\.init\(chartElement\)/);
  assert.match(script, /name: '费用',[\s\S]*?type: 'line'/);
  assert.match(script, /name: 'Token',[\s\S]*?type: 'bar'/);
  assert.match(script, /point\.effective_cost/);
  assert.match(script, /point\.total_tokens/);
});
