// 系统设置页面
const t = window.t;

let originalSettings = {}; // 保存原始值用于比较
let debugPreserveTokenOptions = [];

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
