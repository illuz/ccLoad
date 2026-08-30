(function (root, factory) {
  const api = factory(root);
  if (typeof module === 'object' && module.exports) module.exports = api;
  if (root) root.ModelTestImage = api;
})(typeof window !== 'undefined' ? window : globalThis, function (root) {
  'use strict';

  const STORAGE_PREFIX = 'ccload_model_test_image_';
  const IMAGES_SIZE_OPTIONS = [
    ['auto', 'Auto'],
    ['1024x1024', '1024 x 1024'],
    ['1536x1024', '1536 x 1024'],
    ['1024x1536', '1024 x 1536'],
    ['1792x1024', '1792 x 1024'],
    ['1024x1792', '1024 x 1792'],
    ['512x512', '512 x 512'],
    ['256x256', '256 x 256']
  ];
  const CHAT_SIZE_OPTIONS = [
    ['auto', 'Auto'],
    ['1:1@1k', '1:1 / 1K'],
    ['1:1@2k', '1:1 / 2K'],
    ['16:9@1k', '16:9 / 1K'],
    ['16:9@2k', '16:9 / 2K'],
    ['9:16@1k', '9:16 / 1K'],
    ['9:16@2k', '9:16 / 2K'],
    ['3:2@1k', '3:2 / 1K'],
    ['3:2@2k', '3:2 / 2K'],
    ['2:3@1k', '2:3 / 1K'],
    ['2:3@2k', '2:3 / 2K']
  ];
  const GENERATION_API_OPTIONS = [
    ['images', 'Images API', 'modelTest.image.generationApiImages'],
    ['chat_completions', 'Chat Completions', 'modelTest.image.generationApiChat']
  ];
  const QUALITY_OPTIONS = [
    ['auto', 'Auto', 'modelTest.image.auto'],
    ['low', 'Low', 'modelTest.image.qualityLow'],
    ['medium', 'Medium', 'modelTest.image.qualityMedium'],
    ['high', 'High', 'modelTest.image.qualityHigh'],
    ['standard', 'Standard', 'modelTest.image.qualityStandard'],
    ['hd', 'HD', 'modelTest.image.qualityHD']
  ];
  const BACKGROUND_OPTIONS = [
    ['auto', 'Auto', 'modelTest.image.auto'],
    ['opaque', 'Opaque', 'modelTest.image.backgroundOpaque'],
    ['transparent', 'Transparent', 'modelTest.image.backgroundTransparent']
  ];
  const OUTPUT_FORMAT_OPTIONS = [
    ['auto', 'Auto', 'modelTest.image.auto'],
    ['png', 'PNG'],
    ['jpeg', 'JPEG'],
    ['webp', 'WebP']
  ];

  let dependencies = {};
  let channels = [];
  let channelKeys = [];
  let initialized = false;
  let keyRequestID = 0;
  let submitting = false;
  let preferredChannelID = '';
  let currentSizeOptions = IMAGES_SIZE_OPTIONS;
  let modelCombobox = null;
  let channelCombobox = null;
  let keyCombobox = null;
  let generationAPICombobox = null;
  let sizeCombobox = null;
  let qualityCombobox = null;
  let backgroundCombobox = null;
  let outputFormatCombobox = null;

  function text(key, fallback, params) {
    if (typeof dependencies.t === 'function') return dependencies.t(key, fallback, params);
    if (typeof root?.t === 'function') return root.t(key, params) || fallback;
    return fallback;
  }

  function element(id) {
    return root?.document?.getElementById(id) || null;
  }

  function storageGet(key, fallback = '') {
    try {
      const value = root?.localStorage?.getItem(STORAGE_PREFIX + key);
      return value === null ? fallback : value;
    } catch (_) {
      return fallback;
    }
  }

  function storageSet(key, value) {
    try {
      root?.localStorage?.setItem(STORAGE_PREFIX + key, String(value ?? ''));
    } catch (_) { /* ignore */ }
  }

  function comboboxOptions(definitions) {
    return definitions.map(([value, fallback, key]) => ({
      value,
      label: key ? text(key, fallback) : fallback
    }));
  }

  function setComboboxSelection(combobox, inputID, definitions, value, fallback = '') {
    const options = comboboxOptions(definitions);
    const selected = options.find(option => option.value === value)
      || options.find(option => option.value === fallback)
      || options[0]
      || { value: '', label: '' };
    combobox?.setValue(selected.value, selected.label);
    combobox?.refresh();
    const input = combobox?.getInput?.() || element(inputID);
    if (input && !combobox) input.value = selected.label;
    return selected.value;
  }

  function buildRequestPayload(values) {
    const generationAPI = String(values?.generationAPI || 'images').trim().toLowerCase();
    const payload = {
      generation_api: generationAPI,
      model: String(values?.model || '').trim(),
      prompt: String(values?.prompt || '').trim()
    };
    const rawKeyIndex = String(values?.keyIndex ?? '').trim();
    const keyIndex = Number(rawKeyIndex);
    if (rawKeyIndex && Number.isInteger(keyIndex) && keyIndex >= 0) payload.key_index = keyIndex;

    const size = String(values?.size || '').trim().toLowerCase();
    if (size && size !== 'auto') payload.size = size;
    if (generationAPI === 'images' && values?.supportsExtendedOptions !== false) {
      for (const [inputKey, outputKey] of [
        ['quality', 'quality'],
        ['background', 'background'],
        ['outputFormat', 'output_format']
      ]) {
        const value = String(values?.[inputKey] || '').trim().toLowerCase();
        if (value && value !== 'auto') payload[outputKey] = value;
      }
    }
    return payload;
  }

  function safeRemoteImageURL(value) {
    if (typeof value !== 'string' || !value.trim()) return '';
    try {
      const parsed = new URL(value.trim());
      return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : '';
    } catch (_) {
      return '';
    }
  }

  function normalizeImages(data) {
    if (!Array.isArray(data?.images)) return [];
    return data.images.filter(image => image && (
      safeRemoteImageURL(image.url)
      || typeof image.b64_json === 'string' && image.b64_json.trim()
    ));
  }

  function imageMIMEType(outputFormat, explicitMIMEType) {
    const mimeType = String(explicitMIMEType || '').trim().toLowerCase();
    if (['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(mimeType)) return mimeType;
    const format = String(outputFormat || '').trim().toLowerCase();
    if (format === 'jpeg' || format === 'jpg') return 'image/jpeg';
    if (format === 'webp') return 'image/webp';
    if (format === 'gif') return 'image/gif';
    return 'image/png';
  }

  function dataURLFromImage(image, outputFormat) {
    const remoteURL = safeRemoteImageURL(image?.url);
    if (remoteURL) return remoteURL;
    if (typeof image?.b64_json !== 'string' || !image.b64_json.trim()) return '';
    return `data:${imageMIMEType(outputFormat, image?.mime_type)};base64,${image.b64_json.trim()}`;
  }

  function outputFormatFromMIMEType(value) {
    switch (String(value || '').trim().toLowerCase()) {
      case 'image/jpeg': return 'jpeg';
      case 'image/webp': return 'webp';
      case 'image/gif': return 'gif';
      case 'image/png': return 'png';
      default: return '';
    }
  }

  function getModelName(entry) {
    if (typeof dependencies.getModelName === 'function') return dependencies.getModelName(entry);
    return typeof entry === 'string' ? entry : String(entry?.model || '');
  }

  function modelOptions() {
    if (typeof dependencies.getModelOptions === 'function') {
      return dependencies.getModelOptions().filter(model => model && model !== '*');
    }
    const models = new Set();
    channels.forEach(channel => {
      (channel?.models || []).forEach(entry => {
        const name = String(getModelName(entry) || '').trim();
        if (name && name !== '*') models.add(name);
      });
    });
    return Array.from(models).sort((a, b) => a.localeCompare(b));
  }

  function channelSupportsModel(channel, model) {
    if (!model) return true;
    if (typeof dependencies.isModelSupported === 'function') {
      return dependencies.isModelSupported(channel, model);
    }
    return (channel?.models || []).some(entry => {
      const name = getModelName(entry);
      return name === '*' || name === model;
    });
  }

  function currentModel() {
    return String(modelCombobox?.getValue?.() || element('imageModelInput')?.value || '').trim();
  }

  function currentGenerationAPI() {
    return String(generationAPICombobox?.getValue?.() || storageGet('generation_api', 'images')).trim().toLowerCase();
  }

  function selectedChannel() {
    const selectedID = String(channelCombobox?.getValue?.() || preferredChannelID || '');
    return channels.find(channel => String(channel?.id) === selectedID) || null;
  }

  function supportsExtendedImageOptions() {
    return currentGenerationAPI() === 'images';
  }

  function imageSizeOptions(generationAPI) {
    return generationAPI === 'chat_completions' ? CHAT_SIZE_OPTIONS : IMAGES_SIZE_OPTIONS;
  }

  function imageSizeStorageKey() {
    return currentGenerationAPI() === 'chat_completions' ? 'size_chat' : 'size_images';
  }

  function currentSizeOptionDefinitions() {
    return currentSizeOptions.map(([value, label]) => [
      value,
      label,
      value === 'auto' ? 'modelTest.image.auto' : ''
    ]);
  }

  function syncSizeOptions() {
    currentSizeOptions = imageSizeOptions(currentGenerationAPI());
    const stored = storageGet(imageSizeStorageKey(), 'auto');
    const selected = currentSizeOptions.some(([value]) => value === stored) ? stored : 'auto';
    setComboboxSelection(sizeCombobox, 'imageSizeSelect', currentSizeOptionDefinitions(), selected, 'auto');
  }

  function syncCapabilityControls() {
    const supported = supportsExtendedImageOptions();
    for (const { id, storageKey, combobox, options } of [
      { id: 'imageQualitySelect', storageKey: 'quality', combobox: qualityCombobox, options: QUALITY_OPTIONS },
      { id: 'imageBackgroundSelect', storageKey: 'background', combobox: backgroundCombobox, options: BACKGROUND_OPTIONS },
      { id: 'imageOutputFormatSelect', storageKey: 'output_format', combobox: outputFormatCombobox, options: OUTPUT_FORMAT_OPTIONS }
    ]) {
      const field = combobox?.getInput?.() || element(id);
      if (!field) continue;
      field.disabled = !supported;
      field.setAttribute('aria-disabled', supported ? 'false' : 'true');
      const stored = supported ? storageGet(storageKey, 'auto') : 'auto';
      setComboboxSelection(combobox, id, options, stored, 'auto');
    }
  }

  function syncGenerationAPIControls() {
    syncSizeOptions();
    syncCapabilityControls();
  }

  function availableChannels() {
    const model = currentModel();
    if (typeof dependencies.getChannelsForModel === 'function') {
      return dependencies.getChannelsForModel(model);
    }
    return channels.filter(channel => channelSupportsModel(channel, model));
  }

  function formatChannelLabel(channel) {
    if (typeof dependencies.formatChannelLabel === 'function') return dependencies.formatChannelLabel(channel);
    return String(channel?.name || `#${channel?.id ?? '?'}`);
  }

  function formatKeyLabel(key) {
    if (typeof dependencies.formatKeyLabel === 'function') return dependencies.formatKeyLabel(key);
    const raw = String(key?.api_key || '').trim();
    const masked = raw.length <= 6 ? raw : `${raw.slice(0, 3)}.${raw.slice(-3)}`;
    return `#${key?.key_index ?? '?'} ${masked}`.trim();
  }

  function syncModelOptions() {
    const models = modelOptions();
    const selected = currentModel() || storageGet('model') || models[0] || '';
    modelCombobox?.setValue(selected, selected);
    modelCombobox?.refresh();
  }

  function syncChannelOptions() {
    const available = availableChannels();
    const channel = available.find(item => String(item.id) === preferredChannelID) || available[0] || null;
    const selectedID = channel ? String(channel.id) : '';
    preferredChannelID = selectedID;
    channelCombobox?.setValue(selectedID, channel ? formatChannelLabel(channel) : '');
    channelCombobox?.refresh();
    storageSet('channel_id', selectedID);
    syncGenerationAPIControls();
    void syncKeyOptions(selectedID);
  }

  async function syncKeyOptions(channelID) {
    const currentRequestID = ++keyRequestID;
    channelKeys = [];
    keyCombobox?.setValue('', '');
    keyCombobox?.refresh();
    const input = keyCombobox?.getInput?.() || element('imageKeySelect');
    if (!channelID) {
      if (input) input.placeholder = text('modelTest.image.selectChannel', 'Select a channel');
      return;
    }
    if (input) input.placeholder = text('modelTest.image.loadingKeys', 'Loading API keys...');
    if (typeof dependencies.getChannelKeys !== 'function') return;

    try {
      const keys = await dependencies.getChannelKeys(Number(channelID));
      if (currentRequestID !== keyRequestID) return;
      channelKeys = Array.isArray(keys) ? keys : [];
      const preferredIndex = storageGet(`key_index_${channelID}`);
      const selectedKey = channelKeys.find(key => key?.disabled !== true && String(key.key_index) === preferredIndex)
        || channelKeys.find(key => key?.disabled !== true)
        || null;
      const selectedIndex = selectedKey ? String(selectedKey.key_index) : '';
      keyCombobox?.setValue(selectedIndex, selectedKey ? formatKeyLabel(selectedKey) : '');
      keyCombobox?.refresh();
      if (input) {
        input.placeholder = selectedKey
          ? text('channels.selectApiKey', 'Select API key')
          : text('modelTest.image.noKeys', 'No available API keys');
      }
      if (selectedIndex) storageSet(`key_index_${channelID}`, selectedIndex);
    } catch (_) {
      if (currentRequestID !== keyRequestID) return;
      if (input) input.placeholder = text('modelTest.image.loadKeysFailed', 'Failed to load API keys');
    }
  }

  function initTargetComboboxes() {
    if (typeof root?.createSearchableCombobox !== 'function') return;
    const storedModel = storageGet('model');
    preferredChannelID = storageGet('channel_id');
    modelCombobox = root.createSearchableCombobox({
      attachMode: true,
      inputId: 'imageModelInput',
      dropdownId: 'imageModelDropdown',
      allowCustomInput: true,
      initialValue: storedModel,
      initialLabel: storedModel,
      getOptions: () => modelOptions().map(model => ({ value: model, label: model })),
      onSelect: value => {
        storageSet('model', String(value || '').trim());
        syncChannelOptions();
      }
    });
    channelCombobox = root.createSearchableCombobox({
      attachMode: true,
      inputId: 'imageChannelSelect',
      dropdownId: 'imageChannelDropdown',
      initialValue: preferredChannelID,
      initialLabel: '',
      getOptions: () => availableChannels().map(channel => ({
        value: String(channel.id),
        label: formatChannelLabel(channel)
      })),
      onSelect: value => {
        preferredChannelID = String(value || '');
        storageSet('channel_id', preferredChannelID);
        void syncKeyOptions(preferredChannelID);
      }
    });
    keyCombobox = root.createSearchableCombobox({
      attachMode: true,
      inputId: 'imageKeySelect',
      dropdownId: 'imageKeyDropdown',
      initialValue: '',
      initialLabel: '',
      getOptions: () => channelKeys.map(key => ({
        value: String(key.key_index),
        label: formatKeyLabel(key)
      })),
      onSelect: value => {
        const channelID = String(channelCombobox?.getValue?.() || '');
        if (channelID) storageSet(`key_index_${channelID}`, value);
      }
    });
  }

  function createOptionCombobox(inputId, dropdownId, getDefinitions, onSelect) {
    return root.createSearchableCombobox({
      attachMode: true,
      inputId,
      dropdownId,
      initialValue: '',
      initialLabel: '',
      getOptions: () => comboboxOptions(getDefinitions()),
      onSelect
    });
  }

  function initOptionComboboxes() {
    if (typeof root?.createSearchableCombobox !== 'function') return;
    generationAPICombobox = createOptionCombobox(
      'imageGenerationAPISelect',
      'imageGenerationAPISelectDropdown',
      () => GENERATION_API_OPTIONS,
      value => {
        storageSet('generation_api', value);
        syncGenerationAPIControls();
      }
    );
    sizeCombobox = createOptionCombobox(
      'imageSizeSelect',
      'imageSizeSelectDropdown',
      currentSizeOptionDefinitions,
      value => storageSet(imageSizeStorageKey(), value)
    );
    qualityCombobox = createOptionCombobox(
      'imageQualitySelect',
      'imageQualitySelectDropdown',
      () => QUALITY_OPTIONS,
      value => storageSet('quality', value)
    );
    backgroundCombobox = createOptionCombobox(
      'imageBackgroundSelect',
      'imageBackgroundSelectDropdown',
      () => BACKGROUND_OPTIONS,
      value => storageSet('background', value)
    );
    outputFormatCombobox = createOptionCombobox(
      'imageOutputFormatSelect',
      'imageOutputFormatSelectDropdown',
      () => OUTPUT_FORMAT_OPTIONS,
      value => storageSet('output_format', value)
    );
  }

  function setStatus(message, error = false) {
    const status = element('imageGenerationStatus');
    const errorRegion = element('imageGenerationError');
    const feedback = element('imageGenerationFeedback');
    if (status) status.textContent = error ? '' : message;
    if (errorRegion) errorRegion.textContent = error ? message : '';
    if (feedback) feedback.hidden = !message;
  }

  function setBusy(busy) {
    const form = element('imageGenerationForm');
    const results = element('imageGenerationResults');
    const button = element('imageGenerateBtn');
    form?.setAttribute('aria-busy', busy ? 'true' : 'false');
    results?.setAttribute('aria-busy', busy ? 'true' : 'false');
    if (!button) return;
    button.disabled = busy;
    button.classList.toggle('is-loading', busy);
    const label = button.querySelector('span');
    if (label) {
      label.textContent = busy
        ? text('modelTest.image.generating', 'Generating...')
        : text('modelTest.image.generate', 'Generate');
    }
  }

  function renderResults(data) {
    const container = element('imageGenerationResults');
    const summary = element('imageGenerationSummary');
    if (!container) return;
    const images = normalizeImages(data);
    const outputFormat = data?.output_format || 'png';
    container.replaceChildren();

    images.forEach((image, index) => {
      const imageOutputFormat = outputFormatFromMIMEType(image?.mime_type) || outputFormat;
      const source = dataURLFromImage(image, imageOutputFormat);
      const figure = root.document.createElement('figure');
      figure.className = 'image-test-result-card';

      const preview = root.document.createElement('img');
      preview.className = 'image-test-result-image';
      preview.src = source;
      preview.alt = String(image.revised_prompt || element('imagePrompt')?.value || '').trim()
        || text('modelTest.image.resultAlt', `Generated image ${index + 1}`, { index: index + 1 });
      preview.loading = 'lazy';
      preview.decoding = 'async';
      preview.referrerPolicy = 'no-referrer';
      figure.appendChild(preview);

      const caption = root.document.createElement('figcaption');
      caption.className = 'image-test-result-caption';
      if (typeof image.revised_prompt === 'string' && image.revised_prompt.trim()) {
        const revisedPrompt = root.document.createElement('p');
        revisedPrompt.className = 'image-test-revised-prompt';
        revisedPrompt.textContent = image.revised_prompt.trim();
        caption.appendChild(revisedPrompt);
      }

      const link = root.document.createElement('a');
      link.className = 'btn btn-secondary image-test-download';
      link.href = source;
      link.textContent = image.b64_json
        ? text('modelTest.image.download', 'Download')
        : text('modelTest.image.openOriginal', 'Open original');
      if (image.b64_json) {
        const extension = imageOutputFormat === 'jpeg' ? 'jpg' : imageOutputFormat;
        link.download = `generated-image-${index + 1}.${extension}`;
      } else {
        link.target = '_blank';
        link.rel = 'noopener noreferrer';
      }
      caption.appendChild(link);
      figure.appendChild(caption);
      container.appendChild(figure);
    });

    const summaryParts = [text('modelTest.image.resultCount', `${images.length} images`, { count: images.length })];
    if (Number.isFinite(Number(data?.duration_ms))) {
      const duration = Number(data.duration_ms);
      summaryParts.push(text('modelTest.image.duration', `${duration} ms`, { duration }));
    }
    if (summary) summary.textContent = summaryParts.join(' / ');
  }

  function currentValues() {
    return {
      model: currentModel(),
      prompt: element('imagePrompt')?.value,
      channelID: String(channelCombobox?.getValue?.() || ''),
      keyIndex: String(keyCombobox?.getValue?.() || ''),
      generationAPI: currentGenerationAPI(),
      size: String(sizeCombobox?.getValue?.() || 'auto'),
      quality: String(qualityCombobox?.getValue?.() || 'auto'),
      background: String(backgroundCombobox?.getValue?.() || 'auto'),
      outputFormat: String(outputFormatCombobox?.getValue?.() || 'auto'),
      supportsExtendedOptions: supportsExtendedImageOptions()
    };
  }

  async function submit(event) {
    event?.preventDefault();
    if (submitting) return;
    const form = element('imageGenerationForm');
    const values = currentValues();
    if (!values.channelID) {
      setStatus(text('modelTest.image.selectChannelError', 'Select a channel first'), true);
      element('imageChannelSelect')?.focus();
      return;
    }
    if (!values.keyIndex) {
      setStatus(text('modelTest.image.selectKeyError', 'Select an available API key'), true);
      element('imageKeySelect')?.focus();
      return;
    }
    if (!form?.reportValidity()) return;

    const payload = buildRequestPayload(values);
    storageSet('model', payload.model);
    storageSet('prompt', values.prompt);
    storageSet(`key_index_${values.channelID}`, values.keyIndex);
    storageSet('generation_api', values.generationAPI);
    storageSet(imageSizeStorageKey(), values.size);
    if (values.supportsExtendedOptions) {
      storageSet('quality', values.quality);
      storageSet('background', values.background);
      storageSet('output_format', values.outputFormat);
    }

    submitting = true;
    setStatus(text('modelTest.image.generatingStatus', 'Generating image...'));
    setBusy(true);
    try {
      const data = await root.fetchDataWithAuth(`/admin/channels/${values.channelID}/images/generations`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!data?.success) throw new Error(data?.error || text('modelTest.image.failed', 'Image generation failed'));
      renderResults(data);
      setStatus(text('modelTest.image.success', 'Image generation completed'));
    } catch (error) {
      setStatus(error?.message || text('modelTest.image.failed', 'Image generation failed'), true);
    } finally {
      submitting = false;
      setBusy(false);
    }
  }

  function restoreOptions() {
    setComboboxSelection(
      generationAPICombobox,
      'imageGenerationAPISelect',
      GENERATION_API_OPTIONS,
      storageGet('generation_api', 'images'),
      'images'
    );
    const prompt = element('imagePrompt');
    if (prompt) prompt.value = storageGet('prompt');
    syncGenerationAPIControls();
  }

  function init(nextDependencies = {}) {
    dependencies = { ...dependencies, ...nextDependencies };
    if (initialized || !root?.document) return;
    initialized = true;
    initTargetComboboxes();
    initOptionComboboxes();
    restoreOptions();
    element('imageGenerationForm')?.addEventListener('submit', submit);
    const prompt = element('imagePrompt');
    prompt?.addEventListener('input', () => storageSet('prompt', prompt.value));
    prompt?.addEventListener('keydown', event => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
        event.preventDefault();
        element('imageGenerationForm')?.requestSubmit();
      }
    });
    syncModelOptions();
    syncChannelOptions();
  }

  function setChannels(nextChannels) {
    channels = Array.isArray(nextChannels) ? nextChannels.slice() : [];
    if (!initialized) return;
    syncModelOptions();
    syncChannelOptions();
  }

  function open() {
    syncModelOptions();
    syncChannelOptions();
  }

  return {
    buildRequestPayload,
    normalizeImages,
    dataURLFromImage,
    imageSizeOptions,
    init,
    setChannels,
    open
  };
});
