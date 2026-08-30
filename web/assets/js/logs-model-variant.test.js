const test = require('node:test');
const assert = require('node:assert/strict');

const escapeHtmlStub = value => String(value);
global.escapeHtml = escapeHtmlStub;
global.window = {
  t: key => key,
  escapeHtml: escapeHtmlStub,
  initPageBootstrap() {},
  addEventListener() {}
};
global.document = { addEventListener() {} };
global.localStorage = { getItem: () => null, setItem() {}, removeItem() {} };

const { isPrefixOrSuffixVariant, buildLogModelDisplay } = require('./logs.js');

test('prefix and suffix model variants do not render a redirect marker', () => {
  for (const [requested, actual] of [
    ['gemini-3.6-flash-high', 'gemini-3.6-flash'],
    ['deepseek-v4-pro-0813', 'provider/deepseek-v4-pro-0813'],
    ['deepseek-v4-pro-0813', 'deepseek-v4-pro-ga-260813']
  ]) {
    assert.equal(isPrefixOrSuffixVariant(requested, actual), true);
    const html = buildLogModelDisplay(requested, actual, '', 0, '', false);
    assert.equal(html.includes('redirect-badge'), false);
    assert.equal(html.includes('model-redirected'), false);
  }
});

test('different models still render redirect and upstream mismatch markers', () => {
  assert.equal(isPrefixOrSuffixVariant('gpt-4o', 'claude-sonnet-4'), false);
  const html = buildLogModelDisplay('gpt-4o', 'claude-sonnet-4', '', 0, 'other-model', true);
  assert.equal(html.includes('redirect-badge'), true);
  assert.equal(html.includes('model-upstream-mismatch'), true);
});
