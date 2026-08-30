const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'channels-state.js'), 'utf8');
const sandbox = {
	localStorage: {
		getItem() { return null; }
	},
	window: {
    t(key, values) {
      return `${key}:${JSON.stringify(values)}`;
    }
  },
  console
};
vm.createContext(sandbox);
vm.runInContext(`${source}\nthis.testHumanizeMS = humanizeMS;`, sandbox);

test('cooldowns at or above 48 hours use days and hours', () => {
  assert.equal(
    sandbox.testHumanizeMS((3 * 24 + 5) * 60 * 60 * 1000),
    'common.timeDH:{"d":3,"h":5}'
  );
});

test('shorter cooldowns retain hours and minutes', () => {
  assert.equal(
    sandbox.testHumanizeMS((47 * 60 + 15) * 60 * 1000),
    'common.timeHM:{"h":47,"m":15}'
  );
});
