// 快速添加渠道:粘贴 URL + Key(s),选模型来源,可选追加到 auth token 分组。

const QUICK_ADD_TYPE_DEFAULT = 'codex';
const QUICK_ADD_URL_FIELD_NAMES = new Set(['url', 'baseurl', 'apiurl', 'apibase', 'endpoint']);
const QUICK_ADD_KEY_FIELD_NAMES = new Set(['key', 'keys', 'apikey', 'apikeys', 'token', 'tokens', 'secretkey']);

function detectTypeFromURL(url) {
  const host = (function () {
    try { return new URL(url).hostname.toLowerCase(); } catch (_) { return ''; }
  })();
  if (!host) return QUICK_ADD_TYPE_DEFAULT;
  if (host.includes('codex')) return 'codex';
  if (host.includes('claude') || host.includes('anthropic')) return 'anthropic';
  if (host.includes('openai') || host.includes('gpt')) return 'openai';
  if (host.includes('gemini') || host.includes('google')) return 'gemini';
  return QUICK_ADD_TYPE_DEFAULT;
}

function hostnameFromURL(url) {
  try { return new URL(url).hostname; } catch (_) { return ''; }
}

function addUniqueQuickAddValue(values, value) {
  const normalized = String(value || '')
    .trim()
    .replace(/^[`"'“”‘’\s]+/, '')
    .replace(/[`"'“”‘’\s]+$/, '')
    .replace(/^[=:：\s]+/, '')
    .replace(/[,，;；\s]+$/, '');
  if (!normalized || values.includes(normalized)) return;
  values.push(normalized);
}

function addQuickAddURL(values, value) {
  const normalized = String(value || '')
    .trim()
    .replace(/\\\//g, '/')
    .replace(/^[`"'“”‘’\s]+/, '')
    .replace(/[)\]}>,，。.;；\s]+$/, '');
  if (!/^https?:\/\//i.test(normalized)) return;
  addUniqueQuickAddValue(values, normalized);
}

function addQuickAddDelimitedKeys(values, text) {
  String(text || '')
    .split(/[,\n]/)
    .map(part => part.trim())
    .filter(Boolean)
    .forEach(part => {
      const cleaned = part
        .replace(/^[`"'“”‘’\s]+/, '')
        .replace(/[`"'“”‘’\s]+$/, '')
        .replace(/[,，;；\s]+$/, '');
      if (!cleaned || /^https?:\/\//i.test(cleaned)) return;
      addUniqueQuickAddValue(values, cleaned);
    });
}

function collectQuickAddJSONFields(value, urls, keys) {
  if (Array.isArray(value)) {
    value.forEach(item => collectQuickAddJSONFields(item, urls, keys));
    return;
  }
  if (!value || typeof value !== 'object') return;

  for (const [field, fieldValue] of Object.entries(value)) {
    const fieldName = String(field || '').toLowerCase().replace(/[^a-z0-9]/g, '');
    if (QUICK_ADD_URL_FIELD_NAMES.has(fieldName)) {
      if (Array.isArray(fieldValue)) {
        fieldValue.forEach(item => addQuickAddURL(urls, item));
      } else {
        addQuickAddURL(urls, fieldValue);
      }
    }
    if (QUICK_ADD_KEY_FIELD_NAMES.has(fieldName)) {
      if (Array.isArray(fieldValue)) {
        fieldValue.forEach(item => addQuickAddDelimitedKeys(keys, item));
      } else {
        addQuickAddDelimitedKeys(keys, fieldValue);
      }
    }
    collectQuickAddJSONFields(fieldValue, urls, keys);
  }
}

function parseQuickAddJSONInput(text) {
  const urls = [];
  const keys = [];

  try {
    collectQuickAddJSONFields(JSON.parse(String(text || '').trim()), urls, keys);
  } catch (_) {
    return null;
  }

  if (urls.length === 0 || keys.length === 0) return null;
  return { url: urls[0], keys };
}

function parseQuickAddLineInput(text) {
  const lines = String(text || '').split(/\r?\n/).map(l => l.trim()).filter(l => l);
  if (lines.length < 2) return null;

  const url = lines[0];
  if (!/^https?:\/\/[^\s"'<>，,]+$/i.test(url)) return null;

  const keys = [];
  addQuickAddDelimitedKeys(keys, lines.slice(1).join(','));
  if (keys.length === 0) return null;

  return { url, keys };
}

function parseQuickAddRegexInput(text) {
  const raw = String(text || '');
  if (!raw.trim()) return { url: '', keys: [] };

  const urls = [];
  const keys = [];
  const trimmed = raw.trim();

  const urlRegex = /https?:\/\/[^\s"'<>，,；;。]+/gi;
  for (const match of trimmed.matchAll(urlRegex)) {
    addQuickAddURL(urls, match[0]);
  }

  const keyValueRegex = /(?:^|[,{;\s])["']?(?:api[_-]?key|apikey|key|token|secret)["']?\s*[:=]\s*["']?([^"',\s}\]]+)/gi;
  for (const match of trimmed.matchAll(keyValueRegex)) {
    addQuickAddDelimitedKeys(keys, match[1]);
  }

  const skRegex = /\bsk-[A-Za-z0-9._-]{6,}\b/g;
  for (const match of trimmed.matchAll(skRegex)) {
    addUniqueQuickAddValue(keys, match[0]);
  }

  const lines = raw.split(/\r?\n/).map(l => l.trim()).filter(l => l);
  const url = urls[0] || (lines[0] || '');

  return { url, keys };
}

// 解析粘贴框:优先识别 JSON 和“首行 URL、后续 Key”，两者都不符合时再用正则从任意文本兜底提取 URL 与 sk-* Key。
function parseQuickAddInput(text) {
  const raw = String(text || '');
  if (!raw.trim()) return { url: '', keys: [] };

  return parseQuickAddJSONInput(raw) || parseQuickAddLineInput(raw) || parseQuickAddRegexInput(raw);
}

function getQuickAddModal() {
  return document.getElementById('quickAddChannelModal');
}

function closeQuickAddModal() {
  const modal = getQuickAddModal();
  if (modal) modal.classList.remove('show');
}

function resetQuickAddForm() {
  const input = document.getElementById('quickAddInput');
  if (input) input.value = '';
  const nameInput = document.getElementById('quickAddName');
  if (nameInput) nameInput.value = '';
  const priorityInput = document.getElementById('quickAddPriority');
  if (priorityInput) priorityInput.value = '299';
  const models = document.getElementById('quickAddModels');
  if (models) models.value = '';
  const typeSel = document.getElementById('quickAddType');
  if (typeSel) typeSel.value = QUICK_ADD_TYPE_DEFAULT;
  const srcSel = document.getElementById('quickAddModelSource');
  if (srcSel) srcSel.innerHTML = '<option value="">-- 选择源渠道 --</option>';
  const channelGroupSel = document.getElementById('quickAddChannelGroup');
  if (channelGroupSel && typeof refreshChannelGroupOptions === 'function') refreshChannelGroupOptions();
  const groupSel = document.getElementById('quickAddGroup');
  if (groupSel) groupSel.innerHTML = '<option value="">-- 不加入分组 --</option>';
  const previewContent = document.getElementById('quickAddPreviewContent');
  if (previewContent) previewContent.classList.add('hidden');
}

async function loadQuickAddModelSources(type) {
  const srcSel = document.getElementById('quickAddModelSource');
  if (!srcSel) return;
  srcSel.innerHTML = '<option value="">-- 选择源渠道 --</option>';
  if (!type) return;
  try {
    const list = await fetchDataWithAuth('/admin/channels');
    const channels = (list && Array.isArray(list.channels)) ? list.channels : (Array.isArray(list) ? list : []);
    const sameType = channels.filter(c => c.channel_type === type);
    for (const c of sameType) {
      const opt = document.createElement('option');
      opt.value = String(c.id);
      opt.textContent = c.name ? `${c.name} (#${c.id})` : `#${c.id}`;
      srcSel.appendChild(opt);
    }
  } catch (e) {
    console.error('Failed to load channels for quick-add model source', e);
  }
}

async function loadQuickAddGroups() {
  const groupSel = document.getElementById('quickAddGroup');
  if (!groupSel) return;
  groupSel.innerHTML = '<option value="">-- 不加入分组 --</option>';
  try {
    const resp = await fetchDataWithAuth('/admin/auth-token-groups');
    const groups = (resp && Array.isArray(resp.groups)) ? resp.groups : (Array.isArray(resp) ? resp : []);
    for (const g of groups) {
      const opt = document.createElement('option');
      opt.value = String(g.id);
      opt.textContent = g.name ? `${g.name} (#${g.id})` : `#${g.id}`;
      groupSel.appendChild(opt);
    }
  } catch (e) {
    console.error('Failed to load auth token groups for quick-add', e);
  }
}

function updateQuickAddPreview() {
  const input = document.getElementById('quickAddInput');
  const previewContent = document.getElementById('quickAddPreviewContent');
  const previewText = document.getElementById('quickAddPreviewText');
  if (!input || !previewContent || !previewText) return;

  const { url, keys } = parseQuickAddInput(input.value);
  if (!url || keys.length === 0) {
    previewContent.classList.add('hidden');
    return;
  }

  const typeSel = document.getElementById('quickAddType');
  const type = typeSel ? typeSel.value : QUICK_ADD_TYPE_DEFAULT;
  const srcSel = document.getElementById('quickAddModelSource');
  const modelsInput = document.getElementById('quickAddModels');
  const hasModelSource = srcSel && srcSel.value;
  const manualModels = (modelsInput && modelsInput.value || '')
    .split(',').map(s => s.trim()).filter(Boolean);
  const modelDesc = hasModelSource
    ? '复制源渠道模型'
    : (manualModels.length > 0 ? `${manualModels.length} 个手动模型` : '⚠️ 未配模型');

  const channelGroupSel = document.getElementById('quickAddChannelGroup');
  const channelGroupName = channelGroupSel && channelGroupSel.value && channelGroupSel.value !== '0'
    ? `渠道分组 #${channelGroupSel.value}`
    : '未分组';
  const groupSel = document.getElementById('quickAddGroup');
  const groupName = groupSel && groupSel.value ? `API令牌分组 #${groupSel.value}` : '不加入API令牌分组';

  previewText.textContent = `URL: ${url} | ${keys.length} 个 Key | 类型 ${type} | ${modelDesc} | ${channelGroupName} | ${groupName}`;
  previewContent.classList.remove('hidden');
}

async function showQuickAddModal() {
  resetQuickAddForm();
  const modal = getQuickAddModal();
  if (modal) modal.classList.add('show');
  await Promise.all([
    loadQuickAddModelSources(QUICK_ADD_TYPE_DEFAULT),
    loadQuickAddGroups()
  ]);
  updateQuickAddPreview();
}

function setQuickAddPending(pending) {
  const btn = document.getElementById('quickAddSubmitBtn');
  if (!btn) return;
  btn.disabled = !!pending;
  btn.textContent = pending ? '添加中...' : '确认添加';
}

async function confirmQuickAdd() {
  const input = document.getElementById('quickAddInput');
  if (!input) return;
  const { url, keys } = parseQuickAddInput(input.value);
  if (!url) {
    if (window.showError) window.showError('请粘贴 URL(首行)');
    return;
  }
  if (keys.length === 0) {
    if (window.showError) window.showError('请粘贴至少 1 个 API Key(首行之后的行)');
    return;
  }

  const typeSel = document.getElementById('quickAddType');
  const nameInput = document.getElementById('quickAddName');
  const srcSel = document.getElementById('quickAddModelSource');
  const modelsInput = document.getElementById('quickAddModels');
  const groupSel = document.getElementById('quickAddGroup');
  const channelGroupSel = document.getElementById('quickAddChannelGroup');

  const channelType = typeSel ? typeSel.value : QUICK_ADD_TYPE_DEFAULT;
  const name = (nameInput && nameInput.value.trim()) || hostnameFromURL(url) || '';
  const priorityInput = document.getElementById('quickAddPriority');
  const priority = priorityInput ? (parseInt(priorityInput.value, 10) || 0) : 0;
  const modelSourceId = srcSel && srcSel.value ? Number(srcSel.value) : null;
  const manualModels = (modelsInput && modelsInput.value || '')
    .split(',').map(s => s.trim()).filter(Boolean);
  const groupId = groupSel && groupSel.value ? Number(groupSel.value) : null;
  const channelGroupId = channelGroupSel && channelGroupSel.value ? Number(channelGroupSel.value) : 0;

  if (!modelSourceId && manualModels.length === 0) {
    if (window.showError) window.showError('请选择模型来源渠道,或手动填模型名');
    return;
  }

  const payload = {
    url,
    api_keys: keys,
    channel_type: channelType,
    name,
    priority
  };
  if (modelSourceId) {
    payload.model_source_channel_id = modelSourceId;
  } else {
    payload.models = manualModels;
  }
  if (groupId) payload.auth_token_group_id = groupId;
  if (channelGroupId > 0) payload.channel_group_id = channelGroupId;

  setQuickAddPending(true);
  try {
    const resp = await fetchAPIWithAuth('/admin/channels/quick-add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    if (!resp.success) throw new Error(resp.error || '添加失败');

    closeQuickAddModal();
    if (typeof reloadChannelsList === 'function') {
      void reloadChannelsList();
    } else if (typeof loadChannels === 'function') {
      void loadChannels();
    }
    if (window.showSuccess) {
      const ch = resp.data && resp.data.channel;
      const groupName = (resp.data && resp.data.group && resp.data.group.name) || '';
      const channelGroupName = (resp.data && resp.data.channel_group && resp.data.channel_group.name) || '';
      const tail = [channelGroupName ? `渠道分组「${channelGroupName}」` : '', groupName ? `API令牌分组「${groupName}」` : ''].filter(Boolean).join(',');
      window.showSuccess(`渠道「${ch ? ch.name : url}」已创建${tail ? '，' + tail : ''}`);
    }
  } catch (e) {
    console.error('Quick add channel failed', e);
    if (window.showError) window.showError(`快速添加失败: ${e.message}`);
  } finally {
    setQuickAddPending(false);
  }
}

function setupQuickAddPreview() {
  const input = document.getElementById('quickAddInput');
  if (input && !input.dataset.bound) {
    input.addEventListener('input', () => {
      const { url } = parseQuickAddInput(input.value);
      if (url) {
        const typeSel = document.getElementById('quickAddType');
        const nameInput = document.getElementById('quickAddName');
        if (typeSel && !typeSel.dataset.userChanged) {
          typeSel.value = detectTypeFromURL(url);
          void loadQuickAddModelSources(typeSel.value);
        }
        if (nameInput && !nameInput.dataset.userChanged) {
          nameInput.value = hostnameFromURL(url);
        }
      }
      updateQuickAddPreview();
    });
    input.dataset.bound = '1';
  }

  const typeSel = document.getElementById('quickAddType');
  if (typeSel && !typeSel.dataset.bound) {
    typeSel.addEventListener('change', () => {
      typeSel.dataset.userChanged = '1';
      void loadQuickAddModelSources(typeSel.value);
      updateQuickAddPreview();
    });
    typeSel.dataset.bound = '1';
  }

  const nameInput = document.getElementById('quickAddName');
  if (nameInput && !nameInput.dataset.bound) {
    nameInput.addEventListener('input', () => {
      nameInput.dataset.userChanged = '1';
    });
    nameInput.dataset.bound = '1';
  }

  const srcSel = document.getElementById('quickAddModelSource');
  if (srcSel && !srcSel.dataset.bound) {
    srcSel.addEventListener('change', updateQuickAddPreview);
    srcSel.dataset.bound = '1';
  }

  const modelsInput = document.getElementById('quickAddModels');
  if (modelsInput && !modelsInput.dataset.bound) {
    modelsInput.addEventListener('input', updateQuickAddPreview);
    modelsInput.dataset.bound = '1';
  }

  const channelGroupSel = document.getElementById('quickAddChannelGroup');
  if (channelGroupSel && !channelGroupSel.dataset.bound) {
    channelGroupSel.addEventListener('change', updateQuickAddPreview);
    channelGroupSel.dataset.bound = '1';
  }

  const groupSel = document.getElementById('quickAddGroup');
  if (groupSel && !groupSel.dataset.bound) {
    groupSel.addEventListener('change', updateQuickAddPreview);
    groupSel.dataset.bound = '1';
  }
}

window.setupQuickAddPreview = setupQuickAddPreview;
window.showQuickAddModal = showQuickAddModal;
window.closeQuickAddModal = closeQuickAddModal;
window.confirmQuickAdd = confirmQuickAdd;
