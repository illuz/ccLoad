const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', 'index.html'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, '..', 'css', 'styles.css'), 'utf8');

test('首页已移除 hero 标题容器', () => {
  assert.doesNotMatch(html, /class="hero-header\s+animate-slide-up"/);
  assert.doesNotMatch(html, /data-i18n="index\.heroTitle"/);
});

test('首页时间筛选栏包含自动刷新状态提示', () => {
  assert.doesNotMatch(html, /class="index-auto-refresh-panel"/);
  assert.doesNotMatch(html, /id="index-auto-refresh-status"/);
  assert.doesNotMatch(html, /id="index-auto-refresh-meta"/);
  assert.match(html, /id="index-auto-refresh-button"[\s\S]*class="index-auto-refresh-fab"/);
  assert.match(css, /\.index-auto-refresh-fab\s*\{[\s\S]*?position:\s*fixed;[\s\S]*?right:\s*24px;[\s\S]*?bottom:\s*24px;/);
  assert.match(css, /\.index-auto-refresh-fab\s*\{[\s\S]*?background:\s*color-mix\(in srgb, var\(--surface-bg-strong\) 92%, transparent\);/);
  assert.match(css, /\.index-auto-refresh-fab\.is-refreshing\s+svg\s*\{[\s\S]*?animation:\s*index-auto-refresh-spin/);
  assert.match(css, /@keyframes\s+index-auto-refresh-spin/);
  assert.match(css, /\.index-auto-refresh-fab::after\s*\{/);
  assert.doesNotMatch(css, /\.index-auto-refresh-panel\s*\{/);
  assert.doesNotMatch(css, /\.index-auto-refresh-status\s*\{/);
  assert.doesNotMatch(css, /\.index-auto-refresh-meta\s*\{/);
});

test('首页渠道卡片顺序为 Codex 在 Claude Code 前', () => {
  const codexIndex = html.indexOf('<!-- Codex -->');
  const claudeIndex = html.indexOf('<!-- Claude Code -->');
  assert.ok(codexIndex !== -1 && claudeIndex !== -1 && codexIndex < claudeIndex);
});

test('首页在渠道卡片和总览状态栏之间展示 Codex Guard 卡片', () => {
  const channelSectionEnd = html.indexOf('<!-- Codex Guard 统计卡片 -->');
  const summaryIndex = html.indexOf('<!-- 实时状态栏 - 总览 -->');
  assert.ok(channelSectionEnd !== -1 && summaryIndex !== -1 && channelSectionEnd < summaryIndex);
  assert.match(html, /id="codex-guard-card"/);
  assert.match(html, /id="codex-guard-hit-count"/);
  assert.match(html, /id="codex-guard-retry-success-count"/);
  assert.match(html, /id="codex-guard-top-reasoning"/);
  assert.match(css, /\.codex-guard-card\s*\{/);
  assert.match(css, /\.codex-guard-metrics\s*\{/);
});

test('首页 hero 标题不再使用顶部装饰线', () => {
  assert.doesNotMatch(css, /\.hero-header::before\s*\{/);
});

test('首页自定义时间弹层使用紧凑字号', () => {
  assert.match(css, /\.custom-range-summary\s*\{[\s\S]*?font-size:\s*14px;/);
  assert.match(css, /\.custom-range-calendar-title\s*\{[\s\S]*?font-size:\s*16px;/);
  assert.match(css, /\.custom-range-weekdays span\s*\{[\s\S]*?font-size:\s*13px;/);
  assert.match(css, /\.custom-range-day\s*\{[\s\S]*?font-family:\s*inherit;[\s\S]*?font-size:\s*14px;/);
  assert.match(css, /\.custom-range-time-row input\s*\{[\s\S]*?font-family:\s*inherit;[\s\S]*?font-size:\s*13px;/);
  assert.match(css, /\.custom-range-link-btn,\s*[\r\n\s]*\.custom-range-confirm-btn\s*\{[\s\S]*?font-family:\s*inherit;[\s\S]*?font-size:\s*13px;/);
});

test('自定义时间弹层未来日期使用禁用态样式', () => {
  assert.match(css, /\.custom-range-day\.disabled,\s*[\r\n\s]*\.custom-range-day:disabled\s*\{[\s\S]*?cursor:\s*not-allowed;/);
  assert.match(css, /\.custom-range-day\.disabled:hover,\s*[\r\n\s]*\.custom-range-day:disabled:hover\s*\{[\s\S]*?background:\s*transparent;/);
});

test('筛选栏自定义时间弹层从筛选控件左侧展开', () => {
  assert.match(css, /\.filter-custom-range-host\s+\.custom-range-picker\s*\{[\s\S]*?left:\s*0;[\s\S]*?right:\s*auto;/);
});

test('筛选栏允许自定义时间弹层覆盖后续内容', () => {
  assert.match(css, /\.filter-bar\s*\{[\s\S]*?position:\s*relative;[\s\S]*?z-index:\s*20;[\s\S]*?overflow:\s*visible;/);
});
