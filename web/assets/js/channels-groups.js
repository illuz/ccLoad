const CHANNEL_GROUP_COLOR_PRESETS = Object.freeze([
  '#64748b', '#ef4444', '#f97316', '#f59e0b', '#22c55e', '#14b8a6', '#3b82f6', '#8b5cf6'
]);

function normalizeChannelGroupColor(value) {
  return String(value || '').trim().toLowerCase();
}

function getDefaultChannelGroupColor() {
  return CHANNEL_GROUP_COLOR_PRESETS[0];
}

function getChannelGroupColor(value) {
  const color = normalizeChannelGroupColor(value);
  return CHANNEL_GROUP_COLOR_PRESETS.includes(color) ? color : getDefaultChannelGroupColor();
}

function sortChannelGroups(groups = []) {
  return groups.sort((a, b) => {
    const nameCmp = String(a?.name || '').localeCompare(String(b?.name || ''));
    if (nameCmp !== 0) return nameCmp;
    return Number(a?.id || 0) - Number(b?.id || 0);
  });
}

function getChannelGroupByID(groupID) {
  const id = Number(groupID) || 0;
  if (id <= 0) return null;
  return channelGroups.find((group) => Number(group?.id || 0) === id) || null;
}

function getChannelGroupNameByID(groupID) {
  const group = getChannelGroupByID(groupID);
  if (!group) return '';
  return group.name || `${window.t('channels.group')} #${group.id}`;
}

function getChannelGroupColorByID(groupID) {
  const group = getChannelGroupByID(groupID);
  return group ? getChannelGroupColor(group.color) : getDefaultChannelGroupColor();
}

async function loadChannelGroups() {
  try {
    const resp = await fetchDataWithAuth('/admin/channel-groups');
    channelGroups = sortChannelGroups((resp && resp.groups) || []);
  } catch (error) {
    console.error('Failed to load channel groups:', error);
    channelGroups = [];
  }
  refreshChannelGroupOptions();
  return channelGroups;
}

function renderGroupOptions(select, options = {}) {
  if (!select) return;
  const current = options.selected !== undefined ? String(options.selected) : String(select.value || '');
  const previousScroll = select.scrollTop || 0;
  const includeAll = options.includeAll === true;
  const emptyLabel = options.emptyLabel || window.t('channels.ungrouped');
  const parts = [];
  if (includeAll) parts.push(`<option value="all">${window.escapeHtml(window.t('channels.groupAll'))}</option>`);
  parts.push(`<option value="0">${window.escapeHtml(emptyLabel)}</option>`);
  for (const group of channelGroups) {
    parts.push(`<option value="${window.escapeHtml(group.id)}">${window.escapeHtml(group.name || ('#' + group.id))}</option>`);
  }
  select.innerHTML = parts.join('');
  if (current && Array.from(select.options).some((opt) => opt.value === current)) select.value = current;
  else select.value = includeAll ? 'all' : '0';
  if (previousScroll) select.scrollTop = previousScroll;
}

function refreshChannelGroupOptions() {
  renderGroupOptions(document.getElementById('channelGroupFilter'), { includeAll: true, selected: filters.group || 'all' });
  renderGroupOptions(document.getElementById('channelGroup'), { selected: document.getElementById('channelGroup')?.value || '0' });
  renderGroupOptions(document.getElementById('batchChannelGroupSelect'), { selected: document.getElementById('batchChannelGroupSelect')?.value || '0' });
  renderGroupOptions(document.getElementById('quickAddChannelGroup'), { selected: document.getElementById('quickAddChannelGroup')?.value || '0' });
}

function setChannelViewMode(mode) {
  channelViewMode = mode === 'group' ? 'group' : 'list';
  localStorage.setItem('channels.viewMode', channelViewMode);
  updateChannelViewButtons();
  renderChannels(Array.isArray(filteredChannels) ? filteredChannels : channels);
}

function updateChannelViewButtons() {
  const groupBtn = document.getElementById('channelGroupViewBtn');
  const listBtn = document.getElementById('channelListViewBtn');
  if (groupBtn) groupBtn.classList.toggle('active', channelViewMode === 'group');
  if (listBtn) listBtn.classList.toggle('active', channelViewMode !== 'group');
}

function buildChannelGroupsForView(list = channels) {
  const map = new Map();
  for (const channel of list || []) {
    const key = channel && channel.group_id ? String(channel.group_id) : '0';
    const group = getChannelGroupByID(key);
    const synthetic = key === '0' ? null : {
      id: key,
      name: channel.group_name || `${window.t('channels.group')} #${key}`,
      color: channel.group_color || getDefaultChannelGroupColor(),
      __synthetic: true
    };
    const displayGroup = group || synthetic;
    if (!map.has(key)) {
      map.set(key, {
        key,
        group: displayGroup,
        name: displayGroup ? displayGroup.name : window.t('channels.ungrouped'),
        color: displayGroup ? getChannelGroupColor(displayGroup.color) : getDefaultChannelGroupColor(),
        channels: []
      });
    }
    map.get(key).channels.push(channel);
  }
  return Array.from(map.values()).sort((a, b) => {
    if (a.key === '0') return 1;
    if (b.key === '0') return -1;
    return a.name.localeCompare(b.name);
  });
}

function toggleChannelGroupCollapsed(groupKey) {
  const key = String(groupKey || '0');
  if (collapsedChannelGroups.has(key)) collapsedChannelGroups.delete(key);
  else collapsedChannelGroups.add(key);
  localStorage.setItem('channels.collapsedGroups', JSON.stringify(Array.from(collapsedChannelGroups)));
  renderChannels(Array.isArray(filteredChannels) ? filteredChannels : channels);
}

function renderGroupedChannels(container, list = channels) {
  const el = container || document.getElementById('channels-container');
  if (!el) return;
  initChannelEventDelegation();
  if (!list || list.length === 0) {
    el.innerHTML = `<div class="glass-card">${window.t('channels.noChannels')}</div>`;
    if (typeof updateBatchChannelSelectionUI === 'function') updateBatchChannelSelectionUI();
    return;
  }

  const wrapper = document.createElement('div');
  wrapper.className = 'channel-grouped-view';
  buildChannelGroupsForView(list).forEach(({ key, group, name, color, channels: groupChannels }) => {
    const collapsed = collapsedChannelGroups.has(key);
    const section = document.createElement('section');
    section.className = `channel-group-section${collapsed ? ' channel-group-section--collapsed' : ''}`;
    const canEdit = group && group.id && !group.__synthetic;
    section.innerHTML = `
      <div class="channel-group-header">
        <button type="button" class="channel-group-toggle" data-action="toggle-channel-group-section" data-group-key="${window.escapeHtml(key)}">
          <span class="channel-group-chevron">${collapsed ? '▶' : '▼'}</span>
          <span class="channel-group-dot" style="background:${window.escapeHtml(color)}"></span>
          <span class="channel-group-name">${window.escapeHtml(name || window.t('channels.ungrouped'))}</span>
          <span class="channel-group-count">${groupChannels.length}</span>
        </button>
        ${canEdit ? `<button type="button" class="btn btn-secondary btn-sm" data-action="edit-channel-group-from-view" data-group-id="${window.escapeHtml(group.id)}">${window.escapeHtml(window.t('common.edit'))}</button>` : ''}
      </div>
      <div class="channel-group-content"></div>
    `;
    if (!collapsed) {
      renderChannelTable(section.querySelector('.channel-group-content'), groupChannels);
    }
    wrapper.appendChild(section);
  });
  el.innerHTML = '';
  el.appendChild(wrapper);
  el.querySelectorAll('.channel-select-checkbox').forEach(cb => {
    cb.checked = selectedChannelIds.has(normalizeSelectedChannelID(cb.dataset.channelId));
  });
  if (typeof updateBatchChannelSelectionUI === 'function') updateBatchChannelSelectionUI();
}

function upsertChannelGroupLocal(group) {
  if (!group) return;
  const id = Number(group.id || 0);
  if (id <= 0) return;
  const idx = channelGroups.findIndex((item) => Number(item.id || 0) === id);
  if (idx >= 0) channelGroups[idx] = { ...channelGroups[idx], ...group };
  else channelGroups.push(group);
  sortChannelGroups(channelGroups);
  refreshChannelGroupOptions();
}

function removeChannelGroupLocal(groupID) {
  const id = Number(groupID) || 0;
  channelGroups = channelGroups.filter((group) => Number(group.id || 0) !== id);
  refreshChannelGroupOptions();
}

function setChannelGroupColorValue(color) {
  const normalized = getChannelGroupColor(color);
  const hidden = document.getElementById('channelGroupColor');
  if (hidden) hidden.value = normalized;
  document.querySelectorAll('#channelGroupColorPicker .channel-group-color-option').forEach((btn) => {
    btn.classList.toggle('active', normalizeChannelGroupColor(btn.dataset.color) === normalized);
  });
}

function renderChannelGroupColorPicker() {
  const picker = document.getElementById('channelGroupColorPicker');
  if (!picker) return;
  picker.innerHTML = CHANNEL_GROUP_COLOR_PRESETS.map((color) => `
    <button type="button" class="channel-group-color-option" data-color="${color}" style="--channel-group-color:${color}" aria-label="${color}"></button>
  `).join('');
  picker.querySelectorAll('.channel-group-color-option').forEach((btn) => {
    btn.addEventListener('click', () => setChannelGroupColorValue(btn.dataset.color));
  });
  setChannelGroupColorValue(document.getElementById('channelGroupColor')?.value || getDefaultChannelGroupColor());
}

function resetChannelGroupForm() {
  const id = document.getElementById('channelGroupEditId');
  const name = document.getElementById('channelGroupName');
  const desc = document.getElementById('channelGroupDescription');
  const count = document.getElementById('channelGroupChannelCount');
  if (id) id.value = '';
  if (name) name.value = '';
  if (desc) desc.value = '';
  if (count) count.textContent = '0';
  setChannelGroupColorValue(getDefaultChannelGroupColor());
}

function editChannelGroupInModal(groupID) {
  const group = getChannelGroupByID(groupID);
  if (!group) return;
  document.getElementById('channelGroupEditId').value = group.id;
  document.getElementById('channelGroupName').value = group.name || '';
  document.getElementById('channelGroupDescription').value = group.description || '';
  document.getElementById('channelGroupChannelCount').textContent = String(group.channel_count || 0);
  setChannelGroupColorValue(group.color);
  renderChannelGroupList(group.id);
}

function renderChannelGroupList(selectedID = Number(document.getElementById('channelGroupEditId')?.value) || 0) {
  const list = document.getElementById('channelGroupList');
  if (!list) return;
  if (!channelGroups.length) {
    list.innerHTML = `<div class="channel-group-empty">${window.escapeHtml(window.t('channels.noGroups'))}</div>`;
    return;
  }
  list.innerHTML = channelGroups.map((group) => `
    <button type="button" class="channel-group-list-item${Number(group.id) === Number(selectedID) ? ' active' : ''}" data-action="select-channel-group" data-group-id="${window.escapeHtml(group.id)}">
      <span class="channel-group-dot" style="background:${window.escapeHtml(getChannelGroupColor(group.color))}"></span>
      <span class="channel-group-list-name">${window.escapeHtml(group.name || ('#' + group.id))}</span>
      <span class="channel-group-list-count">${Number(group.channel_count || 0)}</span>
    </button>
  `).join('');
}

async function showChannelGroupManager() {
  await loadChannelGroups();
  renderChannelGroupColorPicker();
  renderChannelGroupList();
  if (channelGroups.length > 0) editChannelGroupInModal(channelGroups[0].id);
  else resetChannelGroupForm();
  document.getElementById('channelGroupModal')?.classList.add('show');
}

function closeChannelGroupModal() {
  document.getElementById('channelGroupModal')?.classList.remove('show');
}

async function createChannelGroupDraft() {
  try {
    const savedResp = await fetchAPIWithAuth('/admin/channel-groups', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: window.t('channels.untitledGroup'), description: '', color: getDefaultChannelGroupColor() })
    });
    if (!savedResp.success) throw new Error(savedResp.error || window.t('common.failed'));
    const saved = savedResp.data || {};
    upsertChannelGroupLocal({ channel_count: 0, ...saved });
    editChannelGroupInModal(saved.id);
    window.showSuccess && window.showSuccess(window.t('channels.msg.groupCreateSuccess'));
  } catch (error) {
    window.showError && window.showError(window.t('channels.msg.groupSaveFailed') + ': ' + error.message);
  }
}

async function saveChannelGroup() {
  const id = Number(document.getElementById('channelGroupEditId')?.value) || 0;
  const name = (document.getElementById('channelGroupName')?.value || '').trim();
  const description = (document.getElementById('channelGroupDescription')?.value || '').trim();
  const color = getChannelGroupColor(document.getElementById('channelGroupColor')?.value);
  if (!name) {
    window.showError && window.showError(window.t('channels.msg.enterGroupName'));
    return;
  }
  try {
    const savedResp = await fetchAPIWithAuth(`/admin/channel-groups${id ? '/' + id : ''}`, {
      method: id ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, description, color })
    });
    if (!savedResp.success) throw new Error(savedResp.error || window.t('common.failed'));
    const saved = savedResp.data || {};
    upsertChannelGroupLocal({ id: id || saved.id, name, description, color, ...saved });
    editChannelGroupInModal(id || saved.id);
    await reloadChannelsList(filters.channelType || 'all', filters.status || 'all');
    window.showSuccess && window.showSuccess(window.t(id ? 'channels.msg.groupUpdateSuccess' : 'channels.msg.groupCreateSuccess'));
  } catch (error) {
    window.showError && window.showError(window.t('channels.msg.groupSaveFailed') + ': ' + error.message);
  }
}

async function deleteChannelGroup() {
  const id = Number(document.getElementById('channelGroupEditId')?.value) || 0;
  if (!id) return;
  if (!confirm(window.t('channels.msg.deleteGroupConfirm'))) return;
  try {
    const resp = await fetchAPIWithAuth(`/admin/channel-groups/${id}`, { method: 'DELETE' });
    if (!resp.success) throw new Error(resp.error || window.t('common.failed'));
    removeChannelGroupLocal(id);
    resetChannelGroupForm();
    renderChannelGroupList();
    await reloadChannelsList(filters.channelType || 'all', filters.status || 'all');
    window.showSuccess && window.showSuccess(window.t('channels.msg.groupDeleteSuccess'));
  } catch (error) {
    window.showError && window.showError(window.t('channels.msg.groupDeleteFailed') + ': ' + error.message);
  }
}

function showBatchChannelGroupModal() {
  if (getSelectedChannelIDs().length === 0) {
    window.showWarning && window.showWarning(window.t('channels.batchNoSelection'));
    return;
  }
  refreshChannelGroupOptions();
  document.getElementById('batchChannelGroupModal')?.classList.add('show');
}

function closeBatchChannelGroupModal() {
  document.getElementById('batchChannelGroupModal')?.classList.remove('show');
}

async function confirmBatchChannelGroup() {
  const ids = getSelectedChannelIDs();
  const groupID = Number(document.getElementById('batchChannelGroupSelect')?.value) || 0;
  if (ids.length === 0) return;
  try {
    const resp = await fetchAPIWithAuth('/admin/channels/batch-group', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channel_ids: ids, group_id: groupID })
    });
    if (!resp.success) throw new Error(resp.error || window.t('common.failed'));
    closeBatchChannelGroupModal();
    selectedChannelIds.clear();
    await Promise.all([loadChannelGroups(), reloadChannelsList(filters.channelType || 'all', filters.status || 'all')]);
    window.showSuccess && window.showSuccess(window.t('channels.msg.batchGroupSuccess'));
  } catch (error) {
    window.showError && window.showError(window.t('channels.batchOperationFailed', { error: error.message }));
  }
}

function initChannelGroupActions() {
  updateChannelViewButtons();
  if (typeof window.initDelegatedActions === 'function') {
    window.initDelegatedActions({
      boundKey: 'channelGroupActionsBound',
      click: {
        'set-channel-view-group': () => setChannelViewMode('group'),
        'set-channel-view-list': () => setChannelViewMode('list'),
        'show-channel-group-manager': () => showChannelGroupManager(),
        'close-channel-group-modal': () => closeChannelGroupModal(),
        'create-channel-group-draft': () => createChannelGroupDraft(),
        'save-channel-group': () => saveChannelGroup(),
        'delete-channel-group': () => deleteChannelGroup(),
        'select-channel-group': (target) => editChannelGroupInModal(target?.dataset?.groupId),
        'toggle-channel-group-section': (target) => toggleChannelGroupCollapsed(target?.dataset?.groupKey),
        'edit-channel-group-from-view': (target) => showChannelGroupManager().then(() => editChannelGroupInModal(target?.dataset?.groupId)),
        'show-batch-channel-group-modal': () => showBatchChannelGroupModal(),
        'close-batch-channel-group-modal': () => closeBatchChannelGroupModal(),
        'confirm-batch-channel-group': () => confirmBatchChannelGroup()
      }
    });
  }
}

window.loadChannelGroups = loadChannelGroups;
window.refreshChannelGroupOptions = refreshChannelGroupOptions;
window.getChannelGroupNameByID = getChannelGroupNameByID;
window.getChannelGroupColorByID = getChannelGroupColorByID;
window.renderGroupedChannels = renderGroupedChannels;
window.initChannelGroupActions = initChannelGroupActions;
