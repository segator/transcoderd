'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const {activityCategory, filterJobs} = require('./wwwroot/activity.js');

test('classifies local worker phases into dashboard filters', () => {
  assert.equal(activityCategory({phase:'FFMPEG'}), 'encoding');
  assert.equal(activityCategory({phase:'Download'}), 'downloading');
  assert.equal(activityCategory({phase:'Upload'}), 'uploading');
  assert.equal(activityCategory({phase:'PGS'}), 'preparation');
  assert.equal(activityCategory({phase:'MKVExtract'}), 'preparation');
});

test('combines selected activity filters and leaves the all filter unfiltered', () => {
  const jobs = [{phase:'FFMPEG'}, {phase:'Download'}, {phase:'Upload'}, {phase:'PGS'}];
  assert.equal(filterJobs(jobs, new Set()).length, 4);
  assert.deepEqual(filterJobs(jobs, new Set(['encoding','downloading'])), jobs.slice(0, 2));
  assert.deepEqual(filterJobs(jobs, new Set(['uploading'])), [jobs[2]]);
});
