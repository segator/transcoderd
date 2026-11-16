'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const {load} = require('./wwwroot/release-notes.js');
const release = (tag, extra = {}) => ({tag_name:tag, body:`notes for ${tag}`, ...extra});
const response = values => async () => ({ok:true, json:async () => values});

test('shows all stable releases in the pending range in semantic order', async () => {
  const got = await load('v2.8.0', 'v2.10.0', true, response([release('v2.10.0'), release('v2.9.0'), release('v2.8.0'), release('v2.11.0'), release('v2.9.1-rc1', {prerelease:true}), release('v2.9.2', {draft:true})]));
  assert.deepEqual(got.releases.map(r => r.tag_name), ['v2.10.0','v2.9.0']);
  assert.equal(got.warning, '');
});
test('shows the installed version when there is no pending update', async () => {
  const got = await load('2.8.0', '', false, response([release('v2.9.0'), release('v2.8.0')]));
  assert.deepEqual(got.releases.map(r => r.tag_name), ['v2.8.0']);
});
test('labels missing targets and incomplete history without inventing a target', async () => {
  assert.match((await load('v2.8.0', 'v2.10.0', true, response([release('v2.9.0')]))).warning, /unavailable/);
  assert.match((await load('v2.8.0', 'v2.10.0', true, response([release('v2.10.0')]))).warning, /missing/);
  assert.match((await load('development', '', false, response([]))).warning, /unavailable/);
});
test('loads later pages and reports failures', async () => {
  let calls = 0;
  const first = Array.from({length:100}, (_,i) => release(`v3.0.${i}`));
  const got = await load('v2.8.0', 'v3.0.99', true, async () => ({ok:true, json:async () => ++calls === 1 ? first : [release('v2.8.0')]}));
  assert.equal(calls, 2); assert.equal(got.releases.length, 100); assert.equal(got.warning, '');
  await assert.rejects(load('v2.8.0', '', false, async () => { throw new Error('offline'); }));
});
