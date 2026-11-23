'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const {activityCategory, filterJobs} = require('./wwwroot/activity.js');

test('classifies queued jobs separately from running phases', () => {
  assert.equal(activityCategory({phase:'FFMPEG', state:'queued'}), 'prefetch');
  assert.equal(activityCategory({phase:'Download', state:'queued'}), 'prefetch');
  assert.equal(activityCategory({phase:'FFMPEG', state:'running'}), 'encoding');
  assert.equal(activityCategory({phase:'FFMPEG', state:'paused'}), 'encoding');
  assert.equal(activityCategory({phase:'Download', state:'running'}), 'downloading');
  assert.equal(activityCategory({phase:'Upload', state:'running'}), 'uploading');
  assert.equal(activityCategory({phase:'PGS', state:'running'}), 'preparation');
});

test('shows one selected category or all activity', () => {
  const jobs = [{phase:'FFMPEG', state:'running'}, {phase:'FFMPEG', state:'queued'}, {phase:'Download', state:'running'}];
  assert.deepEqual(filterJobs(jobs, 'all'), jobs);
  assert.deepEqual(filterJobs(jobs, 'encoding'), [jobs[0]]);
  assert.deepEqual(filterJobs(jobs, 'prefetch'), [jobs[1]]);
  assert.deepEqual(filterJobs(jobs, 'downloading'), [jobs[2]]);
});
