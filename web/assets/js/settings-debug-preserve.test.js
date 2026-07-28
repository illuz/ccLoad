const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const source = fs.readFileSync(path.join(__dirname, 'settings.js'), 'utf8');

test('Debug preserve setting uses an auth-token select with masked labels', () => {
  assert.match(source, /fetchDataWithAuth\('\/admin\/auth-tokens'\)/);
  assert.match(source, /setting\.value_type === 'auth_token_id'/);
  assert.match(source, /<select id=/);
  assert.match(source, /value\.slice\(0, 6\).*value\.slice\(-4\)/s);
  assert.match(source, /input, textarea, select/);
});
