(function () {
  const TOKEN_EDITOR_NODE_IDS = [
    'editModal',
    'channelSelectModal',
    'modelSelectModal',
    'tpl-token-expiry-options'
  ];
  const TOKEN_EDITOR_REQUIRED_CONTROL_IDS = [
    'editDailyLimitDoubleEnabled',
    'editDailyLimitTripleEnabled'
  ];

  const TOKEN_EDITOR_STYLESHEET_ID = 'log-token-editor-stylesheet';
  const TOKEN_EDITOR_STYLESHEET_PATH = '/web/assets/css/tokens.css';

  let tokenEditorSetupPromise = null;
  let actionsBound = false;
  let styleLoadPromise = null;

  let editorTokens = [];
  let editorGroups = [];
  let allChannels = [];
  let availableModelsCache = [];
  let editAllowedModels = [];
  let editRawAllowedModels = [];
  let selectedAllowedModelIndices = new Set();
  let editAllowedChannelIDs = [];
  let editRawAllowedChannelIDs = [];
  let selectedAllowedChannelIDs = new Set();
  let selectedModelsForAdd = new Set();
  let currentVisibleModels = [];
  let selectedChannelsForAdd = new Set();
  let currentVisibleChannels = [];
  let editRawCostLimitUSD = 0;
  let editRawDailyCostLimitUSD = 0;
  let editRawMaxConcurrency = 0;
  let editInheritQuota = false;
  let editInheritChannels = false;
  let editInheritModels = false;

  function tr(key, params) {
    if (typeof window.t === 'function') return window.t(key, params);
    return key;
  }

  function i18nText(key, fallback) {
    const value = tr(key);
    return value && value !== key ? value : fallback;
  }

  function html(value) {
    if (typeof window.escapeHtml === 'function') return window.escapeHtml(value);
    return String(value ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function getAssetVersion() {
    const candidates = [
      '/web/assets/js/logs-token-editor.js',
      '/web/assets/js/logs.js'
    ];

    for (const candidate of candidates) {
      const script = Array.from(document.scripts).find((item) => {
        try {
          return item.src && new URL(item.src, window.location.origin).pathname === candidate;
        } catch (_) {
          return false;
        }
      });
      if (!script) continue;
      try {
        return new URL(script.src, window.location.origin).searchParams.get('v') || '';
      } catch (_) {
        return '';
      }
    }
    return '';
  }

  function versioned(path) {
    const version = getAssetVersion();
    if (!version) return path;
    const separator = path.includes('?') ? '&' : '?';
    return `${path}${separator}v=${encodeURIComponent(version)}`;
  }

  function ensureTokenEditorStylesheet() {
    const existing = document.getElementById(TOKEN_EDITOR_STYLESHEET_ID);
    if (existing) return styleLoadPromise || Promise.resolve();

    styleLoadPromise = new Promise((resolve) => {
      const link = document.createElement('link');
      link.id = TOKEN_EDITOR_STYLESHEET_ID;
      link.rel = 'stylesheet';
      link.href = versioned(TOKEN_EDITOR_STYLESHEET_PATH);
      link.onload = () => resolve();
      link.onerror = () => resolve();
      document.head.appendChild(link);
    });
    return styleLoadPromise;
  }

  function removeTokenEditorStylesheet() {
    const link = document.getElementById(TOKEN_EDITOR_STYLESHEET_ID);
    if (link) link.remove();
    styleLoadPromise = null;
  }

  async function fetchTokensDocument() {
    const response = await fetch(versioned('/web/tokens.html'), { credentials: 'same-origin' });
    if (!response.ok) {
      throw new Error(`Failed to fetch token editor markup: HTTP ${response.status}`);
    }
    const htmlText = await response.text();
    return new DOMParser().parseFromString(htmlText, 'text/html');
  }

  function syncNodeByID(sourceDocument, id, replaceExisting) {
    const sourceNode = sourceDocument.getElementById(id);
    if (!sourceNode) throw new Error(`Missing required token editor markup: ${id}`);
    const importedNode = document.importNode(sourceNode, true);
    const existingNode = document.getElementById(id);
    if (existingNode) {
      if (replaceExisting) existingNode.replaceWith(importedNode);
      return;
    }
    document.body.appendChild(importedNode);
  }

  function hasCompleteTokenEditorMarkup(rootDocument = document) {
    return [...TOKEN_EDITOR_NODE_IDS, ...TOKEN_EDITOR_REQUIRED_CONTROL_IDS]
      .every((id) => rootDocument.getElementById(id));
  }

  function initExpirySelects() {
    const template = document.getElementById('tpl-token-expiry-options');
    if (!template) return;
    const optionsHtml = template.innerHTML.trim();
    document.querySelectorAll('[data-expiry-select]').forEach((select) => {
      const current = select.value;
      select.innerHTML = optionsHtml;
      if (current) select.value = current;
    });
  }

  async function ensureTokenEditorMarkup() {
    if (hasCompleteTokenEditorMarkup()) return;
    const sourceDocument = await fetchTokensDocument();
    if (!hasCompleteTokenEditorMarkup(sourceDocument)) {
      throw new Error('Fetched token editor markup is incomplete');
    }
    TOKEN_EDITOR_NODE_IDS.forEach((id) => syncNodeByID(sourceDocument, id, true));
    initExpirySelects();
    if (window.i18n && typeof window.i18n.translatePage === 'function') {
      window.i18n.translatePage();
    }
  }

  function isEditorModalTarget(target) {
    return Boolean(target && target.closest && target.closest('#editModal, #channelSelectModal, #modelSelectModal'));
  }

  function getActionTarget(event, attr) {
    const target = event.target && event.target.closest ? event.target.closest(`[${attr}]`) : null;
    if (!target || !isEditorModalTarget(target)) return null;
    return target;
  }

  function bindTokenEditorActionsOnce() {
    if (actionsBound) return;
    actionsBound = true;

    document.body.addEventListener('click', (event) => {
      const actionTarget = getActionTarget(event, 'data-action');
      if (!actionTarget) return;

      const action = actionTarget.dataset.action;
      const handled = handleTokenEditorClickAction(action, actionTarget);
      if (handled) {
        event.preventDefault();
        event.stopPropagation();
      }
    });

    document.body.addEventListener('change', (event) => {
      const actionTarget = getActionTarget(event, 'data-change-action');
      if (!actionTarget) return;

      const action = actionTarget.dataset.changeAction;
      const handled = handleTokenEditorChangeAction(action, actionTarget);
      if (handled) {
        event.stopPropagation();
      }
    });

    document.body.addEventListener('input', (event) => {
      const actionTarget = getActionTarget(event, 'data-input-action');
      if (!actionTarget) return;

      const action = actionTarget.dataset.inputAction;
      const handled = handleTokenEditorInputAction(action, actionTarget);
      if (handled) {
        event.stopPropagation();
      }
    });

    document.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape') return;
      if (isModalOpen('modelSelectModal')) {
        closeModelSelectModal();
      } else if (isModalOpen('channelSelectModal')) {
        closeChannelSelectModal();
      } else if (isModalOpen('editModal')) {
        closeEditModal();
      }
    });
  }

  function handleTokenEditorClickAction(action, actionTarget) {
    switch (action) {
      case 'close-edit-modal':
        closeEditModal();
        return true;
      case 'update-token':
        void updateToken();
        return true;
      case 'copy-edit-token':
        copyEditToken();
        return true;
      case 'show-channel-select-modal':
        void showChannelSelectModal();
        return true;
      case 'batch-delete-allowed-channels':
        batchDeleteSelectedAllowedChannels();
        return true;
      case 'remove-allowed-channel':
        removeAllowedChannel(Number(actionTarget.dataset.channelId));
        return true;
      case 'close-channel-select-modal':
        closeChannelSelectModal();
        return true;
      case 'confirm-channel-selection':
        confirmChannelSelection();
        return true;
      case 'show-model-select-modal':
        void showModelSelectModal();
        return true;
      case 'show-model-import-modal':
        showModelImportPrompt();
        return true;
      case 'batch-delete-allowed-models':
        batchDeleteSelectedAllowedModels();
        return true;
      case 'remove-allowed-model':
        removeAllowedModel(Number(actionTarget.dataset.index));
        return true;
      case 'close-model-select-modal':
        closeModelSelectModal();
        return true;
      case 'confirm-model-selection':
        confirmModelSelection();
        return true;
      default:
        return false;
    }
  }

  function handleTokenEditorChangeAction(action, actionTarget) {
    switch (action) {
      case 'toggle-edit-custom-expiry':
        toggleEditCustomExpiry(actionTarget.value);
        return true;
      case 'change-edit-token-group':
        changeEditTokenGroup(actionTarget.value);
        return true;
      case 'toggle-inherit-quota':
        setEditInheritQuota(actionTarget.checked);
        return true;
      case 'toggle-daily-limit-multiplier':
        enforceDailyLimitMultiplierExclusivity(actionTarget);
        return true;
      case 'toggle-inherit-channels':
        setEditInheritChannels(actionTarget.checked);
        return true;
      case 'toggle-inherit-models':
        setEditInheritModels(actionTarget.checked);
        return true;
      case 'toggle-select-all-allowed-channels':
        toggleSelectAllAllowedChannels(actionTarget.checked);
        return true;
      case 'toggle-allowed-channel':
        toggleAllowedChannelSelection(Number(actionTarget.dataset.channelId), actionTarget.checked);
        return true;
      case 'toggle-select-all-channels':
        toggleSelectAllChannels(actionTarget.checked);
        return true;
      case 'filter-available-channel-type':
        filterAvailableChannels(document.getElementById('channelSearchInput')?.value || '');
        return true;
      case 'toggle-select-all-allowed-models':
        toggleSelectAllAllowedModels(actionTarget.checked);
        return true;
      case 'toggle-allowed-model':
        toggleAllowedModelSelection(Number(actionTarget.dataset.index), actionTarget.checked);
        return true;
      case 'toggle-select-all-models':
        toggleSelectAllModels(actionTarget.checked);
        return true;
      default:
        return false;
    }
  }

  function enforceDailyLimitMultiplierExclusivity(changedInput) {
    if (!changedInput?.checked) return;
    const otherID = changedInput.id === 'editDailyLimitTripleEnabled'
      ? 'editDailyLimitDoubleEnabled'
      : 'editDailyLimitTripleEnabled';
    const otherInput = document.getElementById(otherID);
    if (otherInput) otherInput.checked = false;
  }

  function handleTokenEditorInputAction(action, actionTarget) {
    switch (action) {
      case 'filter-available-channels':
        filterAvailableChannels(actionTarget.value);
        return true;
      case 'filter-available-models':
        filterAvailableModels(actionTarget.value);
        return true;
      default:
        return false;
    }
  }

  async function ensureLogTokenEditorReady() {
    if (!tokenEditorSetupPromise) {
      tokenEditorSetupPromise = (async () => {
        await ensureTokenEditorMarkup();
        bindTokenEditorActionsOnce();
      })();
    }
    const setupPromise = tokenEditorSetupPromise;

    try {
      await setupPromise;
    } finally {
      if (tokenEditorSetupPromise === setupPromise) {
        tokenEditorSetupPromise = null;
      }
    }
  }

  async function loadEditorTokens() {
    const data = await fetchDataWithAuth('/admin/auth-tokens');
    editorTokens = (data && data.tokens) || [];
    if (Array.isArray(data && data.groups)) {
      editorGroups = data.groups;
    }
    return editorTokens;
  }

  async function loadAuthTokenGroups() {
    const data = await fetchDataWithAuth('/admin/auth-token-groups');
    editorGroups = (data && data.groups) || [];
    return editorGroups;
  }

  async function ensureAuthTokenGroupsLoaded() {
    if (editorGroups.length > 0) return editorGroups;
    try {
      return await loadAuthTokenGroups();
    } catch (error) {
      console.error('Failed to load token groups:', error);
      editorGroups = [];
      return editorGroups;
    }
  }

  async function loadChannelsData() {
    try {
      const data = await fetchDataWithAuth('/admin/channels');
      allChannels = Array.isArray(data) ? data : ((data && (data.channels || data.data)) || []);
      availableModelsCache = getAvailableModels();
    } catch (error) {
      console.error('Failed to load channels data:', error);
      allChannels = [];
      availableModelsCache = [];
    }
    return allChannels;
  }

  function getAvailableModels() {
    const modelSet = new Set();
    allChannels.forEach((channel) => {
      (channel.models || []).forEach((entry) => {
        const model = String(entry && (entry.model || entry) || '').trim();
        if (model) modelSet.add(model);
      });
    });
    return Array.from(modelSet).sort();
  }

  function getAvailableModelsForCurrentChannelRestriction() {
    if (editAllowedChannelIDs.length === 0) return availableModelsCache;
    const allowed = new Set(editAllowedChannelIDs.map(normalizeChannelID));
    const modelSet = new Set();
    allChannels.forEach((channel) => {
      if (!allowed.has(normalizeChannelID(channel.id))) return;
      (channel.models || []).forEach((entry) => {
        const model = String(entry && (entry.model || entry) || '').trim();
        if (model) modelSet.add(model);
      });
    });
    return Array.from(modelSet).sort();
  }

  function getGroupByID(groupID) {
    const id = Number(groupID) || 0;
    if (id <= 0) return null;
    return editorGroups.find((group) => Number(group.id) === id) || null;
  }

  function buildTokenGroupOptionsHtml(selectedID = 0) {
    const current = String(Number(selectedID) || 0);
    return [
      `<option value="0"${current === '0' ? ' selected' : ''}>${html(tr('tokens.ungrouped'))}</option>`,
      ...editorGroups.map((group) => {
        const groupID = String(Number(group.id) || 0);
        const selected = groupID === current ? ' selected' : '';
        return `<option value="${html(groupID)}"${selected}>${html(group.name || (`#${groupID}`))}</option>`;
      })
    ].join('');
  }

  function refreshEditGroupOptions(selectedID) {
    const select = document.getElementById('editTokenGroup');
    if (!select) return;
    const current = String(Number(selectedID) || 0);
    select.innerHTML = buildTokenGroupOptionsHtml(current);
    if (!Array.from(select.options || []).some((option) => option.value === current)) {
      const option = document.createElement('option');
      option.value = current;
      option.textContent = `${tr('tokens.group')} #${current}`;
      select.appendChild(option);
    }
    select.value = current;
  }

  function normalizeChannelID(value) {
    const id = Number(value);
    return Number.isFinite(id) && id > 0 ? Math.trunc(id) : 0;
  }

  function getChannelByID(channelID) {
    const id = normalizeChannelID(channelID);
    return allChannels.find((channel) => normalizeChannelID(channel.id) === id) || null;
  }

  function getChannelDisplayName(channelID) {
    const channel = getChannelByID(channelID);
    if (!channel) return `${tr('common.unknown')} #${channelID}`;
    return `${channel.name || tr('common.unknown')} #${channel.id}`;
  }

  function getChannelTypeText(channelID) {
    const channel = getChannelByID(channelID);
    return channel ? (channel.channel_type || '-') : '-';
  }

  function parseMaxConcurrencyInput(rawValue) {
    const normalized = String(rawValue ?? '').trim();
    if (normalized === '') return { value: 0 };
    const parsed = Number(normalized);
    if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed < 0) {
      return { error: tr('tokens.msg.maxConcurrencyInteger') };
    }
    return { value: parsed };
  }

  function tokenExpiresAtMs(value) {
    if (value === null || value === undefined || value === '') return 0;
    const numberValue = Number(value);
    if (Number.isFinite(numberValue)) return numberValue;
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }

  function toDatetimeLocalValue(ms) {
    const date = new Date(ms);
    if (!Number.isFinite(date.getTime())) return '';
    return date.toISOString().slice(0, 16);
  }

  async function openLogTokenEditor(tokenId) {
    const id = Number(tokenId);
    if (!Number.isFinite(id) || id <= 0) return;

    try {
      await ensureLogTokenEditorReady();
      await ensureTokenEditorStylesheet();
      await Promise.all([
        loadEditorTokens(),
        ensureAuthTokenGroupsLoaded(),
        loadChannelsData()
      ]);

      const token = editorTokens.find((item) => Number(item.id) === Math.trunc(id));
      if (!token) {
        if (window.showError) window.showError(i18nText('tokens.msg.notFound', '令牌不存在'));
        return;
      }

      editTokenInModal(token);
    } catch (error) {
      console.error('Failed to open log token editor', error);
      if (window.showError) {
        window.showError(i18nText('tokens.msg.loadFailed', '无法加载令牌编辑器'));
      }
    }
  }

  function editTokenInModal(token) {
    const tokenID = Number(token.id) || 0;
    document.getElementById('editTokenId').value = String(tokenID);
    document.getElementById('editTokenValue').value = token.plain_token || '';
    document.getElementById('editTokenDescription').value = token.description || '';
    document.getElementById('editTokenActive').checked = token.is_active !== false;
    const editCodexGuardInput = document.getElementById('editCodexGuardEnabled');
    if (editCodexGuardInput) {
      editCodexGuardInput.checked = !!token.codex_guard_enabled;
    }
    refreshEditGroupOptions(token.group_id || 0);

    const expiresAt = tokenExpiresAtMs(token.expires_at);
    if (!expiresAt) {
      document.getElementById('editTokenExpiry').value = 'never';
      document.getElementById('editCustomExpiryContainer').style.display = 'none';
      document.getElementById('editCustomExpiry').value = '';
    } else {
      document.getElementById('editTokenExpiry').value = 'custom';
      document.getElementById('editCustomExpiryContainer').style.display = 'block';
      document.getElementById('editCustomExpiry').value = toDatetimeLocalValue(expiresAt);
    }

    editRawCostLimitUSD = Number(token.cost_limit_usd) || 0;
    editRawDailyCostLimitUSD = Number(token.daily_cost_limit_usd) || 0;
    editRawMaxConcurrency = Number(token.max_concurrency) || 0;
    document.getElementById('editCostLimitUSD').value = String(editRawCostLimitUSD);
    document.getElementById('editDailyCostLimitUSD').value = String(editRawDailyCostLimitUSD);
    document.getElementById('editMaxConcurrency').value = String(editRawMaxConcurrency);
    const editDailyLimitDoubleInput = document.getElementById('editDailyLimitDoubleEnabled');
    if (editDailyLimitDoubleInput) {
      editDailyLimitDoubleInput.checked = !!token.daily_limit_double_enabled;
    }
    const editDailyLimitTripleInput = document.getElementById('editDailyLimitTripleEnabled');
    if (editDailyLimitTripleInput) {
      editDailyLimitTripleInput.checked = !!token.daily_limit_triple_enabled;
    }

    const costUsed = Number(token.cost_used_usd) || 0;
    const dailyCostUsed = Number(token.daily_cost_used_usd) || 0;
    document.getElementById('editCostUsedDisplay').textContent = costUsed > 0 ? `${tr('tokens.costUsedPrefix')}: $${costUsed.toFixed(4)}` : '';
    document.getElementById('editDailyCostUsedDisplay').textContent = dailyCostUsed > 0 ? `${tr('tokens.costUsedPrefix')}: $${dailyCostUsed.toFixed(4)}` : '';

    editRawAllowedModels = (token.allowed_models || []).slice();
    editAllowedModels = editRawAllowedModels.slice();
    selectedAllowedModelIndices.clear();

    editAllowedChannelIDs = (token.allowed_channel_ids || []).map(normalizeChannelID).filter((item) => item > 0);
    editRawAllowedChannelIDs = editAllowedChannelIDs.slice();
    selectedAllowedChannelIDs.clear();

    editInheritQuota = !!token.inherit_quota && !!token.group_id;
    editInheritChannels = !!token.inherit_channels && !!token.group_id;
    editInheritModels = !!token.inherit_models && !!token.group_id;
    syncEditInheritanceUI({ preserveRaw: true });

    const modal = document.getElementById('editModal');
    modal.style.display = 'block';
    if (window.i18n && typeof window.i18n.translatePage === 'function') {
      window.i18n.translatePage();
    }
  }

  function isModalOpen(id) {
    return document.getElementById(id)?.style.display === 'block';
  }

  function closeEditModal() {
    const modal = document.getElementById('editModal');
    if (modal) modal.style.display = 'none';
    const valueInput = document.getElementById('editTokenValue');
    if (valueInput) valueInput.value = '';
    const expiry = document.getElementById('editCustomExpiryContainer');
    if (expiry) expiry.style.display = 'none';
    const editCodexGuardInput = document.getElementById('editCodexGuardEnabled');
    if (editCodexGuardInput) editCodexGuardInput.checked = false;
    const editDailyLimitDoubleInput = document.getElementById('editDailyLimitDoubleEnabled');
    if (editDailyLimitDoubleInput) editDailyLimitDoubleInput.checked = false;
    const editDailyLimitTripleInput = document.getElementById('editDailyLimitTripleEnabled');
    if (editDailyLimitTripleInput) editDailyLimitTripleInput.checked = false;

    editAllowedModels = [];
    editRawAllowedModels = [];
    selectedAllowedModelIndices.clear();
    editAllowedChannelIDs = [];
    editRawAllowedChannelIDs = [];
    selectedAllowedChannelIDs.clear();
    selectedModelsForAdd.clear();
    selectedChannelsForAdd.clear();
    editInheritQuota = false;
    editInheritChannels = false;
    editInheritModels = false;
    removeTokenEditorStylesheet();
  }

  function toggleEditCustomExpiry(value) {
    const container = document.getElementById('editCustomExpiryContainer');
    if (container) container.style.display = value === 'custom' ? 'block' : 'none';
  }

  function captureRawEditValues() {
    if (!editInheritQuota) {
      editRawCostLimitUSD = parseFloat(document.getElementById('editCostLimitUSD')?.value) || 0;
      editRawDailyCostLimitUSD = parseFloat(document.getElementById('editDailyCostLimitUSD')?.value) || 0;
      const maxResult = parseMaxConcurrencyInput(document.getElementById('editMaxConcurrency')?.value);
      if (!maxResult.error) editRawMaxConcurrency = maxResult.value;
    }
    if (!editInheritChannels) {
      editRawAllowedChannelIDs = editAllowedChannelIDs.slice();
    }
    if (!editInheritModels) {
      editRawAllowedModels = editAllowedModels.slice();
    }
  }

  function changeEditTokenGroup(rawValue) {
    const groupID = Number(rawValue) || 0;
    if (groupID > 0) {
      captureRawEditValues();
      editInheritQuota = true;
      editInheritChannels = true;
      editInheritModels = true;
    } else {
      editInheritQuota = false;
      editInheritChannels = false;
      editInheritModels = false;
    }
    syncEditInheritanceUI({ preserveRaw: true });
  }

  function setEditInheritQuota(checked) {
    if (checked && !editInheritQuota) captureRawEditValues();
    editInheritQuota = !!checked;
    syncEditInheritanceUI({ preserveRaw: true });
  }

  function setEditInheritChannels(checked) {
    if (checked && !editInheritChannels) captureRawEditValues();
    editInheritChannels = !!checked;
    syncEditInheritanceUI({ preserveRaw: true });
  }

  function setEditInheritModels(checked) {
    if (checked && !editInheritModels) captureRawEditValues();
    editInheritModels = !!checked;
    syncEditInheritanceUI({ preserveRaw: true });
  }

  function setControlDisabled(selector, disabled) {
    document.querySelectorAll(selector).forEach((element) => {
      element.disabled = !!disabled;
    });
  }

  function syncEditInheritanceUI(options = {}) {
    if (!options.preserveRaw) captureRawEditValues();

    const groupID = Number(document.getElementById('editTokenGroup')?.value) || 0;
    const group = getGroupByID(groupID);
    const hasGroup = !!group;
    if (!hasGroup) {
      editInheritQuota = false;
      editInheritChannels = false;
      editInheritModels = false;
    }

    const inheritQuotaEl = document.getElementById('editInheritQuota');
    const inheritChannelsEl = document.getElementById('editInheritChannels');
    const inheritModelsEl = document.getElementById('editInheritModels');
    if (inheritQuotaEl) {
      inheritQuotaEl.checked = editInheritQuota;
      inheritQuotaEl.disabled = !hasGroup;
    }
    if (inheritChannelsEl) {
      inheritChannelsEl.checked = editInheritChannels;
      inheritChannelsEl.disabled = !hasGroup;
    }
    if (inheritModelsEl) {
      inheritModelsEl.checked = editInheritModels;
      inheritModelsEl.disabled = !hasGroup;
    }

    const costLimitInput = document.getElementById('editCostLimitUSD');
    const dailyCostLimitInput = document.getElementById('editDailyCostLimitUSD');
    const maxConcurrencyInput = document.getElementById('editMaxConcurrency');
    if (editInheritQuota && group) {
      if (costLimitInput) costLimitInput.value = group.cost_limit_usd || 0;
      if (dailyCostLimitInput) dailyCostLimitInput.value = group.daily_cost_limit_usd || 0;
      if (maxConcurrencyInput) maxConcurrencyInput.value = group.max_concurrency || 0;
    } else {
      if (costLimitInput) costLimitInput.value = editRawCostLimitUSD || 0;
      if (dailyCostLimitInput) dailyCostLimitInput.value = editRawDailyCostLimitUSD || 0;
      if (maxConcurrencyInput) maxConcurrencyInput.value = editRawMaxConcurrency || 0;
    }
    if (costLimitInput) costLimitInput.disabled = editInheritQuota && hasGroup;
    if (dailyCostLimitInput) dailyCostLimitInput.disabled = editInheritQuota && hasGroup;
    if (maxConcurrencyInput) maxConcurrencyInput.disabled = editInheritQuota && hasGroup;

    editAllowedChannelIDs = (editInheritChannels && group)
      ? (group.allowed_channel_ids || []).map(normalizeChannelID).filter((item) => item > 0)
      : editRawAllowedChannelIDs.slice();
    editAllowedModels = (editInheritModels && group)
      ? (group.allowed_models || []).slice()
      : editRawAllowedModels.slice();
    selectedAllowedChannelIDs.clear();
    selectedAllowedModelIndices.clear();
    renderAllowedChannelsTable();
    renderAllowedModelsTable();

    setControlDisabled('[data-action="show-channel-select-modal"], [data-action="batch-delete-allowed-channels"], #selectAllAllowedChannels', editInheritChannels && hasGroup);
    setControlDisabled('[data-action="show-model-select-modal"], [data-action="show-model-import-modal"], [data-action="batch-delete-allowed-models"], #selectAllAllowedModels', editInheritModels && hasGroup);
  }

  function sortAllowedChannelIDs() {
    editAllowedChannelIDs.sort((a, b) => {
      const nameA = getChannelDisplayName(a).toLowerCase();
      const nameB = getChannelDisplayName(b).toLowerCase();
      if (nameA < nameB) return -1;
      if (nameA > nameB) return 1;
      return a - b;
    });
  }

  function renderAllowedChannelsTable() {
    const tbody = document.getElementById('allowedChannelsTableBody');
    const countSpan = document.getElementById('editAllowedChannelsCount');
    const selectAllCheckbox = document.getElementById('selectAllAllowedChannels');
    if (!tbody) return;

    if (countSpan) countSpan.textContent = editAllowedChannelIDs.length;
    updateBatchDeleteChannelsBtn();
    if (selectAllCheckbox) {
      selectAllCheckbox.disabled = !!editInheritChannels;
      selectAllCheckbox.checked = editAllowedChannelIDs.length > 0 && selectedAllowedChannelIDs.size === editAllowedChannelIDs.length;
    }

    if (editAllowedChannelIDs.length === 0) {
      tbody.innerHTML = `<tr class="allowed-channels-empty-row"><td colspan="4" class="allowed-channels-empty-cell">${html(tr('tokens.noChannelRestriction'))}</td></tr>`;
      return;
    }

    tbody.innerHTML = editAllowedChannelIDs.map((channelID) => `
      <tr class="mobile-inline-row allowed-channel-row">
        <td class="allowed-channel-col-select mobile-inline-no-label">
          <input type="checkbox" class="allowed-channel-checkbox" data-channel-id="${channelID}" data-change-action="toggle-allowed-channel" ${editInheritChannels ? 'disabled' : ''} ${selectedAllowedChannelIDs.has(channelID) ? 'checked' : ''}>
        </td>
        <td class="allowed-channel-col-name" data-mobile-label="${html(tr('tokens.channelName'))}">${html(getChannelDisplayName(channelID))}</td>
        <td class="allowed-channel-col-type" data-mobile-label="${html(tr('tokens.channelType'))}">${html(getChannelTypeText(channelID))}</td>
        <td class="allowed-channel-col-actions" data-mobile-label="${html(tr('tokens.table.actions'))}">
          <button type="button" class="allowed-channel-remove-btn btn btn-secondary btn-sm" data-action="remove-allowed-channel" data-channel-id="${channelID}" ${editInheritChannels ? 'disabled' : ''}>${html(tr('common.delete'))}</button>
        </td>
      </tr>
    `).join('');
  }

  function toggleAllowedChannelSelection(channelID, checked) {
    const id = normalizeChannelID(channelID);
    if (!id) return;
    if (checked) selectedAllowedChannelIDs.add(id);
    else selectedAllowedChannelIDs.delete(id);
    updateBatchDeleteChannelsBtn();
    updateSelectAllAllowedChannelsCheckbox();
  }

  function toggleSelectAllAllowedChannels(checked) {
    if (checked) editAllowedChannelIDs.forEach((id) => selectedAllowedChannelIDs.add(id));
    else selectedAllowedChannelIDs.clear();
    renderAllowedChannelsTable();
  }

  function updateBatchDeleteChannelsBtn() {
    const btn = document.getElementById('batchDeleteAllowedChannelsBtn');
    if (btn) btn.disabled = editInheritChannels || selectedAllowedChannelIDs.size === 0;
  }

  function updateSelectAllAllowedChannelsCheckbox() {
    const checkbox = document.getElementById('selectAllAllowedChannels');
    if (!checkbox) return;
    checkbox.disabled = !!editInheritChannels;
    checkbox.checked = editAllowedChannelIDs.length > 0 && selectedAllowedChannelIDs.size === editAllowedChannelIDs.length;
  }

  function removeAllowedChannel(channelID) {
    if (editInheritChannels) return;
    const id = normalizeChannelID(channelID);
    editAllowedChannelIDs = editAllowedChannelIDs.filter((item) => item !== id);
    editRawAllowedChannelIDs = editAllowedChannelIDs.slice();
    selectedAllowedChannelIDs.delete(id);
    renderAllowedChannelsTable();
  }

  function batchDeleteSelectedAllowedChannels() {
    if (editInheritChannels || selectedAllowedChannelIDs.size === 0) return;
    editAllowedChannelIDs = editAllowedChannelIDs.filter((id) => !selectedAllowedChannelIDs.has(id));
    editRawAllowedChannelIDs = editAllowedChannelIDs.slice();
    selectedAllowedChannelIDs.clear();
    renderAllowedChannelsTable();
  }

  async function showChannelSelectModal() {
    if (allChannels.length === 0) await loadChannelsData();
    selectedChannelsForAdd.clear();
    const searchInput = document.getElementById('channelSearchInput');
    if (searchInput) searchInput.value = '';
    const filter = document.getElementById('channelTypeFilterSelect');
    if (filter) filter.value = '';
    renderAvailableChannels('');
    document.getElementById('channelSelectModal').style.display = 'block';
  }

  function closeChannelSelectModal() {
    const modal = document.getElementById('channelSelectModal');
    if (modal) modal.style.display = 'none';
    selectedChannelsForAdd.clear();
  }

  function filterAvailableChannels(searchText) {
    renderAvailableChannels(searchText);
  }

  function getChannelTypeKey(channel) {
    return String(channel?.channel_type || 'anthropic').trim().toLowerCase() || 'anthropic';
  }

  function updateChannelTypeFilterOptions(channels) {
    const select = document.getElementById('channelTypeFilterSelect');
    if (!select) return '';
    const current = select.value || '';
    const types = Array.from(new Set(channels.map(getChannelTypeKey))).sort();
    select.innerHTML = [`<option value="">${html(tr('tokens.channelTypeAll'))}</option>`]
      .concat(types.map((type) => `<option value="${html(type)}">${html(type)}</option>`))
      .join('');
    select.value = types.includes(current) ? current : '';
    return select.value;
  }

  function renderAvailableChannels(searchText) {
    const container = document.getElementById('availableChannelsContainer');
    const countSpan = document.getElementById('selectedChannelsCount');
    const selectAllContainer = document.getElementById('selectAllChannelsContainer');
    const selectAllCheckbox = document.getElementById('selectAllChannelsCheckbox');
    const visibleChannelsCount = document.getElementById('visibleChannelsCount');
    if (!container) return;

    const existing = new Set(editAllowedChannelIDs);
    const available = allChannels.filter((channel) => !existing.has(normalizeChannelID(channel.id)));
    const selectedType = updateChannelTypeFilterOptions(available);
    const search = String(searchText || '').trim().toLowerCase();
    let channels = available.filter((channel) => {
      if (selectedType && getChannelTypeKey(channel) !== selectedType) return false;
      if (!search) return true;
      return String(channel.name || '').toLowerCase().includes(search)
        || String(channel.id || '').includes(search)
        || getChannelTypeKey(channel).includes(search);
    });

    currentVisibleChannels = channels;
    if (countSpan) countSpan.textContent = selectedChannelsForAdd.size;

    if (channels.length === 0) {
      container.innerHTML = `<div class="available-channels-empty">${html(search || selectedType ? tr('tokens.noMatchingChannel') : tr('tokens.allChannelsAdded'))}</div>`;
      if (selectAllContainer) selectAllContainer.style.display = 'none';
      return;
    }

    if (selectAllContainer) selectAllContainer.style.display = 'block';
    if (selectAllCheckbox) {
      const allSelected = channels.every((channel) => selectedChannelsForAdd.has(normalizeChannelID(channel.id)));
      selectAllCheckbox.checked = allSelected;
      selectAllCheckbox.indeterminate = !allSelected && channels.some((channel) => selectedChannelsForAdd.has(normalizeChannelID(channel.id)));
    }
    if (visibleChannelsCount) visibleChannelsCount.textContent = tr('tokens.visibleChannelsCount', { count: channels.length });

    container.innerHTML = channels.map((channel) => {
      const channelID = normalizeChannelID(channel.id);
      return `
        <label class="channel-option-item" data-channel-id="${channelID}">
          <input type="checkbox" class="channel-option-checkbox" data-channel-id="${channelID}" ${selectedChannelsForAdd.has(channelID) ? 'checked' : ''}>
          <span class="channel-option-label">${html(channel.name || tr('common.unknown'))}</span>
          <span class="channel-option-meta">#${channelID} · ${html(channel.channel_type || '-')}</span>
        </label>
      `;
    }).join('');

    if (!container.dataset.logTokenDelegated) {
      container.addEventListener('change', (event) => {
        const checkbox = event.target.closest('.channel-option-checkbox');
        if (!checkbox) return;
        toggleChannelForAdd(normalizeChannelID(checkbox.dataset.channelId), checkbox.checked);
      });
      container.dataset.logTokenDelegated = '1';
    }
  }

  function toggleChannelForAdd(channelID, checked) {
    const id = normalizeChannelID(channelID);
    if (!id) return;
    if (checked) selectedChannelsForAdd.add(id);
    else selectedChannelsForAdd.delete(id);
    const count = document.getElementById('selectedChannelsCount');
    if (count) count.textContent = selectedChannelsForAdd.size;
    updateSelectAllChannelsCheckboxState();
  }

  function updateSelectAllChannelsCheckboxState() {
    const checkbox = document.getElementById('selectAllChannelsCheckbox');
    if (!checkbox || currentVisibleChannels.length === 0) return;
    const allSelected = currentVisibleChannels.every((channel) => selectedChannelsForAdd.has(normalizeChannelID(channel.id)));
    checkbox.checked = allSelected;
    checkbox.indeterminate = !allSelected && currentVisibleChannels.some((channel) => selectedChannelsForAdd.has(normalizeChannelID(channel.id)));
  }

  function toggleSelectAllChannels(checked) {
    currentVisibleChannels.forEach((channel) => {
      const id = normalizeChannelID(channel.id);
      if (checked) selectedChannelsForAdd.add(id);
      else selectedChannelsForAdd.delete(id);
    });
    const count = document.getElementById('selectedChannelsCount');
    if (count) count.textContent = selectedChannelsForAdd.size;
    renderAvailableChannels(document.getElementById('channelSearchInput')?.value || '');
  }

  function confirmChannelSelection() {
    if (selectedChannelsForAdd.size === 0) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.selectAtLeastOneChannel'), 'warning');
      return;
    }
    const existing = new Set(editAllowedChannelIDs);
    selectedChannelsForAdd.forEach((id) => {
      if (!existing.has(id)) editAllowedChannelIDs.push(id);
    });
    sortAllowedChannelIDs();
    editRawAllowedChannelIDs = editAllowedChannelIDs.slice();
    const added = selectedChannelsForAdd.size;
    closeChannelSelectModal();
    renderAllowedChannelsTable();
    if (window.showNotification) window.showNotification(tr('tokens.msg.channelsAdded', { count: added }), 'success');
  }

  function renderAllowedModelsTable() {
    const tbody = document.getElementById('allowedModelsTableBody');
    const countSpan = document.getElementById('editAllowedModelsCount');
    const selectAllCheckbox = document.getElementById('selectAllAllowedModels');
    if (!tbody) return;

    if (countSpan) countSpan.textContent = editAllowedModels.length;
    updateBatchDeleteBtn();
    if (selectAllCheckbox) {
      selectAllCheckbox.disabled = !!editInheritModels;
      selectAllCheckbox.checked = editAllowedModels.length > 0 && selectedAllowedModelIndices.size === editAllowedModels.length;
    }

    if (editAllowedModels.length === 0) {
      tbody.innerHTML = `<tr class="allowed-models-empty-row"><td colspan="3" class="allowed-models-empty-cell">${html(tr('tokens.noModelRestriction'))}</td></tr>`;
      return;
    }

    tbody.innerHTML = editAllowedModels.map((model, index) => `
      <tr class="mobile-inline-row allowed-model-row">
        <td class="allowed-model-col-select mobile-inline-no-label">
          <input type="checkbox" class="allowed-model-checkbox" data-index="${index}" data-change-action="toggle-allowed-model" ${editInheritModels ? 'disabled' : ''} ${selectedAllowedModelIndices.has(index) ? 'checked' : ''}>
        </td>
        <td class="allowed-model-col-name" data-mobile-label="${html(tr('tokens.modelName'))}">${html(model)}</td>
        <td class="allowed-model-col-actions" data-mobile-label="${html(tr('tokens.table.actions'))}">
          <button type="button" class="allowed-model-remove-btn btn btn-secondary btn-sm" data-action="remove-allowed-model" data-index="${index}" ${editInheritModels ? 'disabled' : ''}>${html(tr('common.delete'))}</button>
        </td>
      </tr>
    `).join('');
  }

  function toggleAllowedModelSelection(index, checked) {
    if (!Number.isFinite(index) || index < 0) return;
    if (checked) selectedAllowedModelIndices.add(index);
    else selectedAllowedModelIndices.delete(index);
    updateBatchDeleteBtn();
    updateSelectAllCheckbox();
  }

  function toggleSelectAllAllowedModels(checked) {
    if (checked) editAllowedModels.forEach((_, index) => selectedAllowedModelIndices.add(index));
    else selectedAllowedModelIndices.clear();
    renderAllowedModelsTable();
  }

  function updateBatchDeleteBtn() {
    const btn = document.getElementById('batchDeleteAllowedModelsBtn');
    if (btn) btn.disabled = editInheritModels || selectedAllowedModelIndices.size === 0;
  }

  function updateSelectAllCheckbox() {
    const checkbox = document.getElementById('selectAllAllowedModels');
    if (!checkbox) return;
    checkbox.disabled = !!editInheritModels;
    checkbox.checked = editAllowedModels.length > 0 && selectedAllowedModelIndices.size === editAllowedModels.length;
  }

  function removeAllowedModel(index) {
    if (editInheritModels || !Number.isFinite(index) || index < 0) return;
    editAllowedModels.splice(index, 1);
    editRawAllowedModels = editAllowedModels.slice();
    selectedAllowedModelIndices = new Set(Array.from(selectedAllowedModelIndices)
      .filter((item) => item !== index)
      .map((item) => item > index ? item - 1 : item));
    renderAllowedModelsTable();
  }

  function batchDeleteSelectedAllowedModels() {
    if (editInheritModels || selectedAllowedModelIndices.size === 0) return;
    Array.from(selectedAllowedModelIndices).sort((a, b) => b - a).forEach((index) => {
      editAllowedModels.splice(index, 1);
    });
    editRawAllowedModels = editAllowedModels.slice();
    selectedAllowedModelIndices.clear();
    renderAllowedModelsTable();
  }

  async function showModelSelectModal() {
    if (allChannels.length === 0) await loadChannelsData();
    selectedModelsForAdd.clear();
    const input = document.getElementById('modelSearchInput');
    if (input) input.value = '';
    renderAvailableModels('');
    document.getElementById('modelSelectModal').style.display = 'block';
  }

  function closeModelSelectModal() {
    const modal = document.getElementById('modelSelectModal');
    if (modal) modal.style.display = 'none';
    selectedModelsForAdd.clear();
  }

  function filterAvailableModels(searchText) {
    renderAvailableModels(searchText);
  }

  function renderAvailableModels(searchText) {
    const container = document.getElementById('availableModelsContainer');
    const countSpan = document.getElementById('selectedModelsCount');
    const selectAllContainer = document.getElementById('selectAllContainer');
    const selectAllCheckbox = document.getElementById('selectAllModelsCheckbox');
    const visibleModelsCount = document.getElementById('visibleModelsCount');
    if (!container) return;

    const existing = new Set(editAllowedModels.map((model) => model.toLowerCase()));
    const source = getAvailableModelsForCurrentChannelRestriction();
    const search = String(searchText || '').trim().toLowerCase();
    const models = source
      .filter((model) => !existing.has(model.toLowerCase()))
      .filter((model) => !search || model.toLowerCase().includes(search));

    currentVisibleModels = models;
    if (countSpan) countSpan.textContent = selectedModelsForAdd.size;

    if (models.length === 0) {
      container.innerHTML = `<div class="available-models-empty">${html(search ? tr('tokens.noMatchingModel') : tr('tokens.allModelsAdded'))}</div>`;
      if (selectAllContainer) selectAllContainer.style.display = 'none';
      return;
    }

    if (selectAllContainer) selectAllContainer.style.display = 'block';
    if (selectAllCheckbox) {
      const allSelected = models.every((model) => selectedModelsForAdd.has(model));
      selectAllCheckbox.checked = allSelected;
      selectAllCheckbox.indeterminate = !allSelected && models.some((model) => selectedModelsForAdd.has(model));
    }
    if (visibleModelsCount) visibleModelsCount.textContent = tr('tokens.visibleModelsCount', { count: models.length });

    container.innerHTML = models.map((model) => `
      <label class="model-option-item" data-model="${html(model)}">
        <input type="checkbox" class="model-option-checkbox" data-model="${html(model)}" ${selectedModelsForAdd.has(model) ? 'checked' : ''}>
        <span class="model-option-label">${html(model)}</span>
      </label>
    `).join('');

    if (!container.dataset.logTokenDelegated) {
      container.addEventListener('change', (event) => {
        const checkbox = event.target.closest('.model-option-checkbox');
        if (!checkbox) return;
        toggleModelForAdd(checkbox.dataset.model || '', checkbox.checked);
      });
      container.dataset.logTokenDelegated = '1';
    }
  }

  function toggleModelForAdd(model, checked) {
    const value = String(model || '').trim();
    if (!value) return;
    if (checked) selectedModelsForAdd.add(value);
    else selectedModelsForAdd.delete(value);
    const count = document.getElementById('selectedModelsCount');
    if (count) count.textContent = selectedModelsForAdd.size;
    updateSelectAllCheckboxState();
  }

  function updateSelectAllCheckboxState() {
    const checkbox = document.getElementById('selectAllModelsCheckbox');
    if (!checkbox || currentVisibleModels.length === 0) return;
    const allSelected = currentVisibleModels.every((model) => selectedModelsForAdd.has(model));
    checkbox.checked = allSelected;
    checkbox.indeterminate = !allSelected && currentVisibleModels.some((model) => selectedModelsForAdd.has(model));
  }

  function toggleSelectAllModels(checked) {
    currentVisibleModels.forEach((model) => {
      if (checked) selectedModelsForAdd.add(model);
      else selectedModelsForAdd.delete(model);
    });
    const count = document.getElementById('selectedModelsCount');
    if (count) count.textContent = selectedModelsForAdd.size;
    renderAvailableModels(document.getElementById('modelSearchInput')?.value || '');
  }

  function confirmModelSelection() {
    if (selectedModelsForAdd.size === 0) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.selectAtLeastOne'), 'warning');
      return;
    }
    selectedModelsForAdd.forEach((model) => {
      if (!editAllowedModels.includes(model)) editAllowedModels.push(model);
    });
    editAllowedModels.sort();
    editRawAllowedModels = editAllowedModels.slice();
    const added = selectedModelsForAdd.size;
    closeModelSelectModal();
    renderAllowedModelsTable();
    if (window.showNotification) window.showNotification(tr('tokens.msg.modelsAdded', { count: added }), 'success');
  }

  function parseModelInput(input) {
    return String(input || '').split(/[,\n]/).map((item) => item.trim()).filter(Boolean);
  }

  function showModelImportPrompt() {
    if (editInheritModels) return;
    const input = window.prompt(tr('tokens.inputModelLabel'));
    if (!input) return;
    const models = parseModelInput(input);
    const existing = new Set(editAllowedModels.map((model) => model.toLowerCase()));
    const newModels = Array.from(new Set(models)).filter((model) => !existing.has(model.toLowerCase()));
    if (newModels.length === 0) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.allModelsExist'), 'info');
      return;
    }
    newModels.forEach((model) => editAllowedModels.push(model));
    editAllowedModels.sort();
    editRawAllowedModels = editAllowedModels.slice();
    renderAllowedModelsTable();
    if (window.showNotification) window.showNotification(tr('tokens.msg.importSuccess', { count: newModels.length }), 'success');
  }

  function copyEditToken() {
    const input = document.getElementById('editTokenValue');
    if (!input || !input.value) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.noPlainToken'), 'warning');
      return;
    }
    if (typeof window.copyToClipboard === 'function') {
      window.copyToClipboard(input.value).then(() => {
        if (window.showNotification) window.showNotification(tr('tokens.msg.copySuccess'), 'success');
      });
    }
  }

  async function refreshLogsTokenFilterAndList() {
    try {
      if (typeof window.loadAuthTokensIntoSelect === 'function') {
        const currentValue = document.getElementById('f_auth_token')?.value || '';
        const tokens = await window.loadAuthTokensIntoSelect('f_auth_token', { restoreValue: currentValue });
        if (typeof authTokens !== 'undefined') {
          authTokens = tokens;
        }
      }
    } catch (error) {
      console.warn('Failed to refresh logs auth token filter after save', error);
    }

    if (typeof load === 'function') {
      await load(true);
    }
  }

  async function updateToken() {
    const id = document.getElementById('editTokenId').value;
    const plainToken = document.getElementById('editTokenValue').value.trim();
    const description = document.getElementById('editTokenDescription').value.trim();
    const isActive = document.getElementById('editTokenActive').checked;
    const codexGuardEnabled = !!document.getElementById('editCodexGuardEnabled')?.checked;
    const expiryType = document.getElementById('editTokenExpiry').value;

    if (!editInheritQuota || !editInheritChannels || !editInheritModels) {
      captureRawEditValues();
    }

    const groupID = Number(document.getElementById('editTokenGroup')?.value) || 0;
    const costLimitUSD = editInheritQuota ? editRawCostLimitUSD : (parseFloat(document.getElementById('editCostLimitUSD').value) || 0);
    const dailyCostLimitUSD = editInheritQuota ? editRawDailyCostLimitUSD : (parseFloat(document.getElementById('editDailyCostLimitUSD').value) || 0);
    const dailyLimitDoubleEnabled = !!document.getElementById('editDailyLimitDoubleEnabled')?.checked;
    const dailyLimitTripleEnabled = !!document.getElementById('editDailyLimitTripleEnabled')?.checked;
    const maxConcurrencyResult = parseMaxConcurrencyInput(document.getElementById('editMaxConcurrency').value);
    if (editInheritQuota) {
      maxConcurrencyResult.value = editRawMaxConcurrency || 0;
      delete maxConcurrencyResult.error;
    }

    if (costLimitUSD < 0) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.costLimitNegative'), 'error');
      return;
    }
    if (dailyCostLimitUSD < 0) {
      if (window.showNotification) window.showNotification(tr('tokens.msg.dailyCostLimitNegative'), 'error');
      return;
    }
    if (maxConcurrencyResult.error) {
      if (window.showNotification) window.showNotification(maxConcurrencyResult.error, 'error');
      return;
    }

    let expiresAt = null;
    if (expiryType !== 'never') {
      if (expiryType === 'custom') {
        const customDate = document.getElementById('editCustomExpiry').value;
        if (!customDate) {
          if (window.showNotification) window.showNotification(tr('tokens.msg.selectExpiry'), 'error');
          return;
        }
        expiresAt = new Date(customDate).getTime();
      } else {
        const days = parseInt(expiryType, 10);
        if (!Number.isFinite(days) || days <= 0) {
          if (window.showNotification) window.showNotification(tr('tokens.msg.selectExpiry'), 'error');
          return;
        }
        expiresAt = Date.now() + days * 24 * 60 * 60 * 1000;
      }
    }

    try {
      const savedToken = await fetchDataWithAuth(`/admin/auth-tokens/${encodeURIComponent(id)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          plain_token: plainToken,
          description,
          is_active: isActive,
          codex_guard_enabled: codexGuardEnabled,
          expires_at: expiresAt,
          group_id: groupID,
          inherit_quota: groupID > 0 && editInheritQuota,
          inherit_channels: groupID > 0 && editInheritChannels,
          inherit_models: groupID > 0 && editInheritModels,
          allowed_channel_ids: editInheritChannels ? editRawAllowedChannelIDs : editAllowedChannelIDs,
          allowed_models: editInheritModels ? editRawAllowedModels : editAllowedModels,
          cost_limit_usd: costLimitUSD,
          daily_cost_limit_usd: dailyCostLimitUSD,
          daily_limit_double_enabled: dailyLimitDoubleEnabled,
          daily_limit_triple_enabled: dailyLimitTripleEnabled,
          max_concurrency: maxConcurrencyResult.value
        })
      });

      if (savedToken) {
        const idx = editorTokens.findIndex((token) => Number(token.id) === Number(savedToken.id));
        if (idx >= 0) editorTokens[idx] = { ...editorTokens[idx], ...savedToken };
        else editorTokens.push(savedToken);
      }

      closeEditModal();
      await refreshLogsTokenFilterAndList();
      if (window.showNotification) window.showNotification(tr('tokens.msg.updateSuccess'), 'success');
    } catch (error) {
      console.error('Failed to update token:', error);
      if (window.showNotification) window.showNotification(`${tr('tokens.msg.updateFailed')}: ${error.message}`, 'error');
    }
  }

  window.openLogTokenEditor = openLogTokenEditor;
})();
