const test = require('node:test');
const assert = require('node:assert/strict');

const { selectFirstEnabledInlineKey } = require('./channels-keys.js');

function loadChannelModals(windowOverrides = {}) {
  const previousWindow = Object.getOwnPropertyDescriptor(global, 'window');
  Object.defineProperty(global, 'window', {
    configurable: true,
    writable: true,
    value: { ChannelProtocolConfig: {}, ...windowOverrides }
  });
  const modulePath = require.resolve('./channels-modals.js');
  delete require.cache[modulePath];

  try {
    return {
      mod: require(modulePath),
      restore() {
        delete require.cache[modulePath];
        if (previousWindow) Object.defineProperty(global, 'window', previousWindow);
        else delete global.window;
      }
    };
  } catch (error) {
    if (previousWindow) Object.defineProperty(global, 'window', previousWindow);
    else delete global.window;
    throw error;
  }
}

function installEditChannelGlobals(channel, { editorError = null } = {}) {
  const requests = [];
  const errors = [];
  const appliedURLStats = [];
  const elements = new Map();
  const makeElement = () => {
    const classes = new Set();
    return {
      value: '',
      checked: false,
      disabled: false,
      hidden: false,
      style: {},
      dataset: {},
      classList: {
        add: (...names) => names.forEach(name => classes.add(name)),
        remove: (...names) => names.forEach(name => classes.delete(name)),
        contains: name => classes.has(name)
      },
      setAttribute() {},
      addEventListener() {},
      appendChild() {}
    };
  };
  const getElement = id => {
    if ([
      'channelGroup',
      'channelScheduledCheckEnabledWrapper',
      'channelScheduledCheckModelWrapper',
      'protocolTransformsContainer',
      'protocolTransformModeContainer'
    ].includes(id)) return null;
    if (!elements.has(id)) elements.set(id, makeElement());
    return elements.get(id);
  };
  const globals = {
    document: {
      getElementById: getElement,
      querySelector: () => null,
      querySelectorAll: () => []
    },
    editingChannelId: null,
    currentChannelKeyCooldowns: [],
    inlineKeyTableData: [{ api_key: '' }],
    inlineKeyVisible: false,
    selectedModelIndices: new Set(),
    currentModelFilter: '',
    console: { ...console, error() {} },
    TemplateEngine: { render: () => null },
    fetchDataWithAuth: async url => {
      requests.push(url);
      if (editorError) throw editorError;
      return {
        channel,
        keys: [],
        model_stats: { available: true, items: [] },
        url_stats: {
          available: true,
          items: [{ url: channel.url, latency_ms: 125, requests: 1, failures: 0 }]
        },
        features: { scheduled_check_enabled: true }
      };
    },
    clearChannelDuplicateHint() {},
    setInlineURLTableData() {},
    applyURLStats(stats) { appliedURLStats.push(stats); },
    setInlineKeyTableDataFromAPI() {
      global.inlineKeyTableData = [{ api_key: '' }];
    },
    renderInlineKeyTable() {},
    renderRedirectTable() {},
    resetChannelFormDirty() {}
  };
  const previous = new Map();
  for (const [name, value] of Object.entries(globals)) {
    previous.set(name, Object.getOwnPropertyDescriptor(global, name));
    Object.defineProperty(global, name, { configurable: true, writable: true, value });
  }

  return {
    appliedURLStats,
    errors,
    getElement,
    requests,
    window: {
      t: key => key,
      showError: message => errors.push(message),
      ChannelTypeManager: { renderChannelTypeRadios: async () => {} }
    },
    restore() {
      for (const [name, descriptor] of previous) {
        if (descriptor) Object.defineProperty(global, name, descriptor);
        else delete global[name];
      }
    }
  };
}

test('mergeCommonModels de-duplicates remote models and preserves existing rows', () => {
  const runtime = loadChannelModals();
  try {
    const existing = {
      model: 'gpt-5.6',
      redirect_model: 'kept-target',
      fixed_cost_per_request: '0.25'
    };
    const merged = runtime.mod.mergeCommonModels(
      [existing],
      ['GPT-5.6', 'gpt-5.7', 'gpt-5.7'],
      ['gpt-5.4']
    );

    assert.deepEqual(merged.rows, [
      existing,
      { model: 'gpt-5.7', redirect_model: '', fixed_cost_per_request: '', disabled: false }
    ]);
    assert.notEqual(merged.rows[0], existing);
    assert.equal(merged.added, 1);
  } finally {
    runtime.restore();
  }
});

test('mergeCommonModels falls back to embedded models with local pricing defaults', () => {
  const runtime = loadChannelModals();
  try {
    const merged = runtime.mod.mergeCommonModels([], [], ['claude-sonnet-5']);
    assert.deepEqual(merged.rows, [
      { model: 'claude-sonnet-5', redirect_model: '', fixed_cost_per_request: '', disabled: false }
    ]);
    assert.equal(merged.added, 1);
  } finally {
    runtime.restore();
  }
});

function installFetchModelsGlobals({ rows, states, onFetch, onError }) {
  const globals = {
    window: {
      ChannelProtocolConfig: {},
      t: key => key,
      showError: onError
    },
    document: {
      querySelector: () => ({ value: 'openai' })
    },
    getValidInlineURLs: () => ['https://upstream.test'],
    getInlineKeyRows: () => rows,
    currentChannelKeyCooldowns: states,
    selectFirstEnabledInlineKey,
    fetchAPIWithAuth: onFetch,
    alert: onError,
    console: { ...console, error: () => {} }
  };
  const previous = new Map();
  for (const [name, value] of Object.entries(globals)) {
    previous.set(name, Object.getOwnPropertyDescriptor(global, name));
    Object.defineProperty(global, name, { configurable: true, writable: true, value });
  }
  return () => {
    for (const [name, descriptor] of previous) {
      if (descriptor) Object.defineProperty(global, name, descriptor);
      else delete global[name];
    }
  };
}

function loadFetchModelsFromAPI() {
  const modulePath = require.resolve('./channels-modals.js');
  delete require.cache[modulePath];
  return require(modulePath).fetchModelsFromAPI;
}

test('fetched models preserve existing disabled state and enable new rows', () => {
  const runtime = loadChannelModals();
  try {
    const result = runtime.mod.mergeModelRowsWithFetchedModels([
      { model: 'existing-model', redirect_model: 'upstream-model', disabled: true }
    ], [
      { model: 'existing-model', redirect_model: 'ignored-replacement' },
      { model: 'UPSTREAM-MODEL', redirect_model: 'UPSTREAM-MODEL' },
      { model: 'new-model', redirect_model: 'new-upstream' }
    ]);

    assert.deepEqual(result, {
      rows: [
        { model: 'existing-model', redirect_model: 'upstream-model', disabled: true },
        { model: 'new-model', redirect_model: 'new-upstream', disabled: false }
      ],
      added: 1,
      removed: 0
    });
  } finally {
    runtime.restore();
  }
});

test('model disabled state toggles without changing the model mapping', () => {
  const runtime = loadChannelModals();
  try {
    const rows = [{ model: 'model-a', redirect_model: 'upstream-a', disabled: false }];

    assert.equal(runtime.mod.toggleModelDisabledState(rows, 0), true);
    assert.deepEqual(rows, [{ model: 'model-a', redirect_model: 'upstream-a', disabled: true }]);
    assert.equal(runtime.mod.toggleModelDisabledState(rows, 0), true);
    assert.deepEqual(rows, [{ model: 'model-a', redirect_model: 'upstream-a', disabled: false }]);
    assert.equal(runtime.mod.toggleModelDisabledState(rows, 9), false);
  } finally {
    runtime.restore();
  }
});

test('model submit payload includes disabled state and fixed cost', () => {
  const runtime = loadChannelModals();
  try {
    assert.deepEqual(runtime.mod.collectModelsForSubmit([
      { model: '  model-a  ', redirect_model: ' upstream-a ', fixed_cost_per_request: '0.25', disabled: true },
      { model: 'model-b', redirect_model: '', disabled: false },
      { model: '   ', disabled: true }
    ]), [
      { model: 'model-a', redirect_model: 'upstream-a', fixed_cost_per_request: 0.25, disabled: true },
      { model: 'model-b', redirect_model: '', fixed_cost_per_request: 0, disabled: false }
    ]);
  } finally {
    runtime.restore();
  }
});
test('fetchModelsFromAPI sends the first enabled API key', async () => {
  let requestBody;
  const restore = installFetchModelsGlobals({
    rows: [{ api_key: 'disabled-key' }, { api_key: 'enabled-key' }],
    states: [
      { key_index: 0, disabled: true },
      { key_index: 1, disabled: false }
    ],
    onFetch: async (_url, options) => {
      requestBody = JSON.parse(options.body);
      return { success: false, error: 'stop after request capture' };
    },
    onError: () => {}
  });

  try {
    await loadFetchModelsFromAPI()();
  } finally {
    restore();
  }

  assert.equal(requestBody.api_key, 'enabled-key');
});

test('fetchModelsFromAPI rejects a channel whose keys are all disabled', async () => {
  let fetchCalled = false;
  let shownError = '';
  const restore = installFetchModelsGlobals({
    rows: [{ api_key: 'disabled-key' }],
    states: [{ key_index: 0, disabled: true }],
    onFetch: async () => {
      fetchCalled = true;
      return {};
    },
    onError: message => { shownError = message; }
  });

  try {
    await loadFetchModelsFromAPI()();
  } finally {
    restore();
  }

  assert.equal(fetchCalled, false);
  assert.equal(shownError, 'channels.addAtLeastOneEnabledKey');
});

test('editing a channel loads one complete editor snapshot', async () => {
  const channel = {
    id: 73,
    name: 'single-url',
    url: 'https://single.test',
    channel_type: 'anthropic',
    request_delay_seconds: 7,
    models: [],
    priority: 100,
    enabled: true
  };
  const fixture = installEditChannelGlobals(channel);
  const runtime = loadChannelModals(fixture.window);

  try {
    await runtime.mod.editChannel(channel.id);

    assert.deepEqual(fixture.requests, [`/admin/channels/${channel.id}/editor`]);
    assert.equal(fixture.getElement('channelRequestDelaySeconds').value, '7');
    assert.equal(fixture.getElement('channelModal').classList.contains('show'), true);
    assert.equal(fixture.appliedURLStats.length, 1);
  } finally {
    runtime.restore();
    fixture.restore();
  }
});

test('editing a channel does not open a partial editor when bootstrap fails', async () => {
  const channel = {
    id: 74,
    url: 'https://failed-bootstrap.test',
    models: []
  };
  const fixture = installEditChannelGlobals(channel, { editorError: new Error('database unavailable') });
  const runtime = loadChannelModals(fixture.window);

  try {
    await runtime.mod.editChannel(channel.id);

    assert.deepEqual(fixture.requests, [`/admin/channels/${channel.id}/editor`]);
    assert.deepEqual(fixture.errors, ['channels.loadChannelsFailed']);
    assert.equal(fixture.getElement('channelModal').classList.contains('show'), false);
  } finally {
    runtime.restore();
    fixture.restore();
  }
});
