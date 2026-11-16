'use strict';
const TranscoderReleaseNotes = (() => {
  const endpoint = 'https://api.github.com/repos/segator/transcoderd/releases?per_page=100&page=';
  function version(tag) {
    const match = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(tag || '');
    if (!match) return null;
    const parts = match.slice(1).map(Number);
    return parts.every(Number.isSafeInteger) ? parts : null;
  }
  function compare(a, b) {
    for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
    return 0;
  }
  async function load(installedTag, targetTag, pending, fetcher = fetch) {
    const installed = version(installedTag), target = pending ? version(targetTag) : installed;
    if (!installed || !target || (pending && compare(target, installed) <= 0)) return {releases:[], warning:'Release notes are unavailable for the reported version range.'};
    const all = [];
    let complete = false, failed = false;
    for (let page = 1; page <= 3; page++) {
      try {
        const response = await fetcher(endpoint + page, {credentials:'omit', referrerPolicy:'no-referrer', signal:AbortSignal.timeout(10000)});
        if (!response.ok) throw new Error('Release request failed');
        const releases = await response.json();
        if (!Array.isArray(releases)) throw new Error('Invalid release response');
        all.push(...releases);
        if (releases.length < 100) { complete = true; break; }
      } catch { failed = true; break; }
    }
    if (!all.length && failed) throw new Error('Release notes unavailable');
    const stable = all.filter(r => r && !r.draft && !r.prerelease && version(r.tag_name));
    const selected = stable.filter(r => {
      const v = version(r.tag_name);
      return pending ? compare(v, installed) > 0 && compare(v, target) <= 0 : compare(v, installed) === 0;
    }).sort((a,b) => compare(version(b.tag_name), version(a.tag_name)));
    const hasInstalled = stable.some(r => compare(version(r.tag_name), installed) === 0);
    const hasTarget = stable.some(r => compare(version(r.tag_name), target) === 0);
    let warning = '';
    if (!hasTarget) warning = 'Release notes for the selected version are unavailable; the list may be incomplete.';
    else if (pending && (!hasInstalled || !complete || failed)) warning = 'Some intermediate release notes may be missing.';
    return {releases:selected, warning};
  }
  return {load};
})();
if (typeof module !== 'undefined') module.exports = TranscoderReleaseNotes;
