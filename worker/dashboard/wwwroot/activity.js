(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.TranscoderDashboard = api;
})(globalThis, function () {
  'use strict';
  function activityCategory(job) {
    if (job.state === 'queued') return 'prefetch';
    if (job.phase === 'FFMPEG') return 'encoding';
    if (job.phase === 'Download') return 'downloading';
    if (job.phase === 'Upload') return 'uploading';
    return 'preparation';
  }
  function filterJobs(jobs, selected) {
    if (!selected || selected === 'all') return jobs;
    return jobs.filter(job => selected === activityCategory(job));
  }
  return {activityCategory, filterJobs};
});
