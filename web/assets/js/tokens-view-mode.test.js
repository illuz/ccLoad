const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const script = fs.readFileSync(path.join(__dirname, 'tokens.js'), 'utf8');

function makeClassList() {
  return {
    _set: new Set(),
    toggle(name, force) {
      if (force) this._set.add(name);
      else this._set.delete(name);
    },
    contains(name) {
      return this._set.has(name);
    }
  };
}

function makeElement(tag = 'div') {
  return {
    tagName: tag.toUpperCase(),
    children: [],
    dataset: {},
    style: {},
    className: '',
    classList: makeClassList(),
    _html: '',
    appendChild(child) {
      this.children.push(child);
      return child;
    },
    set innerHTML(value) {
      this._html = String(value);
      this.children = [];
    },
    get innerHTML() {
      return this._html;
    },
    get firstElementChild() {
      return this.children[0] || null;
    }
  };
}

function loadHarness() {
  const elements = {
    'tokens-container': makeElement('div'),
    'empty-state': Object.assign(makeElement('div'), {
      querySelector() {
        return null;
      }
    }),
    'tokenListViewBtn': makeElement('button'),
    'tokenGroupViewBtn': makeElement('button'),
    'tokenSearchInput': { value: '', dataset: {}, addEventListener() {} }
  };

  const document = {
    body: { dataset: {} },
    readyState: 'complete',
    addEventListener() {},
    createElement(tag) {
      return makeElement(tag);
    },
    getElementById(id) {
      return elements[id] || null;
    },
    querySelectorAll() {
      return [];
    }
  };

  const localStorage = {
    _store: new Map([['tokens.viewMode', 'list']]),
    getItem(key) {
      return this._store.has(key) ? this._store.get(key) : null;
    },
    setItem(key, value) {
      this._store.set(key, String(value));
    },
    removeItem(key) {
      this._store.delete(key);
    }
  };

  const windowObj = {
    t(key) {
      return key;
    },
    initPageBootstrap() {},
    bindTimeRangeSelector() {},
    initDelegatedActions() {},
    createAutoRefresh() {
      return { init() {} };
    },
    i18n: {
      onLocaleChange() {},
      translatePage() {},
      getLocale() {
        return 'en';
      }
    },
    fetchDataWithAuth: async () => ({ tokens: [], groups: [], total_count: 0, is_today: false }),
    buildDateRangeQuery() {
      return '';
    },
    copyToClipboard: async () => {},
    showNotification() {},
    showError() {},
    showWarning() {},
    showSuccess() {}
  };

  const sandbox = {
    console,
    document,
    window: windowObj,
    localStorage,
    URLSearchParams,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    Promise,
    JSON,
    Math,
    Number,
    String,
    Boolean,
    Date,
    Array,
    Object,
    Map,
    Set,
    escapeHtml: (value) => String(value),
    fetchDataWithAuth: windowObj.fetchDataWithAuth,
    buildDateRangeQuery: windowObj.buildDateRangeQuery,
    copyToClipboard: windowObj.copyToClipboard
  };

  vm.createContext(sandbox);
  vm.runInContext(script, sandbox);

  return { sandbox, elements, localStorage };
}

test('tokens 视图切换会在列表/分组之间切换渲染', () => {
  const { sandbox, elements, localStorage } = loadHarness();

  vm.runInContext(`
    allTokens = [
      { id: 1, group_id: 1, group_name: 'Group A', description: 'alpha', plain_token: 'aaaa', token: 'hash1', is_active: true },
      { id: 2, group_id: 1, group_name: 'Group A', description: 'beta', plain_token: 'bbbb', token: 'hash2', is_active: true },
      { id: 3, group_id: 0, description: 'ung', plain_token: 'cccc', token: 'hash3', is_active: true }
    ];
    authTokenGroups = [{ id: 1, name: 'Group A', cost_limit_usd: 1.5, max_concurrency: 2, allowed_channel_ids: [1], allowed_models: ['m1'] }];
  `, sandbox);

  vm.runInContext('renderTokens();', sandbox);
  assert.equal(elements['tokens-container'].children[0].className, 'mobile-card-table tokens-table');
  assert.equal(elements['tokenListViewBtn'].classList.contains('active'), true);
  assert.equal(elements['tokenGroupViewBtn'].classList.contains('active'), false);

  vm.runInContext('setTokenViewMode("group");', sandbox);
  assert.equal(localStorage.getItem('tokens.viewMode'), 'group');
  assert.equal(elements['tokens-container'].children[0].className, 'token-grouped-view');
  assert.equal(elements['tokenListViewBtn'].classList.contains('active'), false);
  assert.equal(elements['tokenGroupViewBtn'].classList.contains('active'), true);

  vm.runInContext('setTokenViewMode("list");', sandbox);
  assert.equal(elements['tokens-container'].children[0].className, 'mobile-card-table tokens-table');
  assert.equal(elements['tokenListViewBtn'].classList.contains('active'), true);
  assert.equal(elements['tokenGroupViewBtn'].classList.contains('active'), false);
});
