(function () {
  'use strict';

  const ROOT_ID = 'logsChannelPanel';
  const COLLAPSED_GROUPS_KEY = 'ccload.logs.channelPanel.collapsedGroups';
  const ACTIVE_TAB_KEY = 'ccload.logs.channelPanel.activeTab';
  const CHANNEL_TAB = 'channels';
  const TOKEN_TAB = 'tokens';
  const REFRESH_MAX_AGE_MS = 15000;
  const RECENT_CACHE_WINDOW_MS = 30 * 60 * 1000;
  const RECENT_CACHE_BUCKET_MS = 60 * 1000;
  const RECENT_REQUEST_CACHE_COUNT = 50;
  const GROUP_COLORS = new Set([
    '#64748b', '#ef4444', '#f97316', '#f59e0b',
    '#22c55e', '#14b8a6', '#3b82f6', '#8b5cf6'
  ]);

  const FALLBACK_TEXT = Object.freeze({
    'logs.channelPanel.title': 'Channel controls',
    'logs.channelPanel.open': 'Open channel controls',
    'logs.channelPanel.openTokens': 'Open token controls',
    'logs.channelPanel.collapse': 'Collapse channel controls',
    'logs.channelPanel.collapseTokens': 'Collapse token controls',
    'logs.channelPanel.refresh': 'Refresh channels',
    'logs.channelPanel.refreshTokens': 'Refresh tokens',
    'logs.channelPanel.channelTab': 'Channels',
    'logs.channelPanel.tokenTab': 'Token controls',
    'logs.channelPanel.tabList': 'Quick control type',
    'logs.channelPanel.loading': 'Loading channels...',
    'logs.channelPanel.loadingTokens': 'Loading tokens...',
    'logs.channelPanel.loadFailed': 'Failed to load channels',
    'logs.channelPanel.tokenLoadFailed': 'Failed to load tokens',
    'logs.channelPanel.retry': 'Retry',
    'logs.channelPanel.empty': 'No channels',
    'logs.channelPanel.emptyTokens': 'No tokens',
    'logs.channelPanel.ungrouped': 'Ungrouped',
    'logs.channelPanel.tokenUngrouped': 'Ungrouped',
    'logs.channelPanel.enabledSummary': '{enabled}/{total} enabled',
    'logs.channelPanel.tokenEnabledSummary': '{enabled}/{total} enabled',
    'logs.channelPanel.priority': 'Priority {priority}',
    'logs.channelPanel.dailyCost': 'Today {cost}',
    'logs.channelPanel.recentCacheHitRate': 'Last 30m cache hit rate {rate}',
    'logs.channelPanel.recentCacheHitRateShort': 'Hit rate {rate}',
    'logs.channelPanel.recentCacheHitRateWindowShort': '30m {rate}',
    'logs.channelPanel.recent50CacheHitRate': 'Last 50 requests cache hit rate {rate}',
    'logs.channelPanel.recent50CacheHitRateShort': '50 req {rate}',
    'logs.channelPanel.tokenDailyCost': 'Today {cost}',
    'logs.channelPanel.tokenDailyCostWithLimit': 'Today {cost}/{limit}',
    'logs.channelPanel.tokenUsage': '{count} calls',
    'logs.channelPanel.tokenLastUsed': 'Last used {time}',
    'logs.channelPanel.tokenID': 'ID {id}',
    'logs.channelPanel.tokenDailyLimitDouble': 'Double',
    'logs.channelPanel.tokenDailyLimitTriple': 'Triple',
    'logs.channelPanel.tokenDailyLimitOverride': 'Temporary',
    'logs.channelPanel.tokenDailyLimitDoubleTitle': "Today's limit doubled: {base} -> {effective}",
    'logs.channelPanel.tokenDailyLimitTripleTitle': "Today's limit tripled: {base} -> {effective}",
    'logs.channelPanel.tokenDailyLimitOverrideTitle': "Today's temporary limit: {base} -> {effective}",
    'logs.channelPanel.tokenDailyLimitHint': 'Applies today only and resets after midnight',
    'logs.channelPanel.tokenBatteryUnlimited': 'Unlimited quota',
    'logs.channelPanel.tokenBatteryDailyRemaining': 'Daily remaining {remaining}/{limit} ({percent}%)',
    'logs.channelPanel.tokenBatteryMonthlyRemaining': 'Monthly remaining {remaining}/{limit} ({percent}%)',
    'logs.channelPanel.tokenBatteryTotalRemaining': 'Total remaining {remaining}/{limit} ({percent}%)',
    'logs.channelPanel.tokenUnlimited': 'Unlimited',
    'logs.channelPanel.cooldown': 'Cooling down',
    'logs.channelPanel.dragHandle': 'Reorder {name}',
    'logs.channelPanel.editChannel': 'Edit {name}',
    'logs.channelPanel.editToken': 'Edit {name}',
    'logs.channelPanel.toggleEnabled': '{name} enabled',
    'logs.channelPanel.toggleDisabled': '{name} disabled',
    'logs.channelPanel.toggleFailed': 'Failed to update {name}',
    'logs.channelPanel.tokenToggleEnabled': '{name} enabled',
    'logs.channelPanel.tokenToggleDisabled': '{name} disabled',
    'logs.channelPanel.tokenToggleFailed': 'Failed to update {name}',
    'common.enable': 'Enable',
    'common.disable': 'Disable',
    'logs.channelPanel.orderSaving': 'Saving channel order...',
    'logs.channelPanel.orderSaved': 'Channel order saved',
    'logs.channelPanel.orderFailed': 'Failed to save channel order',
    'channels.toggleEnable': 'Enable channel',
    'channels.toggleDisable': 'Disable channel'
  });

  const ICONS = Object.freeze({
    chevron: '<svg class="logs-channel-panel__group-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"></path></svg>',
    grip: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="9" cy="6" r="1"></circle><circle cx="9" cy="12" r="1"></circle><circle cx="9" cy="18" r="1"></circle><circle cx="15" cy="6" r="1"></circle><circle cx="15" cy="12" r="1"></circle><circle cx="15" cy="18" r="1"></circle></svg>',
    edit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 20h9"></path><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L8 18l-4 1 1-4Z"></path></svg>'
  });

  const state = {
    root: null,
    initialized: false,
    expanded: false,
    activeTab: CHANNEL_TAB,
    restoreFocusID: 'logsChannelPanelTrigger',
    loaded: false,
    tokenLoaded: false,
    loading: false,
    loadError: null,
    tokenLoadError: null,
    lastLoadedAt: 0,
    channels: [],
    groups: [],
    tokens: [],
    tokenGroups: [],
    recentChannelCacheStats: new Map(),
    recentTokenCacheStats: new Map(),
    recentChannel50CacheStats: new Map(),
    recentToken50CacheStats: new Map(),
    collapsedGroups: new Set(),
    pendingChannelIDs: new Set(),
    pendingTokenIDs: new Set(),
    orderSaving: false,
    nativeDragArmed: null,
    nativeDrag: null,
    pointerDrag: null,
    localeUnsubscribe: null,
    statusTimer: null,
    lifecycleID: 0,
    refreshSequence: 0,
    handlers: {}
  };

  function translate(key, params = {}) {
    let value = '';
    if (typeof window.t === 'function') {
      value = window.t(key, params);
    }
    if (!value || value === key) {
      value = FALLBACK_TEXT[key] || key;
      Object.entries(params).forEach(([name, replacement]) => {
        value = value.replace(new RegExp(`\\{${name}\\}`, 'g'), String(replacement));
      });
    }
    return value;
  }

  function escapeHTML(value) {
    return String(value === null || value === undefined ? '' : value).replace(/[&<>"']/g, (char) => ({
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;'
    }[char]));
  }

  function normalizeChannelID(value) {
    const id = Number(value);
    if (!Number.isFinite(id) || id <= 0) return 0;
    return Math.trunc(id);
  }

  function normalizeGroupKey(value) {
    const id = Number(value);
    if (!Number.isFinite(id) || id <= 0) return '0';
    return String(Math.trunc(id));
  }

  function normalizePriority(value) {
    const priority = Number(value);
    return Number.isFinite(priority) ? Math.trunc(priority) : 0;
  }

  function normalizeCacheTokenCount(value) {
    const count = Number(value);
    return Number.isFinite(count) && count > 0 ? count : 0;
  }

  function buildRecentCacheRange(now = Date.now()) {
    const numericNow = Number(now);
    const safeNow = Number.isFinite(numericNow) && numericNow > 0 ? numericNow : Date.now();
    const endMs = Math.floor(safeNow / RECENT_CACHE_BUCKET_MS) * RECENT_CACHE_BUCKET_MS;
    return {
      startMs: endMs - RECENT_CACHE_WINDOW_MS,
      endMs
    };
  }

  function buildCacheMetric(inputTokens, cacheReadTokens, cacheCreationTokens) {
    const input = normalizeCacheTokenCount(inputTokens);
    const cacheRead = normalizeCacheTokenCount(cacheReadTokens);
    const cacheCreation = normalizeCacheTokenCount(cacheCreationTokens);
    const denominator = input + cacheRead + cacheCreation;
    if (denominator <= 0) return null;
    return {
      inputTokens: input,
      cacheReadTokens: cacheRead,
      cacheCreationTokens: cacheCreation,
      denominator,
      rate: cacheRead / denominator
    };
  }

  function accumulateCacheMetric(rawStats, key, inputTokens, cacheReadTokens, cacheCreationTokens) {
    if (!rawStats.has(key)) {
      rawStats.set(key, {
        inputTokens: 0,
        cacheReadTokens: 0,
        cacheCreationTokens: 0
      });
    }
    const current = rawStats.get(key);
    current.inputTokens += normalizeCacheTokenCount(inputTokens);
    current.cacheReadTokens += normalizeCacheTokenCount(cacheReadTokens);
    current.cacheCreationTokens += normalizeCacheTokenCount(cacheCreationTokens);
  }

  function finalizeCacheMetrics(rawStats) {
    const result = new Map();
    for (const [key, totals] of rawStats.entries()) {
      const metric = buildCacheMetric(
        totals.inputTokens,
        totals.cacheReadTokens,
        totals.cacheCreationTokens
      );
      if (metric) result.set(key, metric);
    }
    return result;
  }

  function buildChannelCacheStats(data) {
    const entries = Array.isArray(data)
      ? data
      : (data && Array.isArray(data.stats) ? data.stats : []);
    const rawStats = new Map();
    for (const entry of entries) {
      const id = normalizeChannelID(entry && (entry.channel_id ?? entry.channelID));
      if (!id) continue;
      accumulateCacheMetric(
        rawStats,
        id,
        entry.total_input_tokens,
        entry.total_cache_read_input_tokens,
        entry.total_cache_creation_input_tokens
      );
    }
    return finalizeCacheMetrics(rawStats);
  }

  function buildTokenCacheStats(data) {
    const tokens = Array.isArray(data)
      ? data
      : (data && Array.isArray(data.tokens) ? data.tokens : []);
    const rawStats = new Map();
    for (const token of tokens) {
      const id = normalizeTokenID(token && token.id);
      if (!id) continue;
      accumulateCacheMetric(
        rawStats,
        id,
        token.prompt_tokens_total,
        token.cache_read_tokens_total,
        token.cache_creation_tokens_total
      );
    }
    return finalizeCacheMetrics(rawStats);
  }

  function formatRecentCacheHitRate(metric) {
    if (!metric || !Number.isFinite(Number(metric.rate))) return '';
    return translate('logs.channelPanel.recentCacheHitRate', {
      rate: `${(Math.max(0, Math.min(1, Number(metric.rate))) * 100).toFixed(1)}%`
    });
  }

  function formatRecentCacheHitRateShort(metric) {
    if (!metric || !Number.isFinite(Number(metric.rate))) return '';
    return translate('logs.channelPanel.recentCacheHitRateShort', {
      rate: `${(Math.max(0, Math.min(1, Number(metric.rate))) * 100).toFixed(1)}%`
    });
  }

  function formatRecentCacheHitRateWindowShort(metric) {
    if (!metric || !Number.isFinite(Number(metric.rate))) return '';
    return translate('logs.channelPanel.recentCacheHitRateWindowShort', {
      rate: `${(Math.max(0, Math.min(1, Number(metric.rate))) * 100).toFixed(1)}%`
    });
  }

  function formatRecent50CacheHitRate(metric) {
    if (!metric || !Number.isFinite(Number(metric.rate))) return '';
    return translate('logs.channelPanel.recent50CacheHitRate', {
      rate: `${(Math.max(0, Math.min(1, Number(metric.rate))) * 100).toFixed(1)}%`
    });
  }

  function formatRecent50CacheHitRateShort(metric) {
    if (!metric || !Number.isFinite(Number(metric.rate))) return '';
    return translate('logs.channelPanel.recent50CacheHitRateShort', {
      rate: `${(Math.max(0, Math.min(1, Number(metric.rate))) * 100).toFixed(1)}%`
    });
  }

  function buildRecentRequestCacheQuery(limit = RECENT_REQUEST_CACHE_COUNT) {
    const numericLimit = Number(limit);
    const safeLimit = Number.isFinite(numericLimit) && numericLimit > 0
      ? Math.min(1000, Math.trunc(numericLimit))
      : RECENT_REQUEST_CACHE_COUNT;
    const query = { limit: String(safeLimit) };
    if (typeof URLSearchParams === 'function') return new URLSearchParams(query).toString();
    return Object.entries(query)
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
      .join('&');
  }

  function buildRecentEntityCacheStats(entries, normalizeID, idFields = ['id', 'entity_id', 'entityID']) {
    const rawStats = new Map();
    if (!Array.isArray(entries)) return finalizeCacheMetrics(rawStats);
    for (const entry of entries) {
      if (!entry || typeof entry !== 'object') continue;
      const idValue = idFields.map((field) => entry[field]).find((value) => value !== undefined && value !== null);
      const id = normalizeID(idValue);
      if (!id) continue;
      accumulateCacheMetric(
        rawStats,
        id,
        entry.input_tokens ?? entry.inputTokens ?? entry.total_input_tokens ?? entry.prompt_tokens_total,
        entry.cache_read_tokens ?? entry.cacheReadTokens ?? entry.total_cache_read_input_tokens ?? entry.cache_read_tokens_total,
        entry.cache_creation_tokens ?? entry.cacheCreationTokens ?? entry.total_cache_creation_input_tokens ?? entry.cache_creation_tokens_total
      );
    }
    return finalizeCacheMetrics(rawStats);
  }

  function buildRecentLogCacheStats(entries) {
    const channelRawStats = new Map();
    const tokenRawStats = new Map();
    if (!Array.isArray(entries)) return { channels: new Map(), tokens: new Map() };
    for (const entry of entries) {
      if (!entry || typeof entry !== 'object') continue;
      const inputTokens = entry.input_tokens ?? entry.inputTokens;
      const cacheReadTokens = entry.cache_read_input_tokens ?? entry.cacheReadInputTokens;
      const cacheCreationTokens = entry.cache_creation_input_tokens ?? entry.cacheCreationInputTokens;
      const channelID = normalizeChannelID(entry.channel_id ?? entry.channelID);
      if (channelID) accumulateCacheMetric(channelRawStats, channelID, inputTokens, cacheReadTokens, cacheCreationTokens);
      const tokenID = normalizeTokenID(entry.auth_token_id ?? entry.authTokenID);
      if (tokenID) accumulateCacheMetric(tokenRawStats, tokenID, inputTokens, cacheReadTokens, cacheCreationTokens);
    }
    return {
      channels: finalizeCacheMetrics(channelRawStats),
      tokens: finalizeCacheMetrics(tokenRawStats)
    };
  }

  function buildRecentRequestCacheStats(data) {
    const payload = data && typeof data === 'object' && data.data && typeof data.data === 'object'
      ? data.data
      : data;
    if (!payload || typeof payload !== 'object') {
      return { channels: new Map(), tokens: new Map() };
    }
    if (Array.isArray(payload.channels) || Array.isArray(payload.tokens)) {
      return {
        channels: buildRecentEntityCacheStats(payload.channels, normalizeChannelID, ['id', 'entity_id', 'entityID', 'channel_id', 'channelID']),
        tokens: buildRecentEntityCacheStats(payload.tokens, normalizeTokenID, ['id', 'entity_id', 'entityID', 'auth_token_id', 'authTokenID'])
      };
    }
    const legacyEntries = Array.isArray(payload)
      ? payload
      : (Array.isArray(payload.logs) ? payload.logs : []);
    return buildRecentLogCacheStats(legacyEntries);
  }

  function buildChannelRecentRequestCacheStats(data) {
    return buildRecentRequestCacheStats(data).channels;
  }

  function buildTokenRecentRequestCacheStats(data) {
    return buildRecentRequestCacheStats(data).tokens;
  }

  function buildCacheRateBadge(shortText, fullText, modifier = '') {
    if (!shortText || !fullText) return '';
    const modifierClass = modifier ? ` logs-channel-panel__cache-rate--${modifier}` : '';
    return `<span class="logs-channel-panel__cache-rate${modifierClass}" title="${escapeHTML(fullText)}" aria-label="${escapeHTML(fullText)}">${escapeHTML(shortText)}</span>`;
  }

  function buildRecentCacheRatesHtml(recentMetric, recent50Metric) {
    const badges = [
      buildCacheRateBadge(
        formatRecentCacheHitRateWindowShort(recentMetric),
        formatRecentCacheHitRate(recentMetric)
      ),
      buildCacheRateBadge(
        formatRecent50CacheHitRateShort(recent50Metric),
        formatRecent50CacheHitRate(recent50Metric),
        'recent50'
      )
    ].filter(Boolean);
    return badges.length
      ? `<span class="logs-channel-panel__cache-rates">${badges.join('')}</span>`
      : '';
  }

  function formatDailyCost(value) {
    const numericValue = Number(value);
    const cost = Number.isFinite(numericValue) && numericValue >= 0 ? numericValue : 0;
    if (typeof window.formatCost === 'function') {
      const formatted = window.formatCost(cost);
      if (formatted) return String(formatted);
    }
    return cost === 0 ? '$0' : `$${cost.toFixed(3)}`;
  }

  // Quota values stay compact while the current usage keeps the page's
  // three-decimal cost precision.
  function formatQuotaCost(value) {
    const numericValue = Number(value);
    if (!Number.isFinite(numericValue) || numericValue <= 0) return '$0';
    return `$${numericValue.toFixed(3).replace(/\.?0+$/, '')}`;
  }

  function isTokenFlagEnabled(value) {
    return value === true || value === 1 || value === '1' || value === 'true';
  }

  function getTokenEffectiveDailyCostLimit(token) {
    if (token && token.effective_daily_cost_limit_usd !== undefined) {
      return Math.max(0, Number(token.effective_daily_cost_limit_usd) || 0);
    }
    const overrideLimit = Number(token && token.daily_limit_override_usd) || 0;
    if (overrideLimit > 0) return overrideLimit;
    const baseLimit = Number(token && token.daily_cost_limit_usd) || 0;
    if (isTokenFlagEnabled(token && token.daily_limit_triple_enabled)) return Math.max(0, baseLimit * 3);
    if (isTokenFlagEnabled(token && token.daily_limit_double_enabled)) return Math.max(0, baseLimit * 2);
    return Math.max(0, baseLimit);
  }

  function getTokenEffectiveMonthlyCostLimit(token) {
    if (token && token.effective_monthly_cost_limit_usd !== undefined) {
      return Math.max(0, Number(token.effective_monthly_cost_limit_usd) || 0);
    }
    return Math.max(0, Number(token && token.monthly_cost_limit_usd) || 0);
  }

  function getTokenEffectiveCostLimit(token) {
    if (token && token.effective_cost_limit_usd !== undefined) {
      return Math.max(0, Number(token.effective_cost_limit_usd) || 0);
    }
    return Math.max(0, Number(token && token.cost_limit_usd) || 0);
  }

  function getTokenBatteryState(token) {
    const windows = [
      {
        name: 'daily',
        used: Math.max(0, Number(token && token.daily_cost_used_usd) || 0),
        limit: getTokenEffectiveDailyCostLimit(token),
        titleKey: 'logs.channelPanel.tokenBatteryDailyRemaining'
      },
      {
        name: 'monthly',
        used: Math.max(0, Number(token && token.monthly_cost_used_usd) || 0),
        limit: getTokenEffectiveMonthlyCostLimit(token),
        titleKey: 'logs.channelPanel.tokenBatteryMonthlyRemaining'
      },
      {
        name: 'total',
        used: Math.max(0, Number(token && (token.cost_used_usd ?? token.total_cost_usd)) || 0),
        limit: getTokenEffectiveCostLimit(token),
        titleKey: 'logs.channelPanel.tokenBatteryTotalRemaining'
      }
    ].filter((windowState) => windowState.limit > 0).map((windowState) => {
      const remaining = Math.max(0, windowState.limit - windowState.used);
      const ratio = Math.max(0, Math.min(1, remaining / windowState.limit));
      return { ...windowState, remaining, ratio };
    });

    if (windows.length === 0) {
      return {
        source: 'unlimited',
        ratio: 1,
        percent: 100,
        remainingUsd: Infinity,
        limitUsd: 0,
        usedUsd: 0,
        levelClass: 'logs-channel-panel__token-battery--full',
        fillClass: 'logs-channel-panel__token-battery-fill--full',
        title: translate('logs.channelPanel.tokenBatteryUnlimited')
      };
    }

    const selected = windows.reduce((tightest, windowState) => (
      windowState.ratio < tightest.ratio ? windowState : tightest
    ));
    const ratio = selected.ratio;
    const percent = Math.round(ratio * 100);
    const title = windows.map((windowState) => translate(windowState.titleKey, {
      remaining: formatQuotaCost(windowState.remaining),
      limit: formatQuotaCost(windowState.limit),
      percent: Math.round(windowState.ratio * 100)
    })).join(' / ');

    let tone = 'critical';
    if (ratio >= 0.8) tone = 'full';
    else if (ratio >= 0.6) tone = 'high';
    else if (ratio >= 0.4) tone = 'medium';
    else if (ratio >= 0.2) tone = 'low';

    return {
      source: windows.length === 1 ? selected.name : `multiple-${selected.name}`,
      ratio,
      percent,
      remainingUsd: selected.remaining,
      limitUsd: selected.limit,
      usedUsd: selected.used,
      levelClass: `logs-channel-panel__token-battery--${tone}`,
      fillClass: `logs-channel-panel__token-battery-fill--${tone}`,
      title
    };
  }

  function buildTokenBatteryHtml(token) {
    const battery = getTokenBatteryState(token);
    return `
      <span class="logs-channel-panel__token-battery-wrap" title="${escapeHTML(battery.title)}" aria-label="${escapeHTML(battery.title)}">
        <span class="logs-channel-panel__token-battery ${battery.levelClass}" aria-hidden="true">
          <span class="logs-channel-panel__token-battery-body">
            <span class="logs-channel-panel__token-battery-fill ${battery.fillClass}" style="width: ${battery.percent}%;"></span>
          </span>
          <span class="logs-channel-panel__token-battery-cap"></span>
        </span>
        <span class="logs-channel-panel__token-battery-percent ${battery.levelClass}">${battery.percent}%</span>
      </span>`;
  }

  function buildTokenLimitBadgesHtml(token) {
    if (!token) return '';
    const rawLimit = Math.max(0, Number(token.daily_cost_limit_usd) || 0);
    const effectiveLimit = getTokenEffectiveDailyCostLimit(token);
    const badges = [];
    const base = rawLimit > 0 ? formatQuotaCost(rawLimit) : translate('logs.channelPanel.tokenUnlimited');

    if (isTokenFlagEnabled(token.daily_limit_triple_enabled)) {
      const title = rawLimit > 0
        ? translate('logs.channelPanel.tokenDailyLimitTripleTitle', {
          base,
          effective: formatQuotaCost(effectiveLimit)
        })
        : translate('logs.channelPanel.tokenDailyLimitHint');
      badges.push(`<span class="logs-channel-panel__token-badge logs-channel-panel__token-badge--daily-triple" title="${escapeHTML(title)}">&times;3 ${escapeHTML(translate('logs.channelPanel.tokenDailyLimitTriple'))}</span>`);
    } else if (isTokenFlagEnabled(token.daily_limit_double_enabled)) {
      const title = rawLimit > 0
        ? translate('logs.channelPanel.tokenDailyLimitDoubleTitle', {
          base,
          effective: formatQuotaCost(effectiveLimit)
        })
        : translate('logs.channelPanel.tokenDailyLimitHint');
      badges.push(`<span class="logs-channel-panel__token-badge logs-channel-panel__token-badge--daily-double" title="${escapeHTML(title)}">&times;2 ${escapeHTML(translate('logs.channelPanel.tokenDailyLimitDouble'))}</span>`);
    }

    const overrideLimit = Math.max(0, Number(token.daily_limit_override_usd) || 0);
    if (overrideLimit > 0) {
      const title = translate('logs.channelPanel.tokenDailyLimitOverrideTitle', {
        base,
        effective: formatQuotaCost(overrideLimit)
      });
      badges.push(`<span class="logs-channel-panel__token-badge logs-channel-panel__token-badge--daily-override" title="${escapeHTML(title)}">${escapeHTML(formatQuotaCost(overrideLimit))} ${escapeHTML(translate('logs.channelPanel.tokenDailyLimitOverride'))}</span>`);
    }

    return badges.join('');
  }

  function formatTokenDailyCost(token) {
    const cost = formatDailyCost(token && token.daily_cost_used_usd);
    const limit = getTokenEffectiveDailyCostLimit(token);
    return limit > 0
      ? translate('logs.channelPanel.tokenDailyCostWithLimit', { cost, limit: formatQuotaCost(limit) })
      : translate('logs.channelPanel.tokenDailyCost', { cost });
  }

  function normalizeGroupColor(value) {
    const color = String(value || '').trim().toLowerCase();
    return GROUP_COLORS.has(color) ? color : '#64748b';
  }

  function compareChannelsByPriority(a, b) {
    const priorityDifference = normalizePriority(b && b.priority) - normalizePriority(a && a.priority);
    if (priorityDifference !== 0) return priorityDifference;
    const nameDifference = String(a && a.name || '').localeCompare(String(b && b.name || ''));
    if (nameDifference !== 0) return nameDifference;
    return normalizeChannelID(a && a.id) - normalizeChannelID(b && b.id);
  }

  function buildChannelGroups(channels = [], groups = [], ungroupedLabel = 'Ungrouped') {
    const definitions = new Map();
    for (const group of Array.isArray(groups) ? groups : []) {
      const key = normalizeGroupKey(group && group.id);
      if (key === '0') continue;
      definitions.set(key, group);
    }

    const buckets = new Map();
    for (const channel of Array.isArray(channels) ? channels : []) {
      if (!normalizeChannelID(channel && channel.id)) continue;
      const key = normalizeGroupKey(channel && channel.group_id);
      const definition = definitions.get(key) || null;
      if (!buckets.has(key)) {
        const fallbackName = key === '0'
          ? ungroupedLabel
          : (channel.group_name || `#${key}`);
        buckets.set(key, {
          key,
          id: Number(key),
          name: String(definition && definition.name || fallbackName),
          description: String(definition && definition.description || ''),
          color: normalizeGroupColor(definition && definition.color || channel.group_color),
          channels: []
        });
      }
      buckets.get(key).channels.push(channel);
    }

    return Array.from(buckets.values())
      .map((group) => {
        group.channels.sort(compareChannelsByPriority);
        group.totalCount = group.channels.length;
        group.enabledCount = group.channels.filter((channel) => channel.enabled === true).length;
        return group;
      })
      .sort((a, b) => {
        if (a.key === '0') return 1;
        if (b.key === '0') return -1;
        const nameDifference = a.name.localeCompare(b.name);
        return nameDifference !== 0 ? nameDifference : a.id - b.id;
      });
  }

  function buildPriorityUpdatesAfterGroupReorder(channels = [], groupID, orderedIDs = []) {
    const targetGroupKey = normalizeGroupKey(groupID);
    const globalOrder = (Array.isArray(channels) ? channels : [])
      .filter((channel) => normalizeChannelID(channel && channel.id) > 0)
      .slice()
      .sort(compareChannelsByPriority);
    const targetChannels = globalOrder.filter((channel) => normalizeGroupKey(channel.group_id) === targetGroupKey);
    const normalizedOrder = [];
    const seen = new Set();

    for (const value of Array.isArray(orderedIDs) ? orderedIDs : []) {
      const id = normalizeChannelID(value);
      if (!id || seen.has(id)) continue;
      seen.add(id);
      normalizedOrder.push(id);
    }

    if (targetChannels.length < 2 || normalizedOrder.length !== targetChannels.length) return [];
    const targetByID = new Map(targetChannels.map((channel) => [normalizeChannelID(channel.id), channel]));
    if (normalizedOrder.some((id) => !targetByID.has(id))) return [];

    let targetIndex = 0;
    const reorderedGlobal = globalOrder.map((channel) => {
      if (normalizeGroupKey(channel.group_id) !== targetGroupKey) return channel;
      const nextChannel = targetByID.get(normalizedOrder[targetIndex]);
      targetIndex += 1;
      return nextChannel;
    });

    return reorderedGlobal.map((channel, index) => ({
      id: normalizeChannelID(channel.id),
      priority: (reorderedGlobal.length - index) * 10
    }));
  }

  function readCollapsedGroups() {
    try {
      const parsed = JSON.parse(window.localStorage.getItem(COLLAPSED_GROUPS_KEY) || '[]');
      if (!Array.isArray(parsed)) return new Set();
      return new Set(parsed.map((value) => {
        const raw = String(value || '');
        return raw.startsWith('token:')
          ? `token:${normalizeGroupKey(raw.slice('token:'.length))}`
          : normalizeGroupKey(raw);
      }));
    } catch (_) {
      return new Set();
    }
  }

  function persistCollapsedGroups() {
    try {
      window.localStorage.setItem(COLLAPSED_GROUPS_KEY, JSON.stringify(Array.from(state.collapsedGroups)));
    } catch (_) {
      // Storage is optional for this UI preference.
    }
  }

  function normalizeTab(value) {
    return value === TOKEN_TAB ? TOKEN_TAB : CHANNEL_TAB;
  }

  function readActiveTab() {
    try {
      return normalizeTab(window.localStorage.getItem(ACTIVE_TAB_KEY));
    } catch (_) {
      return CHANNEL_TAB;
    }
  }

  function persistActiveTab() {
    try {
      window.localStorage.setItem(ACTIVE_TAB_KEY, normalizeTab(state.activeTab));
    } catch (_) {
      // Storage is optional for this UI preference.
    }
  }

  function getElement(id) {
    return document.getElementById(id);
  }

  function getChannel(channelID) {
    const id = normalizeChannelID(channelID);
    return state.channels.find((channel) => normalizeChannelID(channel && channel.id) === id) || null;
  }

  function normalizeTokenID(value) {
    const id = Number(value);
    if (!Number.isFinite(id) || id <= 0) return 0;
    return Math.trunc(id);
  }

  function isTokenActive(token) {
    return Boolean(token && (token.is_active === true || token.is_active === 1 || token.is_active === 'true' || token.is_active === '1'));
  }

  function getToken(tokenID) {
    const id = normalizeTokenID(tokenID);
    return state.tokens.find((token) => normalizeTokenID(token && token.id) === id) || null;
  }

  function tokenEnabledSummary(tokens = state.tokens) {
    const list = Array.isArray(tokens) ? tokens : [];
    return {
      enabled: list.filter((token) => isTokenActive(token)).length,
      total: list.length
    };
  }

  function normalizeTimestamp(value) {
    if (value === null || value === undefined || value === '') return 0;
    const numeric = Number(value);
    if (Number.isFinite(numeric) && numeric > 0) return numeric;
    const parsed = Date.parse(String(value));
    return Number.isFinite(parsed) ? parsed : 0;
  }

  function compareTokens(a, b) {
    const activeDifference = Number(isTokenActive(b)) - Number(isTokenActive(a));
    if (activeDifference !== 0) return activeDifference;
    const lastUsedDifference = normalizeTimestamp(b && b.last_used_at) - normalizeTimestamp(a && a.last_used_at);
    if (lastUsedDifference !== 0) return lastUsedDifference;
    const nameDifference = String(a && a.description || '').localeCompare(String(b && b.description || ''));
    if (nameDifference !== 0) return nameDifference;
    return normalizeTokenID(a && a.id) - normalizeTokenID(b && b.id);
  }

  function buildTokenGroups(tokens = [], groups = [], ungroupedLabel = 'Ungrouped') {
    const definitions = new Map();
    for (const group of Array.isArray(groups) ? groups : []) {
      const key = normalizeGroupKey(group && group.id);
      if (key === '0') continue;
      definitions.set(key, group);
    }

    const buckets = new Map();
    for (const token of Array.isArray(tokens) ? tokens : []) {
      if (!normalizeTokenID(token && token.id)) continue;
      const key = normalizeGroupKey(token && token.group_id);
      const definition = definitions.get(key) || null;
      if (!buckets.has(key)) {
        const fallbackName = key === '0'
          ? ungroupedLabel
          : (token.group_name || `#${key}`);
        buckets.set(key, {
          key,
          id: Number(key),
          name: String(definition && definition.name || fallbackName),
          description: String(definition && definition.description || ''),
          color: normalizeGroupColor(definition && definition.color || token.group_color),
          tokens: []
        });
      }
      buckets.get(key).tokens.push(token);
    }

    return Array.from(buckets.values())
      .map((group) => {
        group.tokens.sort(compareTokens);
        group.totalCount = group.tokens.length;
        group.enabledCount = group.tokens.filter((token) => isTokenActive(token)).length;
        return group;
      })
      .sort((a, b) => {
        if (a.key === '0') return 1;
        if (b.key === '0') return -1;
        const nameDifference = a.name.localeCompare(b.name);
        return nameDifference !== 0 ? nameDifference : a.id - b.id;
      });
  }

  function isChannelCoolingDown(channel) {
    if (Number(channel && channel.cooldown_remaining_ms) > 0) return true;
    const until = Date.parse(channel && channel.cooldown_until || '');
    return Number.isFinite(until) && until > Date.now();
  }

  function enabledSummary(channels = state.channels) {
    const total = channels.length;
    const enabled = channels.filter((channel) => channel.enabled === true).length;
    return { enabled, total };
  }

  function updateChrome() {
    if (!state.root) return;
    const trigger = getElement('logsChannelPanelTrigger');
    const tokenTrigger = getElement('logsTokenPanelTrigger');
    const surface = getElement('logsChannelPanelSurface');
    const badge = getElement('logsChannelPanelBadge');
    const tokenBadge = getElement('logsTokenPanelBadge');
    const summary = getElement('logsChannelPanelSummary');
    const heading = getElement('logsChannelPanelHeading');
    const refresh = getElement('logsChannelPanelRefresh');
    const channelTab = getElement('logsChannelPanelChannelTab');
    const tokenTab = getElement('logsChannelPanelTokenTab');
    const body = getElement('logsChannelPanelBody');
    const openLabel = translate('logs.channelPanel.open');
    const openTokenLabel = translate('logs.channelPanel.openTokens');
    const collapseLabel = translate(state.activeTab === TOKEN_TAB
      ? 'logs.channelPanel.collapseTokens'
      : 'logs.channelPanel.collapse');
    const refreshLabel = state.activeTab === TOKEN_TAB
      ? translate('logs.channelPanel.refreshTokens')
      : translate('logs.channelPanel.refresh');
    const activeLoaded = state.activeTab === TOKEN_TAB ? state.tokenLoaded : state.loaded;

    state.root.dataset.state = state.expanded ? 'expanded' : 'collapsed';
    state.root.dataset.activeTab = state.activeTab;
    state.root.setAttribute('aria-label', state.activeTab === TOKEN_TAB
      ? translate('logs.channelPanel.tokenTab')
      : translate('logs.channelPanel.title'));
    if (trigger) {
      trigger.hidden = state.expanded;
      trigger.setAttribute('aria-expanded', String(state.expanded));
      trigger.title = openLabel;
      trigger.setAttribute('aria-label', openLabel);
    }
    if (tokenTrigger) {
      tokenTrigger.hidden = state.expanded;
      tokenTrigger.setAttribute('aria-expanded', String(state.expanded && state.activeTab === TOKEN_TAB));
      tokenTrigger.title = openTokenLabel;
      tokenTrigger.setAttribute('aria-label', openTokenLabel);
    }
    if (surface) {
      surface.hidden = !state.expanded;
      surface.classList.toggle('is-refreshing', state.loading && activeLoaded);
    }
    if (refresh) {
      refresh.disabled = state.loading
        || state.orderSaving
        || state.pendingChannelIDs.size > 0
        || state.pendingTokenIDs.size > 0;
      refresh.title = refreshLabel;
      refresh.setAttribute('aria-label', refreshLabel);
    }
    const collapseButton = state.root.querySelector('[data-channel-panel-action="collapse"]');
    if (collapseButton) {
      collapseButton.title = collapseLabel;
      collapseButton.setAttribute('aria-label', collapseLabel);
    }

    if (heading) {
      heading.textContent = state.activeTab === TOKEN_TAB
        ? translate('logs.channelPanel.tokenTab')
        : translate('logs.channelPanel.title');
    }
    if (channelTab) {
      const selected = state.activeTab === CHANNEL_TAB;
      channelTab.setAttribute('aria-selected', String(selected));
      channelTab.tabIndex = selected ? 0 : -1;
      channelTab.classList.toggle('is-active', selected);
    }
    if (tokenTab) {
      const selected = state.activeTab === TOKEN_TAB;
      tokenTab.setAttribute('aria-selected', String(selected));
      tokenTab.tabIndex = selected ? 0 : -1;
      tokenTab.classList.toggle('is-active', selected);
    }
    if (body) {
      body.dataset.activeTab = state.activeTab;
      body.setAttribute('aria-labelledby', state.activeTab === TOKEN_TAB
        ? 'logsChannelPanelTokenTab'
        : 'logsChannelPanelChannelTab');
    }

    const counts = state.activeTab === TOKEN_TAB ? tokenEnabledSummary() : enabledSummary();
    if (summary) {
      summary.textContent = activeLoaded
        ? translate(state.activeTab === TOKEN_TAB ? 'logs.channelPanel.tokenEnabledSummary' : 'logs.channelPanel.enabledSummary', counts)
        : '';
    }
    if (badge) {
      badge.hidden = !state.loaded;
      const channelCounts = enabledSummary();
      badge.textContent = channelCounts.enabled > 99 ? '99+' : String(channelCounts.enabled);
      badge.title = state.loaded ? translate('logs.channelPanel.enabledSummary', channelCounts) : '';
    }
    if (tokenBadge) {
      tokenBadge.hidden = !state.tokenLoaded;
      const tokenCounts = tokenEnabledSummary();
      tokenBadge.textContent = tokenCounts.enabled > 99 ? '99+' : String(tokenCounts.enabled);
      tokenBadge.title = state.tokenLoaded ? translate('logs.channelPanel.tokenEnabledSummary', tokenCounts) : '';
    }
  }

  function setStatus(message = '', tone = '', timeout = 2600) {
    const status = getElement('logsChannelPanelStatus');
    if (!status) return;
    if (state.statusTimer) {
      clearTimeout(state.statusTimer);
      state.statusTimer = null;
    }
    status.textContent = message;
    if (tone) status.dataset.tone = tone;
    else delete status.dataset.tone;
    if (message && timeout > 0) {
      state.statusTimer = setTimeout(() => {
        status.textContent = '';
        delete status.dataset.tone;
        state.statusTimer = null;
      }, timeout);
    }
  }

  function renderState(messageKey, options = {}) {
    const body = getElement('logsChannelPanelBody');
    if (!body) return;
    if (options.error) {
      body.innerHTML = `
        <div class="logs-channel-panel__state logs-channel-panel__state--error">
          <span>${escapeHTML(translate(messageKey))}</span>
          <button type="button" class="logs-channel-panel__retry" data-channel-panel-action="retry">${escapeHTML(translate('logs.channelPanel.retry'))}</button>
        </div>`;
      return;
    }
    body.innerHTML = `
      <div class="logs-channel-panel__state">
        ${options.loading ? '<span class="logs-channel-panel__spinner" aria-hidden="true"></span>' : ''}
        <span>${escapeHTML(translate(messageKey))}</span>
      </div>`;
  }

  function renderChannelRow(channel, groupKey, cacheMetric = undefined, cache50Metric = undefined) {
    const id = normalizeChannelID(channel.id);
    const enabled = channel.enabled === true;
    const pending = state.pendingChannelIDs.has(id);
    const coolingDown = isChannelCoolingDown(channel);
    const name = String(channel.name || `#${id}`);
    const type = String(channel.channel_type || '--');
    const priority = normalizePriority(channel.priority);
    const dailyCost = translate('logs.channelPanel.dailyCost', { cost: formatDailyCost(channel.daily_cost_used) });
    const dragLabel = translate('logs.channelPanel.dragHandle', { name });
    const editLabel = translate('logs.channelPanel.editChannel', { name });
    const switchLabel = translate(enabled ? 'channels.toggleDisable' : 'channels.toggleEnable');
    const recentCacheMetric = cacheMetric === undefined
      ? state.recentChannelCacheStats.get(id)
      : cacheMetric;
    const recentCache50Metric = cache50Metric === undefined
      ? state.recentChannel50CacheStats.get(id)
      : cache50Metric;
    const recentCacheRatesHTML = buildRecentCacheRatesHtml(recentCacheMetric, recentCache50Metric);
    const cooldownHTML = coolingDown
      ? `<span class="logs-channel-panel__meta-item logs-channel-panel__meta-item--cooldown"><span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span><span class="logs-channel-panel__cooldown">${escapeHTML(translate('logs.channelPanel.cooldown'))}</span></span>`
      : '';

    return `
      <div class="logs-channel-panel__row${enabled ? '' : ' is-disabled'}" role="listitem" draggable="${state.orderSaving ? 'false' : 'true'}" data-channel-id="${id}" data-group-id="${escapeHTML(groupKey)}">
        <span class="logs-channel-panel__drag" role="button" tabindex="${state.orderSaving ? '-1' : '0'}" aria-disabled="${state.orderSaving ? 'true' : 'false'}" data-channel-panel-action="drag" title="${escapeHTML(dragLabel)}" aria-label="${escapeHTML(dragLabel)}">
          ${ICONS.grip}
        </span>
        <div class="logs-channel-panel__info">
          <div class="logs-channel-panel__name-line">
            <div class="logs-channel-panel__name" title="${escapeHTML(name)}">${escapeHTML(name)}</div>
            ${recentCacheRatesHTML}
          </div>
          <div class="logs-channel-panel__meta">
            <span class="logs-channel-panel__type">${escapeHTML(type)}</span>
            <span class="logs-channel-panel__meta-item logs-channel-panel__meta-item--priority">
              <span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span>
              <span class="logs-channel-panel__priority">${escapeHTML(translate('logs.channelPanel.priority', { priority }))}</span>
            </span>
            <span class="logs-channel-panel__meta-item logs-channel-panel__meta-item--daily-cost">
              <span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span>
              <span class="logs-channel-panel__daily-cost" title="${escapeHTML(dailyCost)}">${escapeHTML(dailyCost)}</span>
            </span>
            ${cooldownHTML}
          </div>
        </div>
        <div class="logs-channel-panel__row-actions">
          <button type="button" class="logs-channel-panel__edit" data-channel-panel-action="edit-channel" data-channel-id="${id}" title="${escapeHTML(editLabel)}" aria-label="${escapeHTML(editLabel)}">${ICONS.edit}</button>
          <button type="button" class="logs-channel-panel__switch" data-channel-panel-action="toggle-channel" data-channel-id="${id}" role="switch" aria-checked="${enabled}" aria-disabled="${pending}" title="${escapeHTML(switchLabel)}" aria-label="${escapeHTML(switchLabel)}"></button>
        </div>
      </div>`;
  }

  function formatTokenLastUsed(value) {
    const timestamp = normalizeTimestamp(value);
    if (!timestamp) return '';
    try {
      return new Date(timestamp).toLocaleString(window.i18n?.getLocale?.() || undefined, {
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      });
    } catch (_) {
      return new Date(timestamp).toISOString();
    }
  }

  function maskToken(value) {
    const token = String(value || '').trim();
    if (!token) return '';
    if (token.length <= 8) return '****';
    return `${token.slice(0, 4)}****${token.slice(-4)}`;
  }

  function renderTokenRow(token, groupKey, cacheMetric = undefined, cache50Metric = undefined) {
    const id = normalizeTokenID(token && token.id);
    const active = isTokenActive(token);
    const pending = state.pendingTokenIDs.has(id);
    const name = String(token && token.description || `#${id}`);
    const successCount = Number(token && token.success_count || 0);
    const failureCount = Number(token && token.failure_count || 0);
    const usage = (Number.isFinite(successCount) ? successCount : 0)
      + (Number.isFinite(failureCount) ? failureCount : 0);
    const dailyCost = formatTokenDailyCost(token);
    const usageText = translate('logs.channelPanel.tokenUsage', { count: usage });
    const lastUsed = formatTokenLastUsed(token && token.last_used_at);
    const lastUsedText = lastUsed
      ? translate('logs.channelPanel.tokenLastUsed', { time: lastUsed })
      : '';
    const tokenIDText = translate('logs.channelPanel.tokenID', { id });
    const editLabel = translate('logs.channelPanel.editToken', { name });
    const switchLabel = `${translate(active ? 'common.disable' : 'common.enable')} ${name}`;
    const masked = maskToken(token && token.plain_token);
    const recentCacheMetric = cacheMetric === undefined
      ? state.recentTokenCacheStats.get(id)
      : cacheMetric;
    const recentCache50Metric = cache50Metric === undefined
      ? state.recentToken50CacheStats.get(id)
      : cache50Metric;
    const recentCacheRatesHTML = buildRecentCacheRatesHtml(recentCacheMetric, recentCache50Metric);
    const tokenBatteryHTML = buildTokenBatteryHtml(token);
    const tokenLimitBadgesHTML = buildTokenLimitBadgesHtml(token);
    const tokenMeta = [tokenIDText, usageText, dailyCost, lastUsedText].filter(Boolean);

    return `
      <div class="logs-channel-panel__row logs-channel-panel__token-row${active ? '' : ' is-disabled'}" role="listitem" data-token-id="${id}" data-group-id="${escapeHTML(groupKey)}">
        <div class="logs-channel-panel__info">
          <div class="logs-channel-panel__name-line">
            <div class="logs-channel-panel__name" title="${escapeHTML(name)}">${escapeHTML(name)}</div>
            ${recentCacheRatesHTML}
            ${tokenBatteryHTML}
          </div>
          ${tokenLimitBadgesHTML ? `<div class="logs-channel-panel__token-badges">${tokenLimitBadgesHTML}</div>` : ''}
          <div class="logs-channel-panel__meta" title="${escapeHTML(tokenMeta.join(' / '))}">
            ${tokenMeta.map((item, index) => `${index ? '<span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span>' : ''}<span class="logs-channel-panel__meta-item">${escapeHTML(item)}</span>`).join('')}
          </div>
          ${masked ? `<div class="logs-channel-panel__token-mask" title="${escapeHTML(masked)}">${escapeHTML(masked)}</div>` : ''}
        </div>
        <div class="logs-channel-panel__row-actions">
          <button type="button" class="logs-channel-panel__edit" data-channel-panel-action="edit-token" data-token-id="${id}" title="${escapeHTML(editLabel)}" aria-label="${escapeHTML(editLabel)}">${ICONS.edit}</button>
          <button type="button" class="logs-channel-panel__switch" data-channel-panel-action="toggle-token" data-token-id="${id}" role="switch" aria-checked="${active}" aria-disabled="${pending}" title="${escapeHTML(switchLabel)}" aria-label="${escapeHTML(switchLabel)}"></button>
        </div>
      </div>`;
  }

  function renderGroup(group) {
    const collapsed = state.collapsedGroups.has(group.key);
    const description = group.description ? ` title="${escapeHTML(group.description)}"` : '';
    const rows = group.channels.map((channel) => renderChannelRow(channel, group.key)).join('');
    return `
      <section class="logs-channel-panel__group${collapsed ? ' is-collapsed' : ''}" data-group-id="${escapeHTML(group.key)}">
        <button type="button" class="logs-channel-panel__group-toggle" data-channel-panel-action="toggle-group" data-group-id="${escapeHTML(group.key)}" aria-expanded="${!collapsed}" aria-controls="logsChannelPanelGroup-${escapeHTML(group.key)}"${description}>
          ${ICONS.chevron}
          <span class="logs-channel-panel__group-dot" style="background-color:${escapeHTML(group.color)}" aria-hidden="true"></span>
          <span class="logs-channel-panel__group-name">${escapeHTML(group.name)}</span>
          <span class="logs-channel-panel__group-count">${escapeHTML(translate('logs.channelPanel.enabledSummary', { enabled: group.enabledCount, total: group.totalCount }))}</span>
        </button>
        <div id="logsChannelPanelGroup-${escapeHTML(group.key)}" class="logs-channel-panel__group-list" role="list" data-group-id="${escapeHTML(group.key)}"${collapsed ? ' hidden' : ''}>${rows}</div>
      </section>`;
  }

  function renderTokenGroup(group) {
    const collapsed = state.collapsedGroups.has(`token:${group.key}`);
    const description = group.description ? ` title="${escapeHTML(group.description)}"` : '';
    const rows = group.tokens.map((token) => renderTokenRow(token, group.key)).join('');
    return `
      <section class="logs-channel-panel__group logs-channel-panel__token-group${collapsed ? ' is-collapsed' : ''}" data-group-id="${escapeHTML(group.key)}" data-token-group="true">
        <button type="button" class="logs-channel-panel__group-toggle" data-channel-panel-action="toggle-token-group" data-group-id="${escapeHTML(group.key)}" aria-expanded="${!collapsed}" aria-controls="logsChannelPanelTokenGroup-${escapeHTML(group.key)}"${description}>
          ${ICONS.chevron}
          <span class="logs-channel-panel__group-dot" style="background-color:${escapeHTML(group.color)}" aria-hidden="true"></span>
          <span class="logs-channel-panel__group-name">${escapeHTML(group.name)}</span>
          <span class="logs-channel-panel__group-count">${escapeHTML(translate('logs.channelPanel.tokenEnabledSummary', { enabled: group.enabledCount, total: group.totalCount }))}</span>
        </button>
        <div id="logsChannelPanelTokenGroup-${escapeHTML(group.key)}" class="logs-channel-panel__group-list logs-channel-panel__token-group-list" role="list" data-group-id="${escapeHTML(group.key)}" data-token-group="true"${collapsed ? ' hidden' : ''}>${rows}</div>
      </section>`;
  }

  function renderPanel(options = {}) {
    updateChrome();
    if (!state.expanded) return;
    const tokenTabActive = state.activeTab === TOKEN_TAB;
    const activeLoaded = tokenTabActive ? state.tokenLoaded : state.loaded;
    const activeError = tokenTabActive ? state.tokenLoadError : state.loadError;
    if (state.loading && !activeLoaded) {
      renderState(tokenTabActive ? 'logs.channelPanel.loadingTokens' : 'logs.channelPanel.loading', { loading: true });
      return;
    }
    if (activeError && !activeLoaded) {
      renderState(tokenTabActive ? 'logs.channelPanel.tokenLoadFailed' : 'logs.channelPanel.loadFailed', { error: true });
      return;
    }

    const body = getElement('logsChannelPanelBody');
    if (!body) return;
    const scrollTop = body.scrollTop;
    if (tokenTabActive) {
      const groups = buildTokenGroups(state.tokens, state.tokenGroups, translate('logs.channelPanel.tokenUngrouped'));
      if (groups.length === 0) {
        renderState('logs.channelPanel.emptyTokens');
        return;
      }
      body.innerHTML = groups.map(renderTokenGroup).join('');
    } else {
      const groups = buildChannelGroups(state.channels, state.groups, translate('logs.channelPanel.ungrouped'));
      if (groups.length === 0) {
        renderState('logs.channelPanel.empty');
        return;
      }
      body.innerHTML = groups.map(renderGroup).join('');
    }
    body.scrollTop = scrollTop;

    if (options.focusGroupKey !== undefined) {
      const groupAction = tokenTabActive ? 'toggle-token-group' : 'toggle-group';
      body.querySelector(`[data-channel-panel-action="${groupAction}"][data-group-id="${String(options.focusGroupKey).replace(/"/g, '')}"]`)?.focus();
    } else if (!tokenTabActive && options.focusChannelID) {
      const action = options.focusAction || 'toggle-channel';
      body.querySelector(`[data-channel-panel-action="${action}"][data-channel-id="${normalizeChannelID(options.focusChannelID)}"]`)?.focus();
    } else if (tokenTabActive && options.focusTokenID) {
      const action = options.focusAction || 'toggle-token';
      body.querySelector(`[data-channel-panel-action="${action}"][data-token-id="${normalizeTokenID(options.focusTokenID)}"]`)?.focus();
    }
  }

  async function refreshPanel(options = {}) {
    if (state.loading) return;
    const lifecycleID = state.lifecycleID;
    const refreshSequence = ++state.refreshSequence;
    state.loading = true;
    state.loadError = null;
    state.tokenLoadError = null;
    state.recentChannelCacheStats = new Map();
    state.recentTokenCacheStats = new Map();
    state.recentChannel50CacheStats = new Map();
    state.recentToken50CacheStats = new Map();
    updateChrome();
    if (!state.loaded || !state.tokenLoaded) renderPanel();

    try {
      const groupRequest = window.fetchDataWithAuth('/admin/channel-groups').catch((error) => {
        console.warn('Channel group metadata unavailable:', error);
        return { groups: [] };
      });
      const channelRequest = window.fetchDataWithAuth('/admin/channels')
        .then((data) => ({ ok: true, data }))
        .catch((error) => ({ ok: false, error }));
      const tokenRequest = window.fetchDataWithAuth('/admin/auth-tokens')
        .then((data) => ({ ok: true, data }))
        .catch((error) => ({ ok: false, error }));
      const recentRange = buildRecentCacheRange();
      const recentRangeQuery = new URLSearchParams({
        range: 'custom',
        start_time: String(recentRange.startMs),
        end_time: String(recentRange.endMs)
      }).toString();
      const recentChannelCacheRequest = window.fetchDataWithAuth(`/admin/stats?${recentRangeQuery}`)
        .then((data) => ({ ok: true, data }))
        .catch((error) => ({ ok: false, error }));
      const recentTokenCacheRequest = window.fetchDataWithAuth(`/admin/auth-tokens?${recentRangeQuery}`)
        .then((data) => ({ ok: true, data }))
        .catch((error) => ({ ok: false, error }));
      const recentRequestCacheRequest = window.fetchDataWithAuth(`/admin/cache-stats/recent?${buildRecentRequestCacheQuery()}`)
        .then((data) => ({ ok: true, data }))
        .catch((error) => ({ ok: false, error }));
      const [channelResult, groupData, tokenResult, recentChannelCacheResult, recentTokenCacheResult, recentRequestCacheResult] = await Promise.all([
        channelRequest,
        groupRequest,
        tokenRequest,
        recentChannelCacheRequest,
        recentTokenCacheRequest,
        recentRequestCacheRequest
      ]);
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;

      if (channelResult && channelResult.ok && Array.isArray(channelResult.data)) {
        state.channels = channelResult.data;
        state.groups = Array.isArray(groupData && groupData.groups) ? groupData.groups : [];
        state.loaded = true;
        state.lastLoadedAt = Date.now();
      } else {
        state.loadError = channelResult && channelResult.error
          ? channelResult.error
          : new Error('Invalid channel list');
        console.error('Failed to load quick channel controls:', state.loadError);
      }

      if (tokenResult && tokenResult.ok) {
        const tokenData = tokenResult.data;
        const tokenList = Array.isArray(tokenData)
          ? tokenData
          : (tokenData && Array.isArray(tokenData.tokens) ? tokenData.tokens : null);
        if (tokenList) {
          state.tokens = tokenList;
          state.tokenGroups = Array.isArray(tokenData && tokenData.groups) ? tokenData.groups : [];
          state.tokenLoaded = true;
          state.lastLoadedAt = Date.now();
        } else {
          state.tokenLoadError = new Error('Invalid token list');
          console.error('Failed to load quick token controls:', state.tokenLoadError);
        }
      } else {
        state.tokenLoadError = tokenResult && tokenResult.error
          ? tokenResult.error
          : new Error('Failed to load token list');
        console.error('Failed to load quick token controls:', state.tokenLoadError);
      }

      if (recentChannelCacheResult && recentChannelCacheResult.ok) {
        state.recentChannelCacheStats = buildChannelCacheStats(recentChannelCacheResult.data);
      } else {
        const error = recentChannelCacheResult && recentChannelCacheResult.error
          ? recentChannelCacheResult.error
          : new Error('Failed to load recent channel cache stats');
        console.warn('Failed to load recent channel cache stats:', error);
      }

      if (recentTokenCacheResult && recentTokenCacheResult.ok) {
        state.recentTokenCacheStats = buildTokenCacheStats(recentTokenCacheResult.data);
      } else {
        const error = recentTokenCacheResult && recentTokenCacheResult.error
          ? recentTokenCacheResult.error
          : new Error('Failed to load recent token cache stats');
        console.warn('Failed to load recent token cache stats:', error);
      }

      if (recentRequestCacheResult && recentRequestCacheResult.ok) {
        const recentRequestStats = buildRecentRequestCacheStats(recentRequestCacheResult.data);
        state.recentChannel50CacheStats = recentRequestStats.channels;
        state.recentToken50CacheStats = recentRequestStats.tokens;
      } else {
        const error = recentRequestCacheResult && recentRequestCacheResult.error
          ? recentRequestCacheResult.error
          : new Error('Failed to load recent request cache stats');
        console.warn('Failed to load recent request cache stats:', error);
      }

      if (!options.silent) {
        if (state.activeTab === TOKEN_TAB && state.tokenLoadError && state.tokenLoaded) {
          setStatus(translate('logs.channelPanel.tokenLoadFailed'), 'error');
        } else if (state.activeTab === CHANNEL_TAB && state.loadError && state.loaded) {
          setStatus(translate('logs.channelPanel.loadFailed'), 'error');
        }
      }
    } catch (error) {
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;
      state.loadError = error;
      console.error('Failed to load quick controls:', error);
      if (!options.silent) setStatus(translate(state.activeTab === TOKEN_TAB
        ? 'logs.channelPanel.tokenLoadFailed'
        : 'logs.channelPanel.loadFailed'), 'error');
    } finally {
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;
      state.loading = false;
      renderPanel();
    }
  }

  function setExpanded(expanded, options = {}) {
    state.expanded = Boolean(expanded);
    if (options.restoreFocusID) state.restoreFocusID = options.restoreFocusID;
    updateChrome();
    if (!state.expanded) {
      cancelPointerDrag(false);
      cancelNativeDrag(false);
      if (options.restoreFocus !== false) {
        const restoreID = options.restoreFocusID || state.restoreFocusID || 'logsChannelPanelTrigger';
        getElement(restoreID)?.focus();
      }
      return;
    }

    renderPanel();
    const stale = Date.now() - state.lastLoadedAt > REFRESH_MAX_AGE_MS;
    if (!state.loaded || !state.tokenLoaded || stale) {
      void refreshPanel({ silent: state.loaded && state.tokenLoaded });
    }
    const lifecycleID = state.lifecycleID;
    requestAnimationFrame(() => {
      if (lifecycleID !== state.lifecycleID || !state.expanded) return;
      state.root?.querySelector('[data-channel-panel-action="collapse"]')?.focus();
    });
  }

  function setActiveTab(tab, options = {}) {
    const nextTab = normalizeTab(tab);
    state.activeTab = nextTab;
    state.restoreFocusID = nextTab === TOKEN_TAB ? 'logsTokenPanelTrigger' : 'logsChannelPanelTrigger';
    persistActiveTab();
    renderPanel();

    if (state.expanded && nextTab === TOKEN_TAB && (!state.tokenLoaded || state.tokenLoadError) && !state.loading) {
      void refreshPanel({ silent: state.tokenLoaded });
    }
    if (options.focus !== false) {
      requestAnimationFrame(() => {
        if (!state.expanded || !state.root) return;
        const target = nextTab === TOKEN_TAB
          ? getElement('logsChannelPanelTokenTab')
          : getElement('logsChannelPanelChannelTab');
        target?.focus();
      });
    }
  }

  async function toggleChannel(channelID) {
    const id = normalizeChannelID(channelID);
    const channel = getChannel(id);
    if (!channel || state.pendingChannelIDs.has(id)) return;
    const previousEnabled = channel.enabled === true;
    const nextEnabled = !previousEnabled;
    const name = String(channel.name || `#${id}`);
    const lifecycleID = state.lifecycleID;

    channel.enabled = nextEnabled;
    state.pendingChannelIDs.add(id);
    renderPanel({ focusChannelID: id, focusAction: 'toggle-channel' });

    try {
      const result = await window.fetchDataWithAuth('/admin/channels/batch-enabled', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel_ids: [id], enabled: nextEnabled })
      });
      if (lifecycleID !== state.lifecycleID) return;
      if (Number(result && result.not_found_count) > 0) throw new Error('Channel not found');
      setStatus(translate(nextEnabled ? 'logs.channelPanel.toggleEnabled' : 'logs.channelPanel.toggleDisabled', { name }), 'success');
    } catch (error) {
      if (lifecycleID !== state.lifecycleID) return;
      channel.enabled = previousEnabled;
      console.error('Failed to toggle channel from logs:', error);
      setStatus(translate('logs.channelPanel.toggleFailed', { name }), 'error');
    } finally {
      if (lifecycleID !== state.lifecycleID) return;
      state.pendingChannelIDs.delete(id);
      renderPanel({ focusChannelID: id, focusAction: 'toggle-channel' });
    }
  }

  async function toggleToken(tokenID) {
    const id = normalizeTokenID(tokenID);
    const token = getToken(id);
    if (!token || state.pendingTokenIDs.has(id)) return;
    const previousActive = isTokenActive(token);
    const nextActive = !previousActive;
    const name = String(token.description || `#${id}`);
    const lifecycleID = state.lifecycleID;

    token.is_active = nextActive;
    state.pendingTokenIDs.add(id);
    renderPanel({ focusTokenID: id, focusAction: 'toggle-token' });

    try {
      await window.fetchDataWithAuth(`/admin/auth-tokens/${encodeURIComponent(id)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_active: nextActive })
      });
      if (lifecycleID !== state.lifecycleID) return;
      setStatus(translate(nextActive
        ? 'logs.channelPanel.tokenToggleEnabled'
        : 'logs.channelPanel.tokenToggleDisabled', { name }), 'success');
    } catch (error) {
      if (lifecycleID !== state.lifecycleID) return;
      token.is_active = previousActive;
      console.error('Failed to toggle token from logs:', error);
      setStatus(translate('logs.channelPanel.tokenToggleFailed', { name }), 'error');
    } finally {
      if (lifecycleID !== state.lifecycleID) return;
      state.pendingTokenIDs.delete(id);
      renderPanel({ focusTokenID: id, focusAction: 'toggle-token' });
    }
  }

  function channelIDsInList(list) {
    return Array.from(list.querySelectorAll('.logs-channel-panel__row'))
      .map((row) => normalizeChannelID(row.dataset.channelId))
      .filter(Boolean);
  }

  function arraysEqual(left, right) {
    return left.length === right.length && left.every((value, index) => value === right[index]);
  }

  async function commitGroupOrder(groupID, orderedIDs, focusChannelID = 0) {
    if (state.orderSaving) return;
    const lifecycleID = state.lifecycleID;
    const key = normalizeGroupKey(groupID);
    const currentGroup = buildChannelGroups(state.channels, state.groups, translate('logs.channelPanel.ungrouped'))
      .find((group) => group.key === key);
    const currentIDs = currentGroup ? currentGroup.channels.map((channel) => normalizeChannelID(channel.id)) : [];
    if (arraysEqual(currentIDs, orderedIDs)) {
      renderPanel({ focusChannelID, focusAction: 'drag' });
      return;
    }

    const updates = buildPriorityUpdatesAfterGroupReorder(state.channels, key, orderedIDs);
    if (updates.length === 0) {
      renderPanel({ focusChannelID, focusAction: 'drag' });
      return;
    }

    const previousPriorities = new Map(state.channels.map((channel) => [
      normalizeChannelID(channel.id),
      normalizePriority(channel.priority)
    ]));
    const channelsByID = new Map(state.channels.map((channel) => [normalizeChannelID(channel.id), channel]));
    for (const update of updates) {
      const channel = channelsByID.get(update.id);
      if (channel) channel.priority = update.priority;
    }

    state.orderSaving = true;
    setStatus(translate('logs.channelPanel.orderSaving'), 'working', 0);
    renderPanel({ focusChannelID, focusAction: 'drag' });

    try {
      await window.fetchDataWithAuth('/admin/channels/batch-priority', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ updates })
      });
      if (lifecycleID !== state.lifecycleID) return;
      setStatus(translate('logs.channelPanel.orderSaved'), 'success');
    } catch (error) {
      if (lifecycleID !== state.lifecycleID) return;
      for (const channel of state.channels) {
        const previous = previousPriorities.get(normalizeChannelID(channel.id));
        if (previous !== undefined) channel.priority = previous;
      }
      console.error('Failed to save channel order from logs:', error);
      setStatus(translate('logs.channelPanel.orderFailed'), 'error');
    } finally {
      if (lifecycleID !== state.lifecycleID) return;
      state.orderSaving = false;
      renderPanel({ focusChannelID, focusAction: 'drag' });
    }
  }

  function moveDraggedRow(row, list, target, clientY) {
    if (!row || !list || row.parentElement !== list) return;
    if (!target || target === row) {
      if (!target && list.lastElementChild !== row) list.appendChild(row);
      return;
    }
    const bounds = target.getBoundingClientRect();
    const insertBefore = clientY < bounds.top + bounds.height / 2;
    list.insertBefore(row, insertBefore ? target : target.nextElementSibling);
  }

  function handleMouseDown(event) {
    const handle = event.target.closest('[data-channel-panel-action="drag"]');
    if (!handle || state.orderSaving || handle.getAttribute('aria-disabled') === 'true') {
      state.nativeDragArmed = null;
      return;
    }
    state.nativeDragArmed = {
      handle,
      row: handle.closest('.logs-channel-panel__row')
    };
  }

  function handleMouseUp() {
    if (!state.nativeDrag) state.nativeDragArmed = null;
  }

  function handleDragStart(event) {
    const row = event.target.closest('.logs-channel-panel__row');
    if (!row || !state.nativeDragArmed || state.nativeDragArmed.row !== row || state.orderSaving) {
      event.preventDefault();
      return;
    }
    const list = row.closest('.logs-channel-panel__group-list');
    if (!list) {
      event.preventDefault();
      return;
    }
    state.nativeDrag = {
      row,
      list,
      groupID: normalizeGroupKey(row.dataset.groupId)
    };
    row.classList.add('is-dragging');
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      try {
        event.dataTransfer.setData('text/plain', row.dataset.channelId || '');
      } catch (_) {
        // Firefox only needs the call to be attempted.
      }
    }
  }

  function handleDragOver(event) {
    if (!state.nativeDrag) return;
    const list = event.target.closest('.logs-channel-panel__group-list');
    if (!list || list !== state.nativeDrag.list) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    const target = event.target.closest('.logs-channel-panel__row');
    moveDraggedRow(state.nativeDrag.row, list, target, event.clientY);
  }

  function handleDrop(event) {
    if (!state.nativeDrag) return;
    const list = event.target.closest('.logs-channel-panel__group-list');
    if (!list || list !== state.nativeDrag.list) return;
    event.preventDefault();
    const { row, groupID } = state.nativeDrag;
    const orderedIDs = channelIDsInList(list);
    const focusChannelID = normalizeChannelID(row.dataset.channelId);
    row.classList.remove('is-dragging');
    state.nativeDrag = null;
    state.nativeDragArmed = null;
    void commitGroupOrder(groupID, orderedIDs, focusChannelID);
  }

  function cancelNativeDrag(rerender = true) {
    if (state.nativeDrag && state.nativeDrag.row) {
      state.nativeDrag.row.classList.remove('is-dragging');
    }
    state.nativeDrag = null;
    state.nativeDragArmed = null;
    if (rerender && state.expanded && state.loaded) renderPanel();
  }

  function handleDragEnd() {
    if (state.nativeDrag) cancelNativeDrag(true);
    else state.nativeDragArmed = null;
  }

  function handlePointerDown(event) {
    if (event.pointerType === 'mouse') return;
    const handle = event.target.closest('[data-channel-panel-action="drag"]');
    if (!handle || state.orderSaving || handle.getAttribute('aria-disabled') === 'true') return;
    const row = handle.closest('.logs-channel-panel__row');
    const list = row && row.closest('.logs-channel-panel__group-list');
    if (!row || !list) return;
    state.pointerDrag = {
      pointerID: event.pointerId,
      handle,
      row,
      list,
      groupID: normalizeGroupKey(row.dataset.groupId),
      startX: event.clientX,
      startY: event.clientY,
      moved: false
    };
    try {
      handle.setPointerCapture(event.pointerId);
    } catch (_) {
      // Pointer capture is not available in all embedded browsers.
    }
    event.preventDefault();
  }

  function handlePointerMove(event) {
    const drag = state.pointerDrag;
    if (!drag || drag.pointerID !== event.pointerId) return;
    const distance = Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY);
    if (!drag.moved && distance < 6) return;
    if (!drag.moved) {
      drag.moved = true;
      drag.row.classList.add('is-dragging');
    }
    event.preventDefault();
    const pointTarget = document.elementFromPoint(event.clientX, event.clientY);
    const list = pointTarget && pointTarget.closest('.logs-channel-panel__group-list');
    if (list !== drag.list) return;
    const target = pointTarget.closest('.logs-channel-panel__row');
    moveDraggedRow(drag.row, drag.list, target, event.clientY);
  }

  function finishPointerDrag(event, commit) {
    const drag = state.pointerDrag;
    if (!drag || (event && drag.pointerID !== event.pointerId)) return;
    state.pointerDrag = null;
    drag.row.classList.remove('is-dragging');
    try {
      drag.handle.releasePointerCapture(drag.pointerID);
    } catch (_) {
      // Ignore missing pointer capture.
    }
    if (commit && drag.moved) {
      const orderedIDs = channelIDsInList(drag.list);
      void commitGroupOrder(drag.groupID, orderedIDs, normalizeChannelID(drag.row.dataset.channelId));
    } else if (drag.moved && state.expanded && state.loaded) {
      renderPanel();
    }
  }

  function cancelPointerDrag(rerender = true) {
    const drag = state.pointerDrag;
    if (!drag) return;
    state.pointerDrag = null;
    drag.row.classList.remove('is-dragging');
    try {
      drag.handle.releasePointerCapture(drag.pointerID);
    } catch (_) {
      // Ignore missing pointer capture.
    }
    if (rerender && drag.moved && state.expanded && state.loaded) renderPanel();
  }

  function handlePointerUp(event) {
    finishPointerDrag(event, true);
  }

  function handlePointerCancel(event) {
    finishPointerDrag(event, false);
  }

  function handleRootKeydown(event) {
    const tab = event.target.closest('[role="tab"]');
    if (tab && ['ArrowLeft', 'ArrowRight'].includes(event.key)) {
      event.preventDefault();
      setActiveTab(event.key === 'ArrowRight' ? TOKEN_TAB : CHANNEL_TAB);
      return;
    }
    const handle = event.target.closest('[data-channel-panel-action="drag"]');
    if (!handle || state.orderSaving || !['ArrowUp', 'ArrowDown'].includes(event.key)) return;
    const row = handle.closest('.logs-channel-panel__row');
    const list = row && row.closest('.logs-channel-panel__group-list');
    if (!row || !list) return;
    const sibling = event.key === 'ArrowUp' ? row.previousElementSibling : row.nextElementSibling;
    if (!sibling) return;
    event.preventDefault();
    if (event.key === 'ArrowUp') list.insertBefore(row, sibling);
    else list.insertBefore(sibling, row);
    void commitGroupOrder(row.dataset.groupId, channelIDsInList(list), normalizeChannelID(row.dataset.channelId));
  }

  function handleRootClick(event) {
    const actionTarget = event.target.closest('[data-channel-panel-action]');
    if (!actionTarget || !state.root || !state.root.contains(actionTarget)) return;
    const action = actionTarget.dataset.channelPanelAction;
    if (action === 'expand') {
      setActiveTab(CHANNEL_TAB, { focus: false });
      setExpanded(true, { restoreFocusID: 'logsChannelPanelTrigger' });
    } else if (action === 'open-token') {
      setActiveTab(TOKEN_TAB, { focus: false });
      setExpanded(true, { restoreFocusID: 'logsTokenPanelTrigger' });
    } else if (action === 'collapse') {
      setExpanded(false);
    } else if (action === 'refresh' || action === 'retry') {
      void refreshPanel();
    } else if (action === 'select-tab') {
      setActiveTab(actionTarget.dataset.panelTab);
    } else if (action === 'toggle-channel') {
      void toggleChannel(actionTarget.dataset.channelId);
    } else if (action === 'edit-channel' && typeof window.openLogChannelEditor === 'function') {
      void window.openLogChannelEditor(actionTarget.dataset.channelId);
    } else if (action === 'toggle-token') {
      void toggleToken(actionTarget.dataset.tokenId);
    } else if (action === 'edit-token' && typeof window.openLogTokenEditor === 'function') {
      void window.openLogTokenEditor(actionTarget.dataset.tokenId);
    } else if (action === 'toggle-group') {
      const key = normalizeGroupKey(actionTarget.dataset.groupId);
      if (state.collapsedGroups.has(key)) state.collapsedGroups.delete(key);
      else state.collapsedGroups.add(key);
      persistCollapsedGroups();
      renderPanel({ focusGroupKey: key });
    } else if (action === 'toggle-token-group') {
      const key = `token:${normalizeGroupKey(actionTarget.dataset.groupId)}`;
      if (state.collapsedGroups.has(key)) state.collapsedGroups.delete(key);
      else state.collapsedGroups.add(key);
      persistCollapsedGroups();
      renderPanel({ focusGroupKey: normalizeGroupKey(actionTarget.dataset.groupId) });
    }
  }

  function handleDocumentKeydown(event) {
    if (event.key !== 'Escape' || !state.expanded) return;
    if (document.querySelector('.modal.show')) return;
    setExpanded(false);
  }

  function bindEvents() {
    const root = state.root;
    state.handlers.click = handleRootClick;
    state.handlers.keydown = handleRootKeydown;
    state.handlers.mousedown = handleMouseDown;
    state.handlers.dragstart = handleDragStart;
    state.handlers.dragover = handleDragOver;
    state.handlers.drop = handleDrop;
    state.handlers.dragend = handleDragEnd;
    state.handlers.pointerdown = handlePointerDown;
    state.handlers.pointermove = handlePointerMove;
    state.handlers.pointerup = handlePointerUp;
    state.handlers.pointercancel = handlePointerCancel;
    state.handlers.documentKeydown = handleDocumentKeydown;
    state.handlers.documentMouseup = handleMouseUp;

    root.addEventListener('click', state.handlers.click);
    root.addEventListener('keydown', state.handlers.keydown);
    root.addEventListener('mousedown', state.handlers.mousedown);
    root.addEventListener('dragstart', state.handlers.dragstart);
    root.addEventListener('dragover', state.handlers.dragover);
    root.addEventListener('drop', state.handlers.drop);
    root.addEventListener('dragend', state.handlers.dragend);
    root.addEventListener('pointerdown', state.handlers.pointerdown);
    root.addEventListener('pointermove', state.handlers.pointermove);
    root.addEventListener('pointerup', state.handlers.pointerup);
    root.addEventListener('pointercancel', state.handlers.pointercancel);
    document.addEventListener('keydown', state.handlers.documentKeydown);
    document.addEventListener('mouseup', state.handlers.documentMouseup);
  }

  function unbindEvents() {
    const root = state.root;
    if (!root) return;
    root.removeEventListener('click', state.handlers.click);
    root.removeEventListener('keydown', state.handlers.keydown);
    root.removeEventListener('mousedown', state.handlers.mousedown);
    root.removeEventListener('dragstart', state.handlers.dragstart);
    root.removeEventListener('dragover', state.handlers.dragover);
    root.removeEventListener('drop', state.handlers.drop);
    root.removeEventListener('dragend', state.handlers.dragend);
    root.removeEventListener('pointerdown', state.handlers.pointerdown);
    root.removeEventListener('pointermove', state.handlers.pointermove);
    root.removeEventListener('pointerup', state.handlers.pointerup);
    root.removeEventListener('pointercancel', state.handlers.pointercancel);
    document.removeEventListener('keydown', state.handlers.documentKeydown);
    document.removeEventListener('mouseup', state.handlers.documentMouseup);
    state.handlers = {};
  }

  function init() {
    const root = getElement(ROOT_ID);
    if (!root) return;
    if (state.initialized && state.root === root) return;
    if (state.initialized) destroy();

    state.root = root;
    state.initialized = true;
    state.lifecycleID += 1;
    state.refreshSequence += 1;
    state.expanded = false;
    state.activeTab = readActiveTab();
    state.restoreFocusID = state.activeTab === TOKEN_TAB ? 'logsTokenPanelTrigger' : 'logsChannelPanelTrigger';
    state.loaded = false;
    state.tokenLoaded = false;
    state.loading = false;
    state.loadError = null;
    state.tokenLoadError = null;
    state.lastLoadedAt = 0;
    state.channels = [];
    state.groups = [];
    state.tokens = [];
    state.tokenGroups = [];
    state.recentChannelCacheStats = new Map();
    state.recentTokenCacheStats = new Map();
    state.recentChannel50CacheStats = new Map();
    state.recentToken50CacheStats = new Map();
    state.pendingChannelIDs.clear();
    state.pendingTokenIDs.clear();
    state.orderSaving = false;
    state.collapsedGroups = readCollapsedGroups();
    bindEvents();
    if (window.i18n && typeof window.i18n.onLocaleChange === 'function') {
      state.localeUnsubscribe = window.i18n.onLocaleChange(() => renderPanel());
    }
    updateChrome();
  }

  function destroy() {
    if (!state.initialized) return;
    state.lifecycleID += 1;
    state.refreshSequence += 1;
    cancelPointerDrag(false);
    cancelNativeDrag(false);
    unbindEvents();
    if (typeof state.localeUnsubscribe === 'function') state.localeUnsubscribe();
    state.localeUnsubscribe = null;
    if (state.statusTimer) clearTimeout(state.statusTimer);
    state.statusTimer = null;
    state.root = null;
    state.initialized = false;
    state.expanded = false;
    state.activeTab = CHANNEL_TAB;
    state.restoreFocusID = 'logsChannelPanelTrigger';
  }

  window.LogsChannelQuickPanel = Object.freeze({
    init,
    destroy,
    refresh: refreshPanel,
    __test: Object.freeze({
      buildChannelGroups,
      buildPriorityUpdatesAfterGroupReorder,
      compareChannelsByPriority,
      formatDailyCost,
      normalizeGroupKey,
      renderChannelRow,
      normalizeTokenID,
      compareTokens,
      buildTokenGroups,
      renderTokenRow,
      tokenEnabledSummary,
      buildRecentCacheRange,
      buildCacheMetric,
      buildChannelCacheStats,
      buildTokenCacheStats,
      formatRecentCacheHitRate,
      formatRecentCacheHitRateShort,
      formatRecentCacheHitRateWindowShort,
      formatRecent50CacheHitRate,
      formatRecent50CacheHitRateShort,
      buildRecentRequestCacheQuery,
      buildRecentRequestCacheStats,
      buildChannelRecentRequestCacheStats,
      buildTokenRecentRequestCacheStats,
      buildRecentCacheRatesHtml,
      formatQuotaCost,
      getTokenEffectiveDailyCostLimit,
      getTokenEffectiveMonthlyCostLimit,
      getTokenEffectiveCostLimit,
      getTokenBatteryState,
      buildTokenBatteryHtml,
      buildTokenLimitBadgesHtml,
      formatTokenDailyCost
    })
  });
})();
