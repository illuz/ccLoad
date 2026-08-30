// 系统设置页面
const t = window.t;

let originalSettings = {}; // 保存原始值用于比较
let debugPreserveTokenOptions = [];
let runtimeMetricsLoading = false;
let runtimeMetricsPreviousFocus = null;
let runtimeMetricsRefreshTimer = null;
const RUNTIME_METRICS_REFRESH_MS = 3000;

function isHotReloadableSetting(setting) {
  return Boolean(setting?.hot_reload);
}

function renderHotReloadBadge(setting) {
  if (!isHotReloadableSetting(setting)) return '';
  const title = escapeHtml(t('settings.hotReload'));
  return `<span class="settings-hot-reload" title="${title}" aria-label="${title}">
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m13 2-9.5 12H12l-1 8 9.5-12H12l1-8z" />
    </svg>
  </span>`;
}

const runtimeMetricDomains = [
  {
    sourceKey: 'process',
    titleKey: 'settings.runtimeMetrics.group.process',
    descriptionKey: 'settings.runtimeMetrics.processNote',
    metrics: [
      { key: 'uptime_seconds', labelKey: 'settings.runtimeMetrics.metric.uptime', format: 'duration' },
      { key: 'concurrency_slots_in_use', labelKey: 'settings.runtimeMetrics.metric.concurrencySlotsInUse' },
      { key: 'max_concurrency', labelKey: 'settings.runtimeMetrics.metric.maxConcurrency' },
      { key: 'goroutines', labelKey: 'settings.runtimeMetrics.metric.goroutines' }
    ]
  },
  {
    sourceKey: 'process',
    titleKey: 'settings.runtimeMetrics.group.resources',
    descriptionKey: 'settings.runtimeMetrics.resourcesNote',
    metrics: [
      { key: 'cpu_usage_percent', labelKey: 'settings.runtimeMetrics.metric.cpuUsagePercent', format: 'percent' },
      { key: 'cpu_user_seconds', labelKey: 'settings.runtimeMetrics.metric.cpuUserSeconds', format: 'seconds' },
      { key: 'cpu_system_seconds', labelKey: 'settings.runtimeMetrics.metric.cpuSystemSeconds', format: 'seconds' },
      { key: 'rss_bytes', labelKey: 'settings.runtimeMetrics.metric.rssBytes', format: 'bytes', zeroUnavailable: true },
      { key: 'max_rss_bytes', labelKey: 'settings.runtimeMetrics.metric.maxRssBytes', format: 'bytes', zeroUnavailable: true },
      { key: 'heap_alloc_bytes', labelKey: 'settings.runtimeMetrics.metric.heapAllocBytes', format: 'bytes' },
      { key: 'heap_sys_bytes', labelKey: 'settings.runtimeMetrics.metric.heapSysBytes', format: 'bytes' },
      { key: 'gc_count', labelKey: 'settings.runtimeMetrics.metric.gcCount' },
      { key: 'gc_pause_total_ns', labelKey: 'settings.runtimeMetrics.metric.gcPauseTotal', format: 'durationNs' },
      { key: 'gc_cpu_percent', labelKey: 'settings.runtimeMetrics.metric.gcCPUPercent', format: 'percent' }
    ]
  },
  {
    sourceKey: 'http_proxy',
    titleKey: 'settings.runtimeMetrics.group.httpProxy',
    descriptionKey: 'settings.runtimeMetrics.httpProxyNote',
    metrics: [
      { key: 'active_requests', labelKey: 'settings.runtimeMetrics.metric.httpActiveRequests' },
      { key: 'completed_requests', labelKey: 'settings.runtimeMetrics.metric.httpCompletedRequests' },
      { key: 'non_error_responses', labelKey: 'settings.runtimeMetrics.metric.httpNonErrorResponses' },
      { key: 'client_error_responses', labelKey: 'settings.runtimeMetrics.metric.httpClientErrorResponses' },
      { key: 'server_error_responses', labelKey: 'settings.runtimeMetrics.metric.httpServerErrorResponses' },
      { key: 'streaming_requests', labelKey: 'settings.runtimeMetrics.metric.httpStreamingRequests' },
      { key: 'non_streaming_requests', labelKey: 'settings.runtimeMetrics.metric.httpNonStreamingRequests' },
      { key: 'request_body_bytes', labelKey: 'settings.runtimeMetrics.metric.httpRequestBodyBytes', format: 'bytes' },
      { key: 'response_body_bytes', labelKey: 'settings.runtimeMetrics.metric.httpResponseBodyBytes', format: 'bytes' }
    ]
  },
  {
    sourceKey: 'logs',
    titleKey: 'settings.runtimeMetrics.group.logs',
    descriptionKey: 'settings.runtimeMetrics.logsNote',
    metrics: [
      { key: 'backlog_entries', labelKey: 'settings.runtimeMetrics.metric.logBacklogEntries' },
      { key: 'queue_capacity_entries', labelKey: 'settings.runtimeMetrics.metric.logQueueCapacityEntries' },
      { key: 'dropped_entries', labelKey: 'settings.runtimeMetrics.metric.logDroppedEntries' },
      { key: 'persistence_failed_entries', labelKey: 'settings.runtimeMetrics.metric.logPersistenceFailedEntries' }
    ]
  },
  {
    sourceKey: 'storage',
    titleKey: 'settings.runtimeMetrics.group.storage',
    descriptionKey: 'settings.runtimeMetrics.storageNote',
    optional: true,
    metrics: [
      { key: 'sqlite_sync_failures', labelKey: 'settings.runtimeMetrics.metric.sqliteSyncFailures' },
      { key: 'mysql_sync_pending', labelKey: 'settings.runtimeMetrics.metric.mysqlSyncPending' },
      { key: 'mysql_sync_queue_capacity', labelKey: 'settings.runtimeMetrics.metric.mysqlSyncQueueCapacity' },
      { key: 'mysql_sync_failures', labelKey: 'settings.runtimeMetrics.metric.mysqlSyncFailures' },
      { key: 'mysql_sync_dropped', labelKey: 'settings.runtimeMetrics.metric.mysqlSyncDropped' },
      { key: 'mysql_sync_last_success_unix_ms', labelKey: 'settings.runtimeMetrics.metric.mysqlSyncLastSuccess', format: 'unixMilliseconds' }
    ]
  }
];

function bindSettingsPageActions() {
  const saveAllBtn = document.getElementById('save-all-btn');
  if (saveAllBtn && !saveAllBtn.dataset.bound) {
    saveAllBtn.addEventListener('click', () => {
      saveAllSettings();
    });
    saveAllBtn.dataset.bound = '1';
  }

  const saveRestartBtn = document.getElementById('save-restart-btn');
  if (saveRestartBtn && !saveRestartBtn.dataset.bound) {
    saveRestartBtn.addEventListener('click', () => {
      saveAndRestartSettings();
    });
    saveRestartBtn.dataset.bound = '1';
  }

  const runtimeMetricsBtn = document.getElementById('runtime-metrics-btn');
  if (runtimeMetricsBtn && !runtimeMetricsBtn.dataset.bound) {
    runtimeMetricsBtn.addEventListener('click', openRuntimeMetricsModal);
    runtimeMetricsBtn.dataset.bound = '1';
  }

  const refreshBtn = document.getElementById('refresh-runtime-metrics-btn');
  if (refreshBtn && !refreshBtn.dataset.bound) {
    refreshBtn.addEventListener('click', loadRuntimeMetrics);
    refreshBtn.dataset.bound = '1';
  }

  document.querySelectorAll('[data-action="close-runtime-metrics"]').forEach((button) => {
    if (button.dataset.bound) return;
    button.addEventListener('click', closeRuntimeMetricsModal);
    button.dataset.bound = '1';
  });

  const modal = document.getElementById('runtimeMetricsModal');
  if (modal && !modal.dataset.bound) {
    modal.addEventListener('click', (event) => {
      if (event.target === modal) closeRuntimeMetricsModal();
    });
    modal.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeRuntimeMetricsModal();
      }
    });
    modal.dataset.bound = '1';
  }
}

function openRuntimeMetricsModal() {
  const modal = document.getElementById('runtimeMetricsModal');
  if (!modal) return;

  runtimeMetricsPreviousFocus = document.activeElement;
  document.querySelector('.app-container')?.setAttribute('inert', '');
  modal.classList.add('show');
  modal.setAttribute('aria-hidden', 'false');
  modal.querySelector('.close-btn')?.focus();
  loadRuntimeMetrics();
  if (runtimeMetricsRefreshTimer === null) {
    runtimeMetricsRefreshTimer = setInterval(() => loadRuntimeMetrics({ silent: true }), RUNTIME_METRICS_REFRESH_MS);
  }
}

function closeRuntimeMetricsModal() {
  const modal = document.getElementById('runtimeMetricsModal');
  if (!modal) return;

  if (runtimeMetricsRefreshTimer !== null) {
    clearInterval(runtimeMetricsRefreshTimer);
    runtimeMetricsRefreshTimer = null;
  }
  modal.classList.remove('show');
  modal.setAttribute('aria-hidden', 'true');
  document.querySelector('.app-container')?.removeAttribute('inert');
  if (runtimeMetricsPreviousFocus?.isConnected) runtimeMetricsPreviousFocus.focus();
  runtimeMetricsPreviousFocus = null;
}

function normalizeRuntimeMetric(value) {
  if (value === null || value === undefined || value === '') return null;
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric >= 0 ? numeric : null;
}

function runtimeMetricsLocale() {
  return window.i18n?.getLocale?.() || document.documentElement.lang || 'zh-CN';
}

function formatRuntimeInteger(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null) return 'N/A';
  return new Intl.NumberFormat(runtimeMetricsLocale(), { maximumFractionDigits: 0 }).format(numeric);
}

function formatRuntimeDecimal(value, maximumFractionDigits = 1) {
  return new Intl.NumberFormat(runtimeMetricsLocale(), { maximumFractionDigits }).format(value);
}

function formatRuntimeBytes(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null) return 'N/A';

  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let unitIndex = 0;
  let amount = numeric;
  while (amount >= 1024 && unitIndex < units.length - 1) {
    amount /= 1024;
    unitIndex++;
  }
  return `${formatRuntimeDecimal(amount, unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatRuntimeDuration(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null) return 'N/A';

  const seconds = Math.round(numeric);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return t('common.timeHM', { h: hours, m: minutes });
  if (minutes > 0) return t('common.timeMS', { m: minutes, s: seconds % 60 });
  return t('common.timeS', { s: seconds });
}

function formatRuntimeSeconds(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null) return 'N/A';
  if (numeric < 60) return t('common.timeS', { s: formatRuntimeDecimal(numeric, 1) });
  return formatRuntimeDuration(numeric);
}

function formatRuntimeDurationNs(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null) return 'N/A';
  if (numeric < 1e9) return `${formatRuntimeDecimal(numeric / 1e6, 1)} ms`;
  return formatRuntimeDuration(numeric / 1e9);
}

function formatRuntimeTimestamp(value) {
  const numeric = normalizeRuntimeMetric(value);
  if (numeric === null || numeric <= 0) return 'N/A';
  const date = new Date(numeric);
  return Number.isNaN(date.getTime()) ? 'N/A' : date.toLocaleString(runtimeMetricsLocale());
}

function formatRuntimeMetric(metric, stats) {
  if (metric.zeroUnavailable && normalizeRuntimeMetric(stats[metric.key]) === 0) return 'N/A';
  if (metric.format === 'bytes') return formatRuntimeBytes(stats[metric.key]);
  if (metric.format === 'duration') return formatRuntimeDuration(stats[metric.key]);
  if (metric.format === 'seconds') return formatRuntimeSeconds(stats[metric.key]);
  if (metric.format === 'durationNs') return formatRuntimeDurationNs(stats[metric.key]);
  if (metric.format === 'percent') {
    const numeric = normalizeRuntimeMetric(stats[metric.key]);
    return numeric === null ? 'N/A' : `${formatRuntimeDecimal(numeric, 1)}%`;
  }
  if (metric.format === 'unixMilliseconds') return formatRuntimeTimestamp(stats[metric.key]);
  return formatRuntimeInteger(stats[metric.key]);
}

function renderRuntimeMetricDomain(domain, payload) {
  const stats = payload[domain.sourceKey];
  if (!stats || typeof stats !== 'object' || Array.isArray(stats)) {
    if (domain.optional) return '';
    return `
      <section class="runtime-metrics-section">
        <div class="runtime-metrics-section-header"><h3>${escapeHtml(t(domain.titleKey))}</h3></div>
        <p class="runtime-metrics-note">${escapeHtml(t('settings.runtimeMetrics.groupUnavailable'))}</p>
      </section>`;
  }

  const cards = domain.metrics.map((metric) => `
    <div class="runtime-metric-card">
      <span class="runtime-metric-label">${escapeHtml(t(metric.labelKey))}</span>
      <strong class="runtime-metric-value">${escapeHtml(formatRuntimeMetric(metric, stats))}</strong>
      <code class="runtime-metric-key">${escapeHtml(metric.key)}</code>
    </div>`).join('');

  return `
    <section class="runtime-metrics-section">
      <div class="runtime-metrics-section-header"><h3>${escapeHtml(t(domain.titleKey))}</h3></div>
      <p class="runtime-metrics-section-description">${escapeHtml(t(domain.descriptionKey))}</p>
      <div class="runtime-metrics-grid">${cards}</div>
    </section>`;
}

function renderRuntimeMetrics(payload) {
  const content = document.getElementById('runtime-metrics-content');
  if (!content) return;
  content.innerHTML = runtimeMetricDomains.map((domain) => renderRuntimeMetricDomain(domain, payload)).join('');
}

function renderRuntimeMetricsLoading() {
  const content = document.getElementById('runtime-metrics-content');
  if (!content) return;
  content.innerHTML = `
    <div class="runtime-metrics-message">
      <span class="loading-spinner" aria-hidden="true"></span>
      <span>${escapeHtml(t('settings.runtimeMetrics.loading'))}</span>
    </div>`;
}

function renderRuntimeMetricsError(error) {
  const content = document.getElementById('runtime-metrics-content');
  if (!content) return;
  const message = error?.message || t('settings.runtimeMetrics.loadFailed');
  content.innerHTML = `
    <div class="runtime-metrics-message runtime-metrics-message--error" role="alert">
      <strong>${escapeHtml(t('settings.runtimeMetrics.loadFailed'))}</strong>
      <span>${escapeHtml(message)}</span>
    </div>`;
}

async function loadRuntimeMetrics(options) {
  if (runtimeMetricsLoading) return;
  const silent = options?.silent === true;
  const content = document.getElementById('runtime-metrics-content');
  const refreshBtn = document.getElementById('refresh-runtime-metrics-btn');
  const updatedAt = document.getElementById('runtime-metrics-updated-at');

  runtimeMetricsLoading = true;
  if (content) content.setAttribute('aria-busy', 'true');
  if (!silent) {
    if (refreshBtn) refreshBtn.disabled = true;
    if (updatedAt) updatedAt.textContent = '';
    renderRuntimeMetricsLoading();
  }

  try {
    const data = await fetchDataWithAuth('/admin/runtime-metrics');
    if (!data || typeof data !== 'object' || Array.isArray(data)) {
      throw new Error(t('settings.runtimeMetrics.invalidResponse'));
    }
    renderRuntimeMetrics(data);
    if (updatedAt) {
      updatedAt.textContent = t('settings.runtimeMetrics.updatedAt', {
        time: new Date().toLocaleString(runtimeMetricsLocale())
      });
    }
  } catch (error) {
    console.error('Failed to load runtime metrics:', error);
    renderRuntimeMetricsError(error);
  } finally {
    runtimeMetricsLoading = false;
    if (content) content.setAttribute('aria-busy', 'false');
    if (refreshBtn) refreshBtn.disabled = false;
  }
}

function getSettingGroupInfo(key) {
  const k = String(key || '').toLowerCase();

  const defs = [
    { id: 'channel', nameKey: 'settings.group.channel', order: 10, match: () => k.startsWith('channel_') || k === 'max_key_retries' },
    { id: 'model', nameKey: 'settings.group.model', order: 15, match: () => k.startsWith('model_') },
    { id: 'upstream-connection', nameKey: 'settings.group.upstreamConnection', order: 19, match: () => k === 'upstream_connection_reuse_limit_seconds' },
    { id: 'timeout', nameKey: 'settings.group.timeout', order: 20, match: () => k.includes('timeout') },
    { id: 'health', nameKey: 'settings.group.health', order: 30, match: () => k.includes('health_score') || k.includes('success_rate') || k.includes('penalty_weight') || k.includes('ttfb') || k === 'enable_health_score' || k === 'health_min_confident_sample' },
    { id: 'cooldown', nameKey: 'settings.group.cooldown', order: 40, match: () => k.startsWith('cooldown_') },
    { id: 'log', nameKey: 'settings.group.log', order: 50, match: () => k.startsWith('log_') || k.startsWith('debug_') || k.startsWith('soft_error_') },
    { id: 'access', nameKey: 'settings.group.access', order: 60, match: () => k.includes('auth_') },
  ];

  for (const d of defs) {
    if (d.match()) return { ...d, name: t(d.nameKey) };
  }
  return { id: 'other', nameKey: 'settings.group.other', name: t('settings.group.other'), order: 999 };
}

function getSettingOrder(key) {
  const orders = {
    upstream_connection_reuse_limit_seconds: 90,
    upstream_first_byte_timeout: 100,
    non_stream_timeout: 101,
    anthropic_first_byte_timeout: 110,
    anthropic_non_stream_timeout: 111,
    codex_first_byte_timeout: 120,
    codex_non_stream_timeout: 121,
    openai_first_byte_timeout: 130,
    openai_non_stream_timeout: 131,
    gemini_first_byte_timeout: 140,
    gemini_non_stream_timeout: 141
  };
  const normalizedKey = String(key || '').toLowerCase();
  return orders[normalizedKey] ?? 1000;
}

function groupSettings(settings) {
  const groupsById = new Map();

  for (const s of settings) {
    const g = getSettingGroupInfo(s.key);
    if (!groupsById.has(g.id)) {
      groupsById.set(g.id, { id: g.id, name: g.name, order: g.order, settings: [] });
    }
    groupsById.get(g.id).settings.push(s);
  }

  const groups = Array.from(groupsById.values())
    .sort((a, b) => a.order - b.order || a.name.localeCompare(b.name));

  for (const g of groups) {
    g.settings.sort((a, b) => {
      const orderDiff = getSettingOrder(a.key) - getSettingOrder(b.key);
      if (orderDiff !== 0) return orderDiff;
      return String(a.key).localeCompare(String(b.key));
    });
  }

  return groups;
}

function renderGroupNav(groups) {
  const nav = document.getElementById('settings-group-nav');
  const navSection = document.getElementById('settings-group-nav-section');
  if (!nav) return;

  nav.innerHTML = '';
  const hasMultipleGroups = Array.isArray(groups) && groups.length > 1;
  if (navSection) navSection.hidden = !hasMultipleGroups;
  if (!hasMultipleGroups) return;

  for (let i = 0; i < groups.length; i++) {
    const g = groups[i];
    const btn = document.createElement('button');
    btn.className = 'time-range-btn' + (i === 0 ? ' active' : '');
    btn.textContent = g.name;
    btn.addEventListener('click', () => {
      // 移除所有按钮的 active 状态
      nav.querySelectorAll('.time-range-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      // 滚动到对应分组
      const target = document.getElementById(`settings-group-${g.id}`);
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
    nav.appendChild(btn);
  }
}

async function loadSettings() {
  try {
    const [data, tokenData] = await Promise.all([
      fetchDataWithAuth('/admin/settings'),
      fetchDataWithAuth('/admin/auth-tokens').catch(() => ({ tokens: [] }))
    ]);
    if (!Array.isArray(data)) throw new Error(t('settings.msg.invalidResponse'));
    debugPreserveTokenOptions = Array.isArray(tokenData?.tokens) ? tokenData.tokens : [];
    renderSettings(data);
  } catch (err) {
    console.error('Failed to load settings:', err);
    showError(t('settings.msg.loadFailed') + ': ' + err.message);
  }
}

function renderSettings(settings) {
  const tbody = document.getElementById('settings-tbody');
  originalSettings = {};
  tbody.innerHTML = '';

  // 初始化事件委托（仅一次）
  initSettingsEventDelegation();

  const groups = groupSettings(settings);
  renderGroupNav(groups);

  for (const g of groups) {
    const groupRow = TemplateEngine.render('tpl-setting-group-row', {
      groupId: g.id,
      groupName: g.name
    });
    if (groupRow) tbody.appendChild(groupRow);

    for (const s of g.settings) {
      originalSettings[s.key] = s.value;
      // 优先使用语言包中的描述，若没有则回退到后端返回的描述
      const descKey = `settings.desc.${s.key}`;
      const translatedDesc = t(descKey);
      const description = (translatedDesc !== descKey) ? translatedDesc : s.description;
      const row = TemplateEngine.render('tpl-setting-row', {
        key: s.key,
        description: description,
        hotReloadHtml: renderHotReloadBadge(s),
        inputHtml: renderInput(s),
        mobileLabelDescription: t('settings.configItem'),
        mobileLabelValue: t('settings.currentValue'),
        mobileLabelActions: t('common.actions')
      });
      if (row) tbody.appendChild(row);
    }
  }
}

// 初始化事件委托（替代 inline onclick）
function initSettingsEventDelegation() {
  const tbody = document.getElementById('settings-tbody');
  if (!tbody || tbody.dataset.delegated) return;
  tbody.dataset.delegated = 'true';

  // 重置按钮点击
  tbody.addEventListener('click', (e) => {
    const resetBtn = e.target.closest('.setting-reset-btn');
    if (resetBtn) {
      resetSetting(resetBtn.dataset.key);
    }
  });

  // 输入变更
  tbody.addEventListener('change', (e) => {
    const input = e.target.closest('input, textarea, select');
    if (input) markChanged(input);
  });
}

function renderInput(setting) {
  const safeKey = escapeHtml(setting.key);
  const safeValue = escapeHtml(setting.value);

  if (setting.key === 'soft_error_text_prefixes') {
    return `<textarea id="${safeKey}" class="settings-input settings-input--textarea">${safeValue}</textarea>`;
  }

  if (setting.value_type === 'auth_token_id') {
    const options = [`<option value="0" ${String(setting.value) === '0' ? 'selected' : ''}>${escapeHtml(t('settings.debugPreserveNone'))}</option>`];
    let hasCurrent = String(setting.value) === '0';
    for (const token of debugPreserveTokenOptions) {
      const id = String(token?.id ?? '');
      if (!/^\d+$/.test(id) || id === '0') continue;
      const selected = id === String(setting.value);
      if (selected) hasCurrent = true;
      const label = formatDebugPreserveTokenLabel(token);
      options.push(`<option value="${escapeHtml(id)}" ${selected ? 'selected' : ''}>${escapeHtml(label)}</option>`);
    }
    if (!hasCurrent) {
      options.push(`<option value="${safeValue}" selected>${escapeHtml(t('settings.debugPreserveUnknown', { id: setting.value }))}</option>`);
    }
    return `<select id="${safeKey}" class="settings-input settings-input--text">${options.join('')}</select>`;
  }

  switch (setting.value_type) {
    case 'bool':
      const isTrue = setting.value === 'true' || setting.value === '1';
      return `
        <div class="settings-bool-group">
          <label class="settings-bool-option">
            <input type="radio" name="${safeKey}" value="true" ${isTrue ? 'checked' : ''}> ${t('common.enable')}
          </label>
          <label class="settings-bool-option">
            <input type="radio" name="${safeKey}" value="false" ${!isTrue ? 'checked' : ''}> ${t('common.disable')}
          </label>
        </div>`;
    case 'int':
    case 'duration':
      return `<input type="number" id="${safeKey}" value="${safeValue}" class="settings-input settings-input--number">`;
    case 'float':
      return `<input type="number" step="any" id="${safeKey}" value="${safeValue}" class="settings-input settings-input--number">`;
    default:
      return `<input type="text" id="${safeKey}" value="${safeValue}" class="settings-input settings-input--text">`;
  }
}

function formatDebugPreserveTokenLabel(token) {
  const id = String(token?.id ?? '');
  const description = String(token?.description || '').trim();
  const value = String(token?.plain_token || '').trim();
  let masked = '';
  if (value.length > 12) masked = `${value.slice(0, 6)}...${value.slice(-4)}`;
  else if (value) masked = `${value.slice(0, Math.min(4, value.length))}...`;
  return [description || `Token #${id}`, masked].filter(Boolean).join(' - ');
}

function markChanged(input) {
  const row = input.closest('tr');
  let key, currentValue;

  if (input.type === 'radio') {
    key = input.name;
    const checkedRadio = row.querySelector(`input[name="${key}"]:checked`);
    currentValue = checkedRadio ? checkedRadio.value : '';
  } else {
    key = input.id;
    currentValue = input.value;
  }

  if (currentValue !== originalSettings[key]) {
    row.style.background = 'rgba(59, 130, 246, 0.08)';
  } else {
    row.style.background = '';
  }
}

function getSettingControl(key) {
  const input = document.getElementById(key);
  if (input) {
    return {
      input,
      row: input.closest('tr'),
      value: input.value
    };
  }

  const radios = document.querySelectorAll(`input[name="${key}"]`);
  if (radios.length === 0) return null;

  const checkedRadio = document.querySelector(`input[name="${key}"]:checked`);
  return {
    input: radios[0],
    radios,
    row: radios[0].closest('tr'),
    value: checkedRadio ? checkedRadio.value : ''
  };
}

function syncSettingState(key, value) {
  const normalizedValue = String(value);
  const control = getSettingControl(key);

  if (control?.radios) {
    for (const radio of control.radios) {
      radio.checked = radio.value === normalizedValue
        || (normalizedValue === '1' && radio.value === 'true')
        || (normalizedValue === '0' && radio.value === 'false');
    }
  } else if (control?.input) {
    control.input.value = normalizedValue;
  }

  originalSettings[key] = normalizedValue;
  if (control?.row) {
    control.row.style.background = '';
  }
}

async function saveAllSettings() {
  const updates = collectSettingUpdates();

  if (Object.keys(updates).length === 0) {
    window.showNotification(t('settings.msg.noChanges'), 'info');
    return;
  }

  // 使用批量更新接口（单次请求，事务保护）
  try {
    const result = await fetchDataWithAuth('/admin/settings/batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates)
    });

    for (const [key, value] of Object.entries(updates)) {
      syncSettingState(key, value);
    }

    showSuccess(result?.message || t('settings.msg.savedCount', { count: Object.keys(updates).length }));
  } catch (err) {
    console.error('保存异常:', err);
    showError(t('settings.msg.saveFailed') + ': ' + err.message);
  }
}

function collectSettingUpdates() {
  const updates = {};
  for (const key of Object.keys(originalSettings)) {
    const control = getSettingControl(key);
    if (control && control.value !== originalSettings[key]) updates[key] = control.value;
  }
  return updates;
}

async function saveAndRestartSettings() {
  const updates = collectSettingUpdates();
  try {
    const result = await fetchDataWithAuth('/admin/settings/save-restart', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates)
    });
    for (const [key, value] of Object.entries(updates)) syncSettingState(key, value);
    showSuccess(result?.message || t('settings.msg.saveAndRestartSuccess'));
  } catch (err) {
    console.error('保存并重启异常:', err);
    showError(t('settings.msg.saveFailed') + ': ' + err.message);
  }
}

async function resetSetting(key) {
  if (!confirm(t('settings.msg.confirmReset', { key }))) return;

  try {
    const result = await fetchDataWithAuth(`/admin/settings/${key}/reset`, { method: 'POST' });
    syncSettingState(key, result?.value ?? '');
    showSuccess(result?.message || t('settings.msg.resetSuccess', { key }));
  } catch (err) {
    console.error('重置异常:', err);
    showError(t('settings.msg.resetFailed') + ': ' + err.message);
  }
}

window.initPageBootstrap({
  topbarKey: 'settings',
  run: () => {
    bindSettingsPageActions();
    loadSettings();
  }
});
