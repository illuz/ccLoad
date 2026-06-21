const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const uiSource = fs.readFileSync(path.join(__dirname, 'ui.js'), 'utf8');

function extractCommonUiHelpers(source) {
  const startMarker = '// 公共工具函数（DRY原则：消除重复代码）';
  const endMarker = '// 通用可搜索下拉选择框组件 (SearchableCombobox)';
  const start = source.indexOf(startMarker);
  const end = source.indexOf(endMarker);

  assert.notEqual(start, -1, '找不到 ui.js 公共工具函数区块起点');
  assert.notEqual(end, -1, '找不到 ui.js 公共工具函数区块终点');

  return source.slice(start, end);
}

function loadUiCommonHelpers() {
  const documentListeners = new Map();
  const windowListeners = new Map();
  const body = {
    dataset: {},
    className: '',
    classList: {
      add() {},
      contains() { return false; }
    },
    children: []
  };
  const sandbox = {
    console,
    URL,
    URLSearchParams,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    history: {
      pushState() {},
      replaceState() {}
    },
    document: {
      body,
      scripts: [],
      readyState: 'complete',
      addEventListener(type, handler) {
        documentListeners.set(type, handler);
      },
      removeEventListener() {},
      querySelector() {
        return null;
      },
      querySelectorAll() {
        return [];
      },
      getElementById() {
        return null;
      },
      createElement(tag) {
        return {
          tagName: String(tag).toUpperCase(),
          dataset: {},
          style: {},
          classList: { add() {}, contains() { return false; } },
          appendChild() {},
          setAttribute() {}
        };
      },
      importNode(node) {
        return { ...node };
      }
    },
    window: {
      location: {
        origin: 'https://example.test',
        href: 'https://example.test/web/index.html',
        pathname: '/web/index.html',
        search: '',
        hash: '',
        assign() {}
      },
      addEventListener(type, handler) {
        windowListeners.set(type, handler);
      },
      removeEventListener() {},
      scrollTo() {}
    }
  };
  sandbox.window.window = sandbox.window;
  sandbox.window.document = sandbox.document;
  sandbox.window.history = sandbox.history;
  sandbox.window.setInterval = setInterval;
  sandbox.window.clearInterval = clearInterval;
  sandbox.window.fetch = async () => ({ ok: false, status: 500, text: async () => '' });

  vm.createContext(sandbox);
  vm.runInContext(extractCommonUiHelpers(uiSource), sandbox);
  return { window: sandbox.window, documentListeners, windowListeners };
}

test('partial navigation router 暴露原生 fetch/DOMParser/main 替换入口', () => {
  assert.match(uiSource, /window\.CCPartialRouter\s*=\s*CCPartialRouter/);
  assert.match(uiSource, /async function navigate\(/);
  assert.match(uiSource, /fetchPageDocument/);
  assert.match(uiSource, /new DOMParser\(\)\.parseFromString\(html,\s*'text\/html'\)/);
  assert.match(uiSource, /querySelector\('main\.main-content'\)/);
  assert.match(uiSource, /const state = \{ ccPartial: true, pageKey: payload\.pageKey \};/);
  assert.match(uiSource, /history\.pushState\(state,\s*'',\s*target\.href\)/);
});

test('partial navigation popstate 重新挂载但不 pushState', () => {
  assert.match(uiSource, /window\.addEventListener\('popstate'/);
  assert.match(uiSource, /navigate\(window\.location\.href,\s*\{ fromPopState: true, replace: true, source: 'popstate' \}\)/);
  assert.match(uiSource, /if \(!options\.fromPopState\)/);
});

test('partial navigation fallback/main missing 不破坏静态整页兜底', () => {
  assert.match(uiSource, /function fallbackToDocumentNavigation\(rawUrl, reason\)/);
  assert.match(uiSource, /window\.location\.assign\(rawUrl\)/);
  assert.match(uiSource, /Target page missing main\.main-content/);
  assert.match(uiSource, /if \(!target \|\| !isRoutableURL\(target\.href\)\)/);
});

test('ui.js 加载后注册 CCPartialRouter 与 CCPageLifecycle，并绑定导航事件', async () => {
  const { window, documentListeners, windowListeners } = loadUiCommonHelpers();

  assert.equal(typeof window.CCPartialRouter, 'object');
  assert.equal(typeof window.CCPartialRouter.navigate, 'function');
  assert.equal(typeof window.CCPartialRouter.isRoutableURL, 'function');
  assert.equal(typeof window.CCPageLifecycle, 'object');
  assert.equal(typeof window.CCPageLifecycle.register, 'function');
  assert.equal(typeof window.CCPageLifecycle.onCleanup, 'function');
  assert.equal(typeof documentListeners.get('click'), 'function');
  assert.equal(typeof windowListeners.get('popstate'), 'function');

  const calls = [];
  window.CCPageLifecycle.register('logs', {
    mount(ctx) {
      calls.push(`mount:${ctx.pageKey}`);
      ctx.onCleanup(() => calls.push('cleanup'));
    }
  });
  await window.CCPageLifecycle.mount('logs');
  window.CCPageLifecycle.unmountCurrent();

  assert.deepEqual(calls, ['mount:logs', 'cleanup']);
});
