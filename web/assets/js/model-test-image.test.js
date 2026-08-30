const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const {
  buildRequestPayload,
  normalizeImages,
  dataURLFromImage,
  imageSizeOptions
} = require('./model-test-image.js');

test('image generation payload follows the selected API contract', () => {
  assert.deepEqual(buildRequestPayload({
    generationAPI: 'chat_completions',
    model: ' gemini-image ',
    prompt: ' draw a lighthouse ',
    keyIndex: '3',
    size: '3:2@2k',
    quality: 'high',
    background: 'transparent',
    outputFormat: 'webp'
  }), {
    generation_api: 'chat_completions',
    model: 'gemini-image',
    prompt: 'draw a lighthouse',
    key_index: 3,
    size: '3:2@2k'
  });

  assert.deepEqual(buildRequestPayload({
    generationAPI: 'images',
    model: 'gpt-image-2',
    prompt: 'draw a lighthouse',
    keyIndex: '0',
    size: 'auto',
    quality: 'high',
    background: 'auto',
    outputFormat: 'webp'
  }), {
    generation_api: 'images',
    model: 'gpt-image-2',
    prompt: 'draw a lighthouse',
    key_index: 0,
    quality: 'high',
    output_format: 'webp'
  });
});

test('image generation exposes API-specific size options', () => {
  assert.ok(imageSizeOptions('images').some(([value]) => value === '512x512'));
  assert.ok(!imageSizeOptions('chat_completions').some(([value]) => value === '512x512'));
  assert.ok(imageSizeOptions('chat_completions').some(([value]) => value === '3:2@2k'));
});

test('image response accepts HTTP URLs and base64 while rejecting unsafe URLs', () => {
  const images = normalizeImages({
    images: [
      { url: 'https://example.com/image.png' },
      { b64_json: 'aW1hZ2U=' },
      { url: 'javascript:alert(1)' },
      { revised_prompt: 'missing image data' }
    ]
  });

  assert.equal(images.length, 2);
  assert.equal(dataURLFromImage(images[0], 'png'), 'https://example.com/image.png');
  assert.equal(dataURLFromImage(images[1], 'jpeg'), 'data:image/jpeg;base64,aW1hZ2U=');
  assert.equal(
    dataURLFromImage({ b64_json: 'aW1hZ2U=', mime_type: 'image/webp' }, 'png'),
    'data:image/webp;base64,aW1hZ2U='
  );
});

test('image controls use shared comboboxes and persist the prompt immediately', () => {
  const elements = new Map();
  const makeElement = () => {
    const listeners = new Map();
    return {
      value: '',
      disabled: false,
      placeholder: '',
      classList: { toggle() {} },
      setAttribute() {},
      addEventListener(type, listener) { listeners.set(type, listener); },
      dispatch(type, event = {}) { listeners.get(type)?.(event); },
      focus() {},
      querySelector() { return null; }
    };
  };
  const document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, makeElement());
      return elements.get(id);
    }
  };
  const comboboxes = new Map();
  const storage = new Map([
    ['ccload_model_test_image_generation_api', 'images'],
    ['ccload_model_test_image_size_images', '1536x1024'],
    ['ccload_model_test_image_quality', 'hd'],
    ['ccload_model_test_image_background', 'transparent'],
    ['ccload_model_test_image_output_format', 'webp'],
    ['ccload_model_test_image_prompt', 'persisted prompt']
  ]);
  const fakeWindow = {
    document,
    localStorage: {
      getItem: key => storage.has(key) ? storage.get(key) : null,
      setItem: (key, value) => storage.set(key, String(value))
    },
    t: () => '',
    createSearchableCombobox(config) {
      let value = config.initialValue || '';
      const input = document.getElementById(config.inputId);
      const instance = {
        config,
        getValue: () => value,
        setValue(nextValue, label) {
          value = nextValue;
          input.value = label;
        },
        refresh() {},
        getInput: () => input,
        select(nextValue) {
          value = nextValue;
          config.onSelect?.(nextValue, nextValue);
        }
      };
      comboboxes.set(config.inputId, instance);
      return instance;
    }
  };

  const modulePath = require.resolve('./model-test-image.js');
  const previousWindow = global.window;
  delete require.cache[modulePath];
  global.window = fakeWindow;
  try {
    const freshModule = require(modulePath);
    freshModule.init({
      getModelOptions: () => ['gpt-image-2'],
      getChannelsForModel: () => [{ id: 7, name: 'Images' }]
    });

    for (const id of [
      'imageGenerationAPISelect',
      'imageSizeSelect',
      'imageQualitySelect',
      'imageBackgroundSelect',
      'imageOutputFormatSelect'
    ]) {
      assert.ok(comboboxes.has(id), `${id} must use the shared searchable combobox`);
    }
    assert.equal(comboboxes.get('imageSizeSelect').getValue(), '1536x1024');
    assert.equal(comboboxes.get('imageQualitySelect').getValue(), 'hd');
    assert.equal(comboboxes.get('imageBackgroundSelect').getValue(), 'transparent');
    assert.equal(comboboxes.get('imageOutputFormatSelect').getValue(), 'webp');
    assert.equal(elements.get('imagePrompt').value, 'persisted prompt');

    elements.get('imagePrompt').value = 'updated prompt';
    elements.get('imagePrompt').dispatch('input');
    assert.equal(storage.get('ccload_model_test_image_prompt'), 'updated prompt');

    comboboxes.get('imageGenerationAPISelect').select('chat_completions');
    const chatSizes = comboboxes.get('imageSizeSelect').config.getOptions().map(option => option.value);
    assert.ok(chatSizes.includes('3:2@2k'));
    assert.ok(!chatSizes.includes('512x512'));
    assert.equal(elements.get('imageQualitySelect').disabled, true);

    comboboxes.get('imageGenerationAPISelect').select('images');
    const imageSizes = comboboxes.get('imageSizeSelect').config.getOptions().map(option => option.value);
    assert.ok(imageSizes.includes('512x512'));
    assert.equal(elements.get('imageQualitySelect').disabled, false);
  } finally {
    delete require.cache[modulePath];
    if (previousWindow === undefined) delete global.window;
    else global.window = previousWindow;
  }
});

test('model test page wires the image panel without OAuth-specific branches', () => {
  const html = fs.readFileSync(path.join(__dirname, '..', '..', 'model-test.html'), 'utf8');
  const mainSource = fs.readFileSync(path.join(__dirname, 'model-test.js'), 'utf8');
  const imageSource = fs.readFileSync(path.join(__dirname, 'model-test-image.js'), 'utf8');

  assert.match(html, /model-test-image\.js[\s\S]*?model-test\.js/);
  assert.match(html, /id="modeTabImage"[\s\S]*?id="imagePanel"/);
  assert.match(html, /id="imageModelInput"[\s\S]*?id="imageChannelSelect"[\s\S]*?id="imageKeySelect"/);
  assert.match(mainSource, /const TEST_MODE_IMAGE = 'image'/);
  assert.match(mainSource, /configuredModel === '\*' \|\| configuredModel === modelName/);
  assert.doesNotMatch(imageSource, /auth_type|xai_oauth|codex|oauth/i);
});
