// 快速添加渠道:粘贴 URL + Key(s),选模型来源,可选追加到 auth token 分组。

const QUICK_ADD_TYPE_DEFAULT = 'codex';

function quickAddFieldKind(name) {
  const compact = String(name || '').trim().toLowerCase().replace(/[^a-z0-9\u5bc6\u94a5]/g, '');
  if (compact.includes('url') || compact.includes('endpoint') || compact.endsWith('apibase')) {
    return 'url';
  }
  if (compact.includes('apikey') || compact === 'key' || compact === 'keys' ||
      compact.endsWith('token') || compact.endsWith('tokens') ||
      compact.includes('secret') || compact.includes('\u5bc6\u94a5')) {
    return 'key';
  }
  return '';
}

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

function normalizeQuickAddURL(value) {
  const candidate = String(value || '')
    .trim()
    .replace(/\\\//g, '/')
    .replace(/^[`"'“”‘’\s]+/, '')
    .replace(/[)\]}>,，。.;；\s]+$/, '');
  let parsed;
  try {
    parsed = new URL(candidate);
  } catch (_) {
    return '';
  }
  if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.host ||
      parsed.username || parsed.password || parsed.search || parsed.hash) {
    return '';
  }

  let path = parsed.pathname.replace(/\/+$/, '');
  path = path.replace(/\/v1(?:\/.*)?$/i, '');
  return `${parsed.protocol}//${parsed.host}${path}`;
}

function addQuickAddURL(values, value) {
  const normalized = normalizeQuickAddURL(value);
  if (!normalized) return;
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
    const fieldKind = quickAddFieldKind(field);
    if (fieldKind === 'url') {
      if (Array.isArray(fieldValue)) {
        fieldValue.forEach(item => addQuickAddURL(urls, item));
      } else {
        addQuickAddURL(urls, fieldValue);
      }
    }
    if (fieldKind === 'key') {
      if (Array.isArray(fieldValue)) {
        fieldValue.forEach(item => addQuickAddDelimitedKeys(keys, item));
      } else {
        addQuickAddDelimitedKeys(keys, fieldValue);
      }
    }
    collectQuickAddJSONFields(fieldValue, urls, keys);
  }
}

function collectQuickAddJSONInput(text, urls, keys) {
  try {
    collectQuickAddJSONFields(JSON.parse(String(text || '').trim()), urls, keys);
  } catch (_) {
    // 普通文本不是 JSON，继续解析环境变量和标签文本。
  }
}

function collectQuickAddAssignments(text, urls, keys) {
  for (const line of String(text || '').split(/\r?\n/)) {
    const assignment = line.match(/^\s*(?:export\s+|set\s+)?([^:=]+?)\s*[:=]\s*(.+?)\s*$/i);
    if (!assignment) continue;
    const fieldKind = quickAddFieldKind(assignment[1]);
    if (fieldKind === 'url') addQuickAddURL(urls, assignment[2]);
    if (fieldKind === 'key') addQuickAddDelimitedKeys(keys, assignment[2].replace(/^bearer\s+/i, ''));
  }
}

function collectQuickAddTextFields(text, urls, keys) {
  const raw = String(text || '');
  const urlRegex = /https?:\/\/[^\s"'<>，,；;。]+/gi;
  for (const match of raw.matchAll(urlRegex)) {
    addQuickAddURL(urls, match[0]);
  }

  const keyValueRegex = /(?:api[\s_-]*keys?|access[\s_-]*tokens?|tokens?|secret(?:[\s_-]*keys?)?|\u5bc6\u94a5)\s*[:=]\s*(?:"([^"]+)"|'([^']+)'|([^\s,;\]}]+))/gi;
  for (const match of raw.matchAll(keyValueRegex)) {
    addQuickAddDelimitedKeys(keys, match[1] || match[2] || match[3]);
  }

  const bearerRegex = /\bbearer\s+([^\s"',;]+)/gi;
  for (const match of raw.matchAll(bearerRegex)) {
    addQuickAddDelimitedKeys(keys, match[1]);
  }

  const skRegex = /\bsk-[A-Za-z0-9._-]{6,}\b/g;
  for (const match of raw.matchAll(skRegex)) {
    addUniqueQuickAddValue(keys, match[0]);
  }
}

// 解析粘贴框:合并识别 JSON、环境变量、标签文本和“首行 URL、后续 Key”。
function parseQuickAddInput(text) {
  const raw = String(text || '');
  if (!raw.trim()) return { url: '', keys: [] };

  const urls = [];
  const keys = [];
  collectQuickAddJSONInput(raw, urls, keys);
  collectQuickAddAssignments(raw, urls, keys);
  collectQuickAddTextFields(raw, urls, keys);

  const lines = raw.split(/\r?\n/).map(line => line.trim()).filter(Boolean);
  if (urls.length === 0 && lines.length > 0) addQuickAddURL(urls, lines[0]);
  if (keys.length === 0 && urls.length > 0 && lines.length > 1) {
    addQuickAddDelimitedKeys(keys, lines.slice(1).join(','));
  }

  return { url: urls[0] || '', keys };
}

async function discoverQuickAddModels(url, apiKey, channelType, request = fetchAPIWithAuth) {
  const response = await request('/admin/channels/models/fetch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      channel_type: channelType || QUICK_ADD_TYPE_DEFAULT,
      url,
      api_key: apiKey
    })
  });
  if (!response?.success) {
    throw new Error(response?.error || '模型探测失败');
  }

  const models = [];
  const seen = new Set();
  for (const entry of response.data?.models || []) {
    const modelName = String(typeof entry === 'string' ? entry : entry?.model || '').trim();
    const modelKey = modelName.toLowerCase();
    if (!modelName || seen.has(modelKey)) continue;
    seen.add(modelKey);
    models.push(modelName);
  }
  if (models.length === 0) throw new Error('上游未返回可用模型');
  return models;
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
  if (nameInput) {
    nameInput.value = '';
    delete nameInput.dataset.userChanged;
  }
  const priorityInput = document.getElementById('quickAddPriority');
  if (priorityInput) priorityInput.value = '299';
  const models = document.getElementById('quickAddModels');
  if (models) models.value = '';
  const typeSel = document.getElementById('quickAddType');
  if (typeSel) {
    typeSel.value = QUICK_ADD_TYPE_DEFAULT;
    delete typeSel.dataset.userChanged;
  }
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
    : (manualModels.length > 0 ? `${manualModels.length} 个手动模型` : '将自动探测模型');

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
  btn.textContent = pending ? '探测并添加中...' : '确认添加';
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
  let manualModels = (modelsInput && modelsInput.value || '')
    .split(/[,\n]/).map(s => s.trim()).filter(Boolean);
  const groupId = groupSel && groupSel.value ? Number(groupSel.value) : null;
  const channelGroupId = channelGroupSel && channelGroupSel.value ? Number(channelGroupSel.value) : 0;

  setQuickAddPending(true);
  try {
    if (!modelSourceId && manualModels.length === 0) {
      manualModels = await discoverQuickAddModels(url, keys[0], channelType);
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
