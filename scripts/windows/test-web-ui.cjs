const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const assetRoot = 'worker/dashboard/wwwroot';
assert.match(fs.readFileSync(`${assetRoot}/index.html`, 'utf8'), /<html lang="en">/, 'the dashboard language must be English');
const elements = new Map();
function element(id) {
  if (!elements.has(id)) elements.set(id, {id, textContent:'', innerHTML:'', value:'', disabled:false, dataset:{}, classList:{add(){}, remove(){}, toggle(){}}, addEventListener(){}});
  return elements.get(id);
}
const filterButtons = ['all','encoding','downloading','uploading','preparation'].map(filter => ({
  dataset:{filter}, classList:{add(){}, remove(){}, toggle(){}},
  addEventListener(_name, callback) { this.callback = callback; },
  click() { this.callback?.(); }
}));
const context = {
  document:{body:{classList:{toggle(){}}},getElementById:element,querySelectorAll:selector => selector === '[data-filter]' ? filterButtons : []},
  window:{chrome:{webview:{addEventListener(){},postMessage(){}}}},
  setTimeout(){}, clearTimeout(){}, setInterval(){}, console
};
vm.createContext(context);
vm.runInContext(fs.readFileSync(`${assetRoot}/activity.js`, 'utf8'), context);
vm.runInContext(fs.readFileSync(`${assetRoot}/app.js`, 'utf8'), context);

context.render({version:'2.8.2-test',configured:true,worker:{maxActiveJobs:0,maxEncodes:1,activeEncodes:1,jobs:[
  {jobId:'1',name:'<img src=x onerror=alert(1)>',phase:'FFMPEG',state:'running',downloaded:true,profile:'gpu',codec:'av1_nvenc'},
  {jobId:'2',name:'film.mkv',phase:'Download',state:'running'}
],progress:[{jobId:'1',phase:'FFMPEG',percent:25,profile:'gpu',codec:'av1_nvenc'}]},queue:{total:1,jobs:[{name:'<script>evil</script>'}]}});
assert.match(element('toggle-pause').textContent, /Resume/, 'pause follows admission, even if accepted work can still encode');
assert.match(element('connection').innerHTML, /Paused/);
assert.match(element('notice-text').textContent, /assignments are paused/);
assert.equal(element('stat-encoding').textContent, 1);
assert.ok(!element('job-list').innerHTML.includes('<img'), 'job titles must be escaped');
assert.ok(!element('server-queue').innerHTML.includes('<script>'), 'server titles must be escaped');
assert.match(element('job-list').innerHTML, /25%/);
assert.match(element('app-version').textContent, /2\.8\.2-test/);
assert.match(element('job-list').innerHTML, /GPU · NVENC · AV1/);

filterButtons[1].click();
assert.ok(!element('job-list').innerHTML.includes('film.mkv'), 'encoding filter should hide downloads');
filterButtons[2].click();
assert.ok(element('job-list').innerHTML.includes('film.mkv'), 'multiple activity filters should combine');
filterButtons[0].click();
assert.ok(element('job-list').innerHTML.includes('film.mkv'), 'all activity should clear the filters');

context.render({configured:true,worker:{maxActiveJobs:2,maxEncodes:2,activeEncodes:0,jobs:[{jobId:'3',name:'ready.mkv',phase:'FFMPEG',state:'queued',downloaded:true}],progress:[]},queueError:'unsupported'});
assert.match(element('toggle-pause').textContent, /Pause/);
assert.equal(element('stat-ready').textContent, 1);
assert.match(element('server-queue').innerHTML, /Update the server/);
context.render({configured:true,worker:{maxActiveJobs:2,maxEncodes:2,activeEncodes:0,withinSchedule:false,jobs:[],progress:[]}});
assert.match(element('connection').innerHTML, /Outside schedule/);
assert.match(element('notice-text').textContent, /Outside the work schedule/);
context.render({configured:true,worker:{maxActiveJobs:2,maxEncodes:2,activeEncodes:0,withinSchedule:true,accepting:false,jobs:[],progress:[]}});
assert.match(element('connection').innerHTML, /Waiting for settings/);
context.render({configured:true,worker:{maxActiveJobs:2,maxEncodes:2,activeEncodes:2,activeJobs:3,reservedJobs:1,maxAcceptedJobs:4,withinSchedule:true,accepting:true,jobs:[],progress:[]}});
assert.match(element('connection').innerHTML, /At capacity/);
console.log('Dashboard UI: version, encoder identity, filters, admission state and escaping passed.');
