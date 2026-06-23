// 快速添加渠道:粘贴 URL + Key(s),选模型来源,可选追加到 auth token 分组。

const QUICK_ADD_TYPE_DEFAULT = 'anthropic';

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

// 解析粘贴框:首行 URL,后续行 Key(逗号或换行分隔),去空去重。
function parseQuickAddInput(text) {
  if (!text || !text.trim()) return { url: '', keys: [] };
  const lines = text.split(/\r?\n/).map(l => l.trim()).filter(l => l);
  if (lines.length === 0) return { url: '', keys: [] };
  const url = lines[0];
  const keyPart = lines.slice(1).join(',');
  const keys = keyPart
    .split(/[,\n]/)
    .map(k => k.trim())
    .filter(k => k);
  const seen = new Set();
  const unique = [];
  for (const k of keys) {
    if (seen.has(k)) continue;
    seen.add(k);
    unique.push(k);
  }
  return { url, keys: unique };
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
  const name = document.getElementById('quickAddName');
  if (name) name.value = '';
  const models = document.getElementById('quickAddModels');
  if (models) models.value = '';
  const typeSel = document.getElementById('quickAddType');
  if (typeSel) typeSel.value = QUICK_ADD_TYPE_DEFAULT;
  const srcSel = document.getElementById('quickAddModelSource');
  if (srcSel) srcSel.innerHTML = '<option value="">-- 选择源渠道 --</option>';
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

  const groupSel = document.getElementById('quickAddGroup');
  const groupName = groupSel && groupSel.value ? `加入分组 #${groupSel.value}` : '不加入分组';

  previewText.textContent = `URL: ${url} | ${keys.length} 个 Key | 类型 ${type} | ${modelDesc} | ${groupName}`;
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

  const channelType = typeSel ? typeSel.value : QUICK_ADD_TYPE_DEFAULT;
  const name = (nameInput && nameInput.value.trim()) || hostnameFromURL(url) || '';
  const modelSourceId = srcSel && srcSel.value ? Number(srcSel.value) : null;
  const manualModels = (modelsInput && modelsInput.value || '')
    .split(',').map(s => s.trim()).filter(Boolean);
  const groupId = groupSel && groupSel.value ? Number(groupSel.value) : null;

  if (!modelSourceId && manualModels.length === 0) {
    if (window.showError) window.showError('请选择模型来源渠道,或手动填模型名');
    return;
  }

  const payload = {
    url,
    api_keys: keys,
    channel_type: channelType,
    name
  };
  if (modelSourceId) {
    payload.model_source_channel_id = modelSourceId;
  } else {
    payload.models = manualModels;
  }
  if (groupId) payload.group_id = groupId;

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
      const tail = groupName ? `,已加入分组「${groupName}」` : '';
      window.showSuccess(`渠道「${ch ? ch.name : url}」已创建${tail}`);
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
