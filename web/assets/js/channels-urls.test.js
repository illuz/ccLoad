const test = require('node:test');
const assert = require('node:assert/strict');

const { createURLRow } = require('./channels-urls.js');

test('URL row restores the persisted full URL checkbox state', () => {
  const exactCheckbox = { checked: false };
  const selectionCheckbox = { checked: false };
  const row = {
    querySelector(selector) {
      if (selector === '.url-checkbox') return selectionCheckbox;
      if (selector === '.inline-url-exact-checkbox') return exactCheckbox;
      throw new Error(`unexpected selector: ${selector}`);
    }
  };
  const globals = {
    TemplateEngine: { render: () => row },
    inlineURLTableData: ['https://upstream.test/v1/messages#'],
    selectedURLIndices: new Set(),
    window: { t: key => key }
  };
  const previous = new Map(
    Object.keys(globals).map(key => [key, Object.getOwnPropertyDescriptor(global, key)])
  );

  Object.assign(global, globals);
  try {
    const renderedRow = createURLRow(0);
    assert.equal(renderedRow.querySelector('.inline-url-exact-checkbox').checked, true);
  } finally {
    for (const [key, descriptor] of previous) {
      if (descriptor) Object.defineProperty(global, key, descriptor);
      else delete global[key];
    }
  }
});
