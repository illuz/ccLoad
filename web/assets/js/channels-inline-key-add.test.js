const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const stateSource = fs.readFileSync(path.join(__dirname, 'channels-state.js'), 'utf8');
const keysSource = fs.readFileSync(path.join(__dirname, 'channels-keys.js'), 'utf8');

function createHarness() {
  const sandbox = {
    console,
    document: {
      getElementById() {
        return null;
      },
      querySelectorAll() {
        return [];
      }
    },
    localStorage: {
      getItem() {
        return null;
      }
    },
    setTimeout(callback) {
      callback();
      return 1;
    },
    window: {
      t(key) {
        return key;
      }
    }
  };

  vm.createContext(sandbox);
  vm.runInContext(`${stateSource}
${keysSource}
let __renderCalls = 0;
renderInlineKeyTable = () => {
  __renderCalls += 1;
};
this.__inlineKeyAddTest = {
  addInlineKey,
  getInlineKeys() {
    return inlineKeyTableData.map(row => ({ ...row }));
  },
  getRenderCalls() {
    return __renderCalls;
  },
  setInlineKeys(value) {
    inlineKeyTableData = value;
  }
};`, sandbox);

  return sandbox.__inlineKeyAddTest;
}

function getInlineKeys(api) {
  return JSON.parse(JSON.stringify(api.getInlineKeys()));
}

test('新增 API Key 时支持带备注的行对象', () => {
  const api = createHarness();
  api.setInlineKeys([{ api_key: 'sk-test', note: 'primary' }]);

  api.addInlineKey();

  assert.deepEqual(getInlineKeys(api), [
    { api_key: 'sk-test', note: 'primary' },
    { api_key: '', note: '' }
  ]);
  assert.equal(api.getRenderCalls(), 1);
});

test('最后一行为空时不重复新增 API Key 行', () => {
  const api = createHarness();
  api.setInlineKeys([{ api_key: '', note: '' }]);

  api.addInlineKey();

  assert.deepEqual(getInlineKeys(api), [{ api_key: '', note: '' }]);
  assert.equal(api.getRenderCalls(), 0);
});
