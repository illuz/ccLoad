(function () {
  'use strict';

  const ROOT_ID = 'logsChannelPanel';
  const COLLAPSED_GROUPS_KEY = 'ccload.logs.channelPanel.collapsedGroups';
  const REFRESH_MAX_AGE_MS = 15000;
  const GROUP_COLORS = new Set([
    '#64748b', '#ef4444', '#f97316', '#f59e0b',
    '#22c55e', '#14b8a6', '#3b82f6', '#8b5cf6'
  ]);

  const FALLBACK_TEXT = Object.freeze({
    'logs.channelPanel.title': 'Channel controls',
    'logs.channelPanel.open': 'Open channel controls',
    'logs.channelPanel.collapse': 'Collapse channel controls',
    'logs.channelPanel.refresh': 'Refresh channels',
    'logs.channelPanel.loading': 'Loading channels...',
    'logs.channelPanel.loadFailed': 'Failed to load channels',
    'logs.channelPanel.retry': 'Retry',
    'logs.channelPanel.empty': 'No channels',
    'logs.channelPanel.ungrouped': 'Ungrouped',
    'logs.channelPanel.enabledSummary': '{enabled}/{total} enabled',
    'logs.channelPanel.priority': 'Priority {priority}',
    'logs.channelPanel.cooldown': 'Cooling down',
    'logs.channelPanel.dragHandle': 'Reorder {name}',
    'logs.channelPanel.editChannel': 'Edit {name}',
    'logs.channelPanel.toggleEnabled': '{name} enabled',
    'logs.channelPanel.toggleDisabled': '{name} disabled',
    'logs.channelPanel.toggleFailed': 'Failed to update {name}',
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
    loaded: false,
    loading: false,
    loadError: null,
    lastLoadedAt: 0,
    channels: [],
    groups: [],
    collapsedGroups: new Set(),
    pendingChannelIDs: new Set(),
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
      return new Set(parsed.map(normalizeGroupKey));
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

  function getElement(id) {
    return document.getElementById(id);
  }

  function getChannel(channelID) {
    const id = normalizeChannelID(channelID);
    return state.channels.find((channel) => normalizeChannelID(channel && channel.id) === id) || null;
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
    const surface = getElement('logsChannelPanelSurface');
    const badge = getElement('logsChannelPanelBadge');
    const summary = getElement('logsChannelPanelSummary');
    const refresh = getElement('logsChannelPanelRefresh');
    const openLabel = translate('logs.channelPanel.open');
    const collapseLabel = translate('logs.channelPanel.collapse');
    const refreshLabel = translate('logs.channelPanel.refresh');

    state.root.dataset.state = state.expanded ? 'expanded' : 'collapsed';
    state.root.setAttribute('aria-label', translate('logs.channelPanel.title'));
    if (trigger) {
      trigger.hidden = state.expanded;
      trigger.setAttribute('aria-expanded', String(state.expanded));
      trigger.title = openLabel;
      trigger.setAttribute('aria-label', openLabel);
    }
    if (surface) {
      surface.hidden = !state.expanded;
      surface.classList.toggle('is-refreshing', state.loading && state.loaded);
    }
    if (refresh) {
      refresh.disabled = state.loading || state.orderSaving;
      refresh.title = refreshLabel;
      refresh.setAttribute('aria-label', refreshLabel);
    }
    const collapseButton = state.root.querySelector('[data-channel-panel-action="collapse"]');
    if (collapseButton) {
      collapseButton.title = collapseLabel;
      collapseButton.setAttribute('aria-label', collapseLabel);
    }

    const counts = enabledSummary();
    if (summary) {
      summary.textContent = state.loaded
        ? translate('logs.channelPanel.enabledSummary', counts)
        : '';
    }
    if (badge) {
      badge.hidden = !state.loaded;
      badge.textContent = counts.enabled > 99 ? '99+' : String(counts.enabled);
      badge.title = state.loaded ? translate('logs.channelPanel.enabledSummary', counts) : '';
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

  function renderChannelRow(channel, groupKey) {
    const id = normalizeChannelID(channel.id);
    const enabled = channel.enabled === true;
    const pending = state.pendingChannelIDs.has(id);
    const coolingDown = isChannelCoolingDown(channel);
    const name = String(channel.name || `#${id}`);
    const type = String(channel.channel_type || '--');
    const priority = normalizePriority(channel.priority);
    const dragLabel = translate('logs.channelPanel.dragHandle', { name });
    const editLabel = translate('logs.channelPanel.editChannel', { name });
    const switchLabel = translate(enabled ? 'channels.toggleDisable' : 'channels.toggleEnable');
    const cooldownHTML = coolingDown
      ? `<span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span><span class="logs-channel-panel__cooldown">${escapeHTML(translate('logs.channelPanel.cooldown'))}</span>`
      : '';

    return `
      <div class="logs-channel-panel__row${enabled ? '' : ' is-disabled'}" role="listitem" draggable="${state.orderSaving ? 'false' : 'true'}" data-channel-id="${id}" data-group-id="${escapeHTML(groupKey)}">
        <span class="logs-channel-panel__drag" role="button" tabindex="${state.orderSaving ? '-1' : '0'}" aria-disabled="${state.orderSaving ? 'true' : 'false'}" data-channel-panel-action="drag" title="${escapeHTML(dragLabel)}" aria-label="${escapeHTML(dragLabel)}">
          ${ICONS.grip}
        </span>
        <div class="logs-channel-panel__info">
          <div class="logs-channel-panel__name" title="${escapeHTML(name)}">${escapeHTML(name)}</div>
          <div class="logs-channel-panel__meta">
            <span class="logs-channel-panel__type">${escapeHTML(type)}</span>
            <span class="logs-channel-panel__meta-separator" aria-hidden="true">&middot;</span>
            <span class="logs-channel-panel__priority">${escapeHTML(translate('logs.channelPanel.priority', { priority }))}</span>
            ${cooldownHTML}
          </div>
        </div>
        <div class="logs-channel-panel__row-actions">
          <button type="button" class="logs-channel-panel__edit" data-channel-panel-action="edit-channel" data-channel-id="${id}" title="${escapeHTML(editLabel)}" aria-label="${escapeHTML(editLabel)}">${ICONS.edit}</button>
          <button type="button" class="logs-channel-panel__switch" data-channel-panel-action="toggle-channel" data-channel-id="${id}" role="switch" aria-checked="${enabled}" aria-disabled="${pending}" title="${escapeHTML(switchLabel)}" aria-label="${escapeHTML(switchLabel)}"></button>
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

  function renderPanel(options = {}) {
    updateChrome();
    if (!state.expanded) return;
    if (state.loading && !state.loaded) {
      renderState('logs.channelPanel.loading', { loading: true });
      return;
    }
    if (state.loadError && !state.loaded) {
      renderState('logs.channelPanel.loadFailed', { error: true });
      return;
    }

    const body = getElement('logsChannelPanelBody');
    if (!body) return;
    const scrollTop = body.scrollTop;
    const groups = buildChannelGroups(state.channels, state.groups, translate('logs.channelPanel.ungrouped'));
    if (groups.length === 0) {
      renderState('logs.channelPanel.empty');
      return;
    }
    body.innerHTML = groups.map(renderGroup).join('');
    body.scrollTop = scrollTop;

    if (options.focusGroupKey !== undefined) {
      body.querySelector(`[data-channel-panel-action="toggle-group"][data-group-id="${String(options.focusGroupKey).replace(/"/g, '')}"]`)?.focus();
    } else if (options.focusChannelID) {
      const action = options.focusAction || 'toggle-channel';
      body.querySelector(`[data-channel-panel-action="${action}"][data-channel-id="${normalizeChannelID(options.focusChannelID)}"]`)?.focus();
    }
  }

  async function refreshPanel(options = {}) {
    if (state.loading) return;
    const lifecycleID = state.lifecycleID;
    const refreshSequence = ++state.refreshSequence;
    state.loading = true;
    state.loadError = null;
    updateChrome();
    if (!state.loaded) renderPanel();

    try {
      const groupRequest = window.fetchDataWithAuth('/admin/channel-groups').catch((error) => {
        console.warn('Channel group metadata unavailable:', error);
        return { groups: [] };
      });
      const [channels, groupData] = await Promise.all([
        window.fetchDataWithAuth('/admin/channels'),
        groupRequest
      ]);
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;
      if (!Array.isArray(channels)) throw new Error('Invalid channel list');
      state.channels = channels;
      state.groups = Array.isArray(groupData && groupData.groups) ? groupData.groups : [];
      state.loaded = true;
      state.lastLoadedAt = Date.now();
    } catch (error) {
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;
      state.loadError = error;
      console.error('Failed to load quick channel controls:', error);
      if (state.loaded && !options.silent) {
        setStatus(translate('logs.channelPanel.loadFailed'), 'error');
      }
    } finally {
      if (lifecycleID !== state.lifecycleID || refreshSequence !== state.refreshSequence) return;
      state.loading = false;
      renderPanel();
    }
  }

  function setExpanded(expanded, options = {}) {
    state.expanded = Boolean(expanded);
    updateChrome();
    if (!state.expanded) {
      cancelPointerDrag(false);
      cancelNativeDrag(false);
      if (options.restoreFocus !== false) getElement('logsChannelPanelTrigger')?.focus();
      return;
    }

    renderPanel();
    const stale = Date.now() - state.lastLoadedAt > REFRESH_MAX_AGE_MS;
    if (!state.loaded || stale) void refreshPanel({ silent: state.loaded });
    const lifecycleID = state.lifecycleID;
    requestAnimationFrame(() => {
      if (lifecycleID !== state.lifecycleID || !state.expanded) return;
      state.root?.querySelector('[data-channel-panel-action="collapse"]')?.focus();
    });
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
      setExpanded(true);
    } else if (action === 'collapse') {
      setExpanded(false);
    } else if (action === 'refresh' || action === 'retry') {
      void refreshPanel();
    } else if (action === 'toggle-channel') {
      void toggleChannel(actionTarget.dataset.channelId);
    } else if (action === 'edit-channel' && typeof window.openLogChannelEditor === 'function') {
      void window.openLogChannelEditor(actionTarget.dataset.channelId);
    } else if (action === 'toggle-group') {
      const key = normalizeGroupKey(actionTarget.dataset.groupId);
      if (state.collapsedGroups.has(key)) state.collapsedGroups.delete(key);
      else state.collapsedGroups.add(key);
      persistCollapsedGroups();
      renderPanel({ focusGroupKey: key });
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
    state.loaded = false;
    state.loading = false;
    state.loadError = null;
    state.lastLoadedAt = 0;
    state.channels = [];
    state.groups = [];
    state.pendingChannelIDs.clear();
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
  }

  window.LogsChannelQuickPanel = Object.freeze({
    init,
    destroy,
    refresh: refreshPanel,
    __test: Object.freeze({
      buildChannelGroups,
      buildPriorityUpdatesAfterGroupReorder,
      compareChannelsByPriority,
      normalizeGroupKey
    })
  });
})();
