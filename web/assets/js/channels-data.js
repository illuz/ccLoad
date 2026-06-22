function buildChannelsListParams(type = 'all') {
  const params = new URLSearchParams();
  if (type && type !== 'all') {
    params.set('type', type);
  }
  return params;
}

async function loadChannels(type = 'all') {
  try {
    const params = buildChannelsListParams(type);
    const url = '/admin/channels?' + params.toString();
    const resp = await fetchAPIWithAuth(url);
    if (!resp.success) {
      throw new Error(resp.error || window.t('channels.loadChannelsFailed'));
    }

    channels = Array.isArray(resp.data) ? resp.data : [];
    channelsTotalCount = channels.length;

    if (typeof syncSelectedChannelsWithLoadedChannels === 'function') {
      syncSelectedChannelsWithLoadedChannels();
    }

    filterChannels();
  } catch (e) {
    console.error('Failed to load channels', e);
    if (window.showError) window.showError(window.t('channels.loadChannelsFailed'));
  }
}

function channelMatchesStatusSnapshot(channel, status = 'all') {
  if (!status || status === 'all') return true;
  const enabled = channel?.enabled === true;
  const isCooldown = Number(channel?.cooldown_remaining_ms || 0) > 0;
  if (status === 'enabled') return enabled;
  if (status === 'disabled') return !enabled;
  if (status === 'cooldown') return isCooldown;
  return true;
}

function recomputeLocalChannelFilterOptions(type = filters.channelType, status = filters.status) {
  const scopedChannels = (Array.isArray(channels) ? channels : []).filter((channel) => {
    const channelType = String(channel?.channel_type || '').trim().toLowerCase();
    if (type && type !== 'all' && channelType !== String(type).trim().toLowerCase()) return false;
    return channelMatchesStatusSnapshot(channel, status);
  });

  const nameSet = new Set();
  const modelSet = new Set();
  scopedChannels.forEach((channel) => {
    const name = String(channel?.name || '').trim();
    if (name) nameSet.add(name);
    (Array.isArray(channel?.models) ? channel.models : []).forEach((entry) => {
      const model = String(entry?.model || entry || '').trim();
      if (model) modelSet.add(model);
    });
  });

  allAvailableChannelNames = Array.from(nameSet).sort((a, b) => a.localeCompare(b));
  allAvailableModels = Array.from(modelSet).sort((a, b) => a.localeCompare(b));
  if (typeof updateModelOptions === 'function') updateModelOptions();
  if (typeof updateChannelNameOptions === 'function') updateChannelNameOptions();
}

function normalizeLocalChannel(channel = {}) {
  return {
    cooldown_remaining_ms: 0,
    key_cooldowns: [],
    ...channel,
    channel_type: String(channel?.channel_type || 'anthropic').trim().toLowerCase(),
    models: Array.isArray(channel?.models) ? channel.models : [],
    enabled: channel?.enabled !== false
  };
}

function upsertChannelLocal(channel, options = {}) {
  if (!channel || !Number(channel.id)) return null;
  const normalized = normalizeLocalChannel(channel);
  const type = options.type || filters.channelType || 'all';
  const matchesType = type === 'all' || normalized.channel_type === String(type).trim().toLowerCase();
  const index = (Array.isArray(channels) ? channels : []).findIndex((item) => Number(item?.id) === Number(normalized.id));

  if (!matchesType) {
    if (index >= 0) channels.splice(index, 1);
  } else if (index >= 0) {
    channels[index] = { ...channels[index], ...normalized };
  } else {
    channels.push(normalized);
  }

  channelsTotalCount = channels.length;
  if (typeof syncSelectedChannelsWithLoadedChannels === 'function') {
    syncSelectedChannelsWithLoadedChannels();
  }
  recomputeLocalChannelFilterOptions(type, filters.status);
  if (typeof filterChannels === 'function') filterChannels();
  return normalized;
}

function removeChannelsLocal(channelIDs = [], options = {}) {
  const ids = new Set((Array.isArray(channelIDs) ? channelIDs : [channelIDs]).map((id) => Number(id)).filter((id) => id > 0));
  if (ids.size === 0) return;
  channels = (Array.isArray(channels) ? channels : []).filter((channel) => !ids.has(Number(channel?.id)));
  channelsTotalCount = channels.length;
  ids.forEach((id) => selectedChannelIds.delete(normalizeSelectedChannelID(id)));
  if (typeof syncSelectedChannelsWithLoadedChannels === 'function') {
    syncSelectedChannelsWithLoadedChannels();
  }
  recomputeLocalChannelFilterOptions(options.type || filters.channelType, filters.status);
  if (typeof filterChannels === 'function') filterChannels();
}

function setChannelsEnabledLocal(channelIDs = [], enabled) {
  const ids = new Set((Array.isArray(channelIDs) ? channelIDs : [channelIDs]).map((id) => Number(id)).filter((id) => id > 0));
  if (ids.size === 0) return () => {};

  const snapshots = [];
  (Array.isArray(channels) ? channels : []).forEach((channel) => {
    if (!ids.has(Number(channel?.id))) return;
    snapshots.push({
      channel,
      enabled: channel.enabled,
      cooldown_remaining_ms: channel.cooldown_remaining_ms
    });
    channel.enabled = enabled;
    if (enabled) channel.cooldown_remaining_ms = 0;
  });

  recomputeLocalChannelFilterOptions(filters.channelType, filters.status);
  if (typeof filterChannels === 'function') filterChannels();

  return () => {
    snapshots.forEach((snapshot) => {
      snapshot.channel.enabled = snapshot.enabled;
      snapshot.channel.cooldown_remaining_ms = snapshot.cooldown_remaining_ms;
    });
    recomputeLocalChannelFilterOptions(filters.channelType, filters.status);
    if (typeof filterChannels === 'function') filterChannels();
  };
}

// CRUD 操作后同时刷新列表分页与筛选下拉全集
async function reloadChannelsList(type = filters.channelType, status = filters.status) {
  await Promise.all([
    loadChannelsFilterOptions(type, status),
    loadChannels(type)
  ]);
}

// 加载渠道筛选下拉的全集（按 type/status 联动），与列表分页/搜索/模型筛选解耦
async function loadChannelsFilterOptions(type = 'all', status = 'all') {
  try {
    const params = new URLSearchParams();
    if (type && type !== 'all') params.set('type', type);
    if (status && status !== 'all') params.set('status', status);
    const url = '/admin/channels/filter-options' + (params.toString() ? '?' + params.toString() : '');
    const data = await fetchDataWithAuth(url);
    allAvailableChannelNames = Array.isArray(data && data.channel_names) ? data.channel_names : [];
    allAvailableModels = Array.isArray(data && data.models) ? data.models : [];
  } catch (e) {
    console.error('Failed to load filter options', e);
    allAvailableChannelNames = [];
    allAvailableModels = [];
  }
  if (typeof updateModelOptions === 'function') updateModelOptions();
  if (typeof updateChannelNameOptions === 'function') updateChannelNameOptions();
}

async function loadChannelStatsRange() {
  try {
    const setting = await fetchDataWithAuth('/admin/settings/channel_stats_range');
    if (setting && setting.value) {
      channelStatsRange = setting.value;
    }
  } catch (e) {
    console.error('Failed to load stats range setting', e);
  }
}

async function loadChannelStats(range = channelStatsRange) {
  try {
    const params = new URLSearchParams({ range, limit: '500', offset: '0' });
    const data = await fetchDataWithAuth(`/admin/stats?${params.toString()}`);
    channelStatsById = aggregateChannelStats((data && data.stats) || [], data && data.channel_health);
    filterChannels();
  } catch (err) {
    console.error('Failed to load channel stats', err);
  }
}

function aggregateChannelStats(statsEntries = [], channelHealth = null) {
  const result = {};

  for (const entry of statsEntries) {
    const channelId = Number(entry.channel_id || entry.channelID);
    if (!Number.isFinite(channelId) || channelId <= 0) continue;

    if (!result[channelId]) {
      result[channelId] = {
        success: 0,
        error: 0,
        total: 0,
        totalInputTokens: 0,
        totalOutputTokens: 0,
        totalCacheReadInputTokens: 0,
        totalCacheCreationInputTokens: 0,
        totalCost: 0,
        effectiveCost: 0,
        lastSuccessAt: 0,
        lastSuccessID: 0,
        lastRequestAt: 0,
        lastRequestID: 0,
        lastRequestStatus: null,
        lastRequestMessage: '',
        _firstByteWeightedSum: 0,
        _firstByteWeight: 0,
        _durationWeightedSum: 0,
        _durationWeight: 0
      };
    }

    const stats = result[channelId];
    const success = toSafeNumber(entry.success);
    const error = toSafeNumber(entry.error);
    const total = toSafeNumber(entry.total);

    stats.success += success;
    stats.error += error;
    stats.total += total;

    const avgFirstByte = Number(entry.avg_first_byte_time_seconds);
    const weight = success || total || 0;
    if (Number.isFinite(avgFirstByte) && avgFirstByte > 0 && weight > 0) {
      stats._firstByteWeightedSum += avgFirstByte * weight;
      stats._firstByteWeight += weight;
    }

    const avgDuration = Number(entry.avg_duration_seconds);
    if (Number.isFinite(avgDuration) && avgDuration > 0 && weight > 0) {
      stats._durationWeightedSum += avgDuration * weight;
      stats._durationWeight += weight;
    }

    stats.totalInputTokens += toSafeNumber(entry.total_input_tokens);
    stats.totalOutputTokens += toSafeNumber(entry.total_output_tokens);
    stats.totalCacheReadInputTokens += toSafeNumber(entry.total_cache_read_input_tokens);
    stats.totalCacheCreationInputTokens += toSafeNumber(entry.total_cache_creation_input_tokens);
    stats.totalCost += toSafeNumber(entry.total_cost);
    stats.effectiveCost += (entry.effective_cost !== undefined && entry.effective_cost !== null)
      ? toSafeNumber(entry.effective_cost)
      : toSafeNumber(entry.total_cost);

    const lastSuccessAt = toTimestampMs(entry.last_success_at ?? entry.lastSuccessAt);
    const lastSuccessID = toPositiveNumber(entry.last_success_id ?? entry.lastSuccessId);
    if (lastSuccessAt > stats.lastSuccessAt
      || (lastSuccessAt > 0 && lastSuccessAt === stats.lastSuccessAt && lastSuccessID > stats.lastSuccessID)) {
      stats.lastSuccessAt = lastSuccessAt;
      stats.lastSuccessID = lastSuccessID;
    }

    const lastRequestAt = toTimestampMs(entry.last_request_at ?? entry.lastRequestAt);
    const lastRequestID = toPositiveNumber(entry.last_request_id ?? entry.lastRequestId);
    if (lastRequestAt > stats.lastRequestAt
      || (lastRequestAt > 0 && lastRequestAt === stats.lastRequestAt && lastRequestID > stats.lastRequestID)) {
      stats.lastRequestAt = lastRequestAt;
      stats.lastRequestID = lastRequestID;
      stats.lastRequestStatus = Number.isFinite(Number(entry.last_request_status ?? entry.lastRequestStatus))
        ? Number(entry.last_request_status ?? entry.lastRequestStatus)
        : null;
      stats.lastRequestMessage = entry.last_request_message || entry.lastRequestMessage || '';
    }
  }

  for (const id of Object.keys(result)) {
    const stats = result[id];
    if (stats._firstByteWeight > 0) {
      stats.avgFirstByteTimeSeconds = stats._firstByteWeightedSum / stats._firstByteWeight;
    }
    if (stats._durationWeight > 0) {
      stats.avgDurationSeconds = stats._durationWeightedSum / stats._durationWeight;
    }

    // 使用后端按渠道聚合的健康时间线（无需前端 merge）
    // 保留 rate=-1 的空桶，buildChannelHealthIndicator 会渲染为灰色
    if (channelHealth && channelHealth[id]) {
      stats.healthTimeline = channelHealth[id];
    }

    delete stats._firstByteWeightedSum;
    delete stats._firstByteWeight;
    delete stats._durationWeightedSum;
    delete stats._durationWeight;
  }

  return result;
}

function toSafeNumber(value) {
  const num = Number(value);
  return Number.isFinite(num) ? num : 0;
}

function toTimestampMs(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return 0;
  return num < 1e12 ? num * 1000 : num;
}

function toPositiveNumber(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return 0;
  return num;
}

// 加载默认测试内容（从系统设置）
async function loadDefaultTestContent() {
  try {
    const setting = await fetchDataWithAuth('/admin/settings/channel_test_content');
    if (setting && setting.value) {
      defaultTestContent = setting.value;
    }
  } catch (e) {
    console.warn('Failed to load default test content, using built-in default', e);
  }
}
