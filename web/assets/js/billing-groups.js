(function () {
  'use strict';

  const API_BASE = '/admin';
  let billingGroups = [];
  let channels = [];
  let channelKeys = new Map();
  let selectedKeys = new Map();
  let loading = false;
  let saving = false;

  function text(key, fallback, params) {
    if (typeof window.i18nText === 'function') {
      return window.i18nText(key, fallback, params);
    }
    const value = typeof window.t === 'function' ? window.t(key, params) : '';
    return value && value !== key ? value : fallback;
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, (char) => ({
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;'
    }[char]));
  }

  function normalizeKeyIndex(value) {
    const index = Number(value);
    return Number.isInteger(index) && index >= 0 ? index : -1;
  }

  function normalizeChannelID(value) {
    const id = Number(value);
    return Number.isInteger(id) && id > 0 ? id : 0;
  }

  function getChannelKeys(channelID) {
    return channelKeys.get(normalizeChannelID(channelID)) || [];
  }

  function getChannelName(channel) {
    return String(channel?.name || `#${channel?.id || ''}`).trim();
  }

  function getKeyLabel(key) {
    const index = normalizeKeyIndex(key?.key_index);
    const fallback = `Key ${index + 1}`;
    const alias = String(key?.alias || '').trim();
    return alias ? `${alias}（Key ${index + 1}）` : fallback;
  }

  function showNotice(message, type = 'info') {
    if (typeof window.showNotification === 'function') window.showNotification(message, type);
  }

  function readForm() {
    return {
      id: Number(document.getElementById('billingGroupID')?.value) || 0,
      name: String(document.getElementById('billingGroupName')?.value || '').trim(),
      slug: String(document.getElementById('billingGroupSlug')?.value || '').trim(),
      description: String(document.getElementById('billingGroupDescription')?.value || '').trim(),
      multiplier: Number(document.getElementById('billingGroupMultiplier')?.value),
      enabled: !!document.getElementById('billingGroupEnabled')?.checked
    };
  }

  function writeForm(group) {
    const value = group || {};
    document.getElementById('billingGroupID').value = String(Number(value.id) || 0);
    document.getElementById('billingGroupName').value = value.name || '';
    document.getElementById('billingGroupSlug').value = value.slug || '';
    document.getElementById('billingGroupDescription').value = value.description || '';
    document.getElementById('billingGroupMultiplier').value = String(value.multiplier ?? 1);
    document.getElementById('billingGroupEnabled').checked = value.enabled !== false;
    const title = document.getElementById('billingGroupFormTitle');
    if (title) title.textContent = value.id ? text('billingGroups.editGroup', '编辑计费分组') : text('billingGroups.newGroup', '新建计费分组');
  }

  function normalizeSelectedKeys(group) {
    const result = new Map();
    if (group?.channel_keys && typeof group.channel_keys === 'object') {
      Object.entries(group.channel_keys).forEach(([rawID, indexes]) => {
        const channelID = normalizeChannelID(rawID);
        if (!channelID || !Array.isArray(indexes)) return;
        const normalized = new Set(indexes.map(normalizeKeyIndex).filter((index) => index >= 0));
        if (normalized.size) result.set(channelID, normalized);
      });
      return result;
    }
    (group?.channel_ids || []).map(normalizeChannelID).filter(Boolean).forEach((channelID) => {
      const indexes = getChannelKeys(channelID).map((key) => normalizeKeyIndex(key.key_index)).filter((index) => index >= 0);
      if (indexes.length) result.set(channelID, new Set(indexes));
    });
    return result;
  }

  function selectedCount() {
    let count = 0;
    selectedKeys.forEach((indexes) => { count += indexes.size; });
    return count;
  }

  function updateSelectionSummary() {
    const summary = document.getElementById('billingGroupSelectionSummary');
    if (!summary) return;
    const channelCount = Array.from(selectedKeys.values()).filter((indexes) => indexes.size > 0).length;
    summary.textContent = text(
      'billingGroups.selectionSummary',
      '{channels} 个渠道 · {keys} 个 Key',
      { channels: channelCount, keys: selectedCount() }
    );
  }

  function isKeySelected(channelID, keyIndex) {
    return selectedKeys.get(channelID)?.has(keyIndex) || false;
  }

  function renderChannelOptions() {
    const container = document.getElementById('billingGroupChannelOptions');
    if (!container) return;
    const search = String(document.getElementById('billingGroupKeySearch')?.value || '').trim().toLowerCase();
    const visibleSections = [];
    channels.forEach((channel) => {
      const channelID = normalizeChannelID(channel.id);
      if (!channelID) return;
      const keys = getChannelKeys(channelID);
      const matchingKeys = keys.filter((key) => {
        if (!search) return true;
        const haystack = `${getChannelName(channel)} ${key.alias || ''} ${key.note || ''} Key ${Number(key.key_index) + 1}`.toLowerCase();
        return haystack.includes(search);
      });
      if (search && matchingKeys.length === 0) return;
      const selected = selectedKeys.get(channelID) || new Set();
      const allSelected = keys.length > 0 && keys.every((key) => selected.has(normalizeKeyIndex(key.key_index)));
      const keyHTML = matchingKeys.length > 0
        ? matchingKeys.map((key) => {
          const keyIndex = normalizeKeyIndex(key.key_index);
          const label = getKeyLabel(key);
          const note = String(key.note || '').trim();
          return `<label class="billing-groups-key-option" title="${escapeHTML(note || label)}"><input type="checkbox" class="control-checkbox" data-change-action="toggle-billing-group-key" data-channel-id="${channelID}" data-key-index="${keyIndex}"${isKeySelected(channelID, keyIndex) ? ' checked' : ''}><span>${escapeHTML(label)}</span></label>`;
        }).join('')
        : '<span class="token-limit-hint">暂无 Key</span>';
      visibleSections.push(`<section class="billing-groups-channel-section" data-channel-id="${channelID}">
        <div class="billing-groups-channel-section__header">
          <label><input type="checkbox" class="control-checkbox" data-change-action="toggle-billing-group-channel" data-channel-id="${channelID}"${allSelected ? ' checked' : ''}${keys.length ? '' : ' disabled'}><span class="billing-groups-channel-section__name">${escapeHTML(getChannelName(channel))}</span></label>
          <span class="billing-groups-channel-section__id">${escapeHTML(text('billingGroups.channelMeta', '#{id} · {keys} Key', { id: channelID, keys: keys.length }))}</span>
        </div>
        <div class="billing-groups-channel-section__keys">${keyHTML}</div>
      </section>`);
    });
    container.innerHTML = visibleSections.join('') || `<div class="billing-groups-empty">${escapeHTML(search ? text('billingGroups.noMatchingKeys', '没有匹配的渠道或 Key') : text('billingGroups.noKeys', '暂无可用渠道 Key'))}</div>`;
    updateSelectionSummary();
  }

  function toggleKey(channelID, keyIndex, checked) {
    const id = normalizeChannelID(channelID);
    const index = normalizeKeyIndex(keyIndex);
    if (!id || index < 0) return;
    const indexes = selectedKeys.get(id) || new Set();
    if (checked) indexes.add(index);
    else indexes.delete(index);
    if (indexes.size) selectedKeys.set(id, indexes);
    else selectedKeys.delete(id);
    renderChannelOptions();
  }

  function toggleChannel(channelID, checked) {
    const id = normalizeChannelID(channelID);
    const indexes = getChannelKeys(id).map((key) => normalizeKeyIndex(key.key_index)).filter((index) => index >= 0);
    if (checked && indexes.length) selectedKeys.set(id, new Set(indexes));
    else selectedKeys.delete(id);
    renderChannelOptions();
  }

  function buildChannelPayload() {
    const channelKeysPayload = {};
    selectedKeys.forEach((indexes, channelID) => {
      const values = Array.from(indexes).filter((index) => Number.isInteger(index)).sort((a, b) => a - b);
      if (values.length) channelKeysPayload[channelID] = values;
    });
    return {
      channel_ids: Object.keys(channelKeysPayload).map(Number),
      channel_keys: channelKeysPayload
    };
  }

  function renderGroups() {
    const container = document.getElementById('billingGroupList');
    const count = document.getElementById('billingGroupsCount');
    if (!container) return;
    if (count) count.textContent = String(billingGroups.length);
    if (billingGroups.length === 0) {
      container.innerHTML = `<div class="billing-groups-empty">${escapeHTML(text('billingGroups.noGroups', '暂无计费分组'))}</div>`;
      return;
    }
    container.innerHTML = billingGroups.map((group) => {
      const keyCount = Object.values(group.channel_keys || {}).reduce((total, indexes) => total + (Array.isArray(indexes) ? indexes.length : 0), 0);
      const statusClass = group.enabled ? '' : ' billing-groups-list-item__status--disabled';
      const statusText = group.enabled ? text('common.enabled', '已启用') : text('common.disabled', '已停用');
      return `<div class="billing-groups-list-item">
        <div>
          <div class="billing-groups-list-item__name"><span>${escapeHTML(group.name || group.slug || `#${group.id}`)}</span><code class="billing-groups-list-item__slug">${escapeHTML(group.slug || '')}</code><span class="billing-groups-list-item__status${statusClass}">${escapeHTML(statusText)}</span></div>
          <div class="billing-groups-list-item__meta">${escapeHTML(text('billingGroups.listMeta', '× {multiplier} · {channels} 个渠道 · {keys} 个 Key', {
            multiplier: Number(group.multiplier || 0).toFixed(4),
            channels: (group.channel_ids || []).length,
            keys: keyCount
          }))}</div>
          <div class="billing-groups-list-item__description">${escapeHTML(group.description || '')}</div>
        </div>
        <div class="billing-groups-list-item__actions"><button type="button" class="btn btn-secondary btn-sm" data-action="edit-billing-group" data-group-id="${Number(group.id)}">${escapeHTML(text('common.edit', '编辑'))}</button><button type="button" class="btn btn-danger btn-sm" data-action="delete-billing-group" data-group-id="${Number(group.id)}">${escapeHTML(text('common.delete', '删除'))}</button></div>
      </div>`;
    }).join('');
  }

  async function loadChannelKeys() {
    const entries = await Promise.all(channels.map(async (channel) => {
      const channelID = normalizeChannelID(channel.id);
      if (!channelID) return [0, []];
      try {
        const data = await window.fetchDataWithAuth(`${API_BASE}/channels/${channelID}/keys`);
        const keys = Array.isArray(data) ? data : (data?.keys || []);
        return [channelID, keys.map((key) => ({
          key_index: normalizeKeyIndex(key?.key_index),
          alias: String(key?.alias || '').trim(),
          note: String(key?.note || '').trim()
        })).filter((key) => key.key_index >= 0)];
      } catch (_) {
        const fallback = Array.isArray(channel.key_cooldowns) ? channel.key_cooldowns : [];
        return [channelID, fallback.map((key) => ({
          key_index: normalizeKeyIndex(key?.key_index),
          alias: String(key?.alias || '').trim(),
          note: String(key?.note || '').trim()
        })).filter((key) => key.key_index >= 0)];
      }
    }));
    channelKeys = new Map(entries.filter(([id]) => id > 0));
  }

  async function loadData() {
    if (loading) return;
    loading = true;
    const container = document.getElementById('billingGroupChannelOptions');
    if (container) container.innerHTML = `<div class="billing-groups-loading">${escapeHTML(text('common.loading', '加载中...'))}</div>`;
    try {
      const [groupData, channelData] = await Promise.all([
        window.fetchDataWithAuth(`${API_BASE}/billing-groups`),
        window.fetchDataWithAuth(`${API_BASE}/channels`)
      ]);
      billingGroups = Array.isArray(groupData?.groups) ? groupData.groups : [];
      channels = Array.isArray(channelData) ? channelData : (channelData?.channels || channelData?.data || []);
      await loadChannelKeys();
      renderGroups();
      renderChannelOptions();
    } catch (error) {
      console.error('Failed to load billing groups page:', error);
      showNotice(`${text('billingGroups.loadFailed', '加载计费分组失败')}: ${error.message}`, 'error');
      renderGroups();
      renderChannelOptions();
    } finally {
      loading = false;
    }
  }

  function resetForm() {
    writeForm(null);
    selectedKeys = new Map();
    renderChannelOptions();
  }

  function editGroup(id) {
    const group = billingGroups.find((item) => Number(item.id) === Number(id));
    if (!group) return;
    writeForm(group);
    selectedKeys = normalizeSelectedKeys(group);
    renderChannelOptions();
    document.getElementById('billingGroupName')?.focus();
  }

  async function saveGroup() {
    if (saving) return;
    const form = readForm();
    if (!form.name || !form.slug || !Number.isFinite(form.multiplier) || form.multiplier < 0) {
      showNotice(text('billingGroups.invalidForm', '请填写有效的名称、标识和倍率'), 'error');
      return;
    }
    saving = true;
    try {
      const payload = { ...form, ...buildChannelPayload() };
      delete payload.id;
      await window.fetchDataWithAuth(`${API_BASE}/billing-groups${form.id ? `/${form.id}` : ''}`, {
        method: form.id ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      await loadData();
      resetForm();
      showNotice(text('billingGroups.saved', '计费分组已保存'), 'success');
    } catch (error) {
      showNotice(`${text('billingGroups.saveFailed', '保存失败')}: ${error.message}`, 'error');
    } finally {
      saving = false;
    }
  }

  async function deleteGroup(id) {
    if (!window.confirm(text('billingGroups.deleteConfirm', '确定删除这个计费分组？'))) return;
    try {
      await window.fetchDataWithAuth(`${API_BASE}/billing-groups/${id}`, { method: 'DELETE' });
      await loadData();
      resetForm();
      showNotice(text('billingGroups.deleted', '计费分组已删除'), 'success');
    } catch (error) {
      showNotice(`${text('billingGroups.deleteFailed', '删除失败')}: ${error.message}`, 'error');
    }
  }

  function bindActions() {
    if (typeof window.initDelegatedActions !== 'function') return;
    window.initDelegatedActions({
      boundKey: 'billingGroupsPageActionsBound',
      click: {
        'refresh-billing-groups': () => loadData(),
        'reset-billing-group-form': () => resetForm(),
        'save-billing-group': () => saveGroup(),
        'edit-billing-group': (target) => editGroup(Number(target.dataset.groupId)),
        'delete-billing-group': (target) => deleteGroup(Number(target.dataset.groupId))
      },
      change: {
        'toggle-billing-group-key': (target) => toggleKey(target.dataset.channelId, target.dataset.keyIndex, target.checked),
        'toggle-billing-group-channel': (target) => toggleChannel(target.dataset.channelId, target.checked)
      },
      input: {
        'filter-billing-group-keys': () => renderChannelOptions()
      }
    });
  }

  window.initPageBootstrap({
    topbarKey: 'billing-groups',
    run: async () => {
      bindActions();
      resetForm();
      await loadData();
    }
  });
})();
