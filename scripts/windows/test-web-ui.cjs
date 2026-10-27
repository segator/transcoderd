const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const assetRoot = 'worker/dashboard/wwwroot';
assert.match(fs.readFileSync(`${assetRoot}/index.html`, 'utf8'), /<html lang="en">/, 'the dashboard language must be English');
const elements = new Map();
function element(id) {
  if (!elements.has(id)) elements.set(id, {id, textContent:'', innerHTML:'', value:'', disabled:false, dataset:{},style:{},handlers:{},classList:{add(){}, remove(){}, toggle(){}}, addEventListener(name,callback){this.handlers[name]=callback;}});
  return elements.get(id);
}
const filterButtons = ['all','encoding','downloading','uploading','preparation'].map(filter => ({
  dataset:{filter}, classList:{add(){}, remove(){}, toggle(){}},
  addEventListener(_name, callback) { this.callback = callback; },
  click() { this.callback?.(); }
}));
const saved = new Map();
const storage = {getItem:key=>saved.get(key)??null,setItem:(key,value)=>saved.set(key,value),removeItem:key=>saved.delete(key)};
const context = {
  document:{body:{classList:{toggle(){}}},getElementById:element,querySelectorAll:selector => selector === '[data-filter]' ? filterButtons : []},
  window:{chrome:{webview:{addEventListener(){},postMessage(){}}}},
  localStorage:storage,setTimeout(){}, clearTimeout(){}, setInterval(){}, console
};
vm.createContext(context);
vm.runInContext(fs.readFileSync(`${assetRoot}/activity.js`, 'utf8'), context);
vm.runInContext(fs.readFileSync(`${assetRoot}/app.js`, 'utf8'), context);

context.render({version:'2.8.2-test',configured:true,worker:{maxActiveJobs:0,maxEncodes:1,activeEncodes:1,jobs:[
  {jobId:'1',name:'<img src=x onerror=alert(1)>',phase:'FFMPEG',state:'running',downloaded:true,profile:'gpu',codec:'av1_nvenc'},
  {jobId:'2',name:'film.mkv',phase:'Download',state:'running'},
  {jobId:'3',name:'encoded.mkv',phase:'Upload',state:'running'}
],progress:[
  {jobId:'1',phase:'FFMPEG',percent:25,profile:'gpu',codec:'av1_nvenc'},
  {jobId:'2',phase:'Download',percent:75,value:1.5*1024**3,total:2*1024**3,bytesPerSecond:20*1024**2},
  {jobId:'3',phase:'Upload',percent:50,value:550*1024**2,total:3*1024**3}
]},queue:{total:1,jobs:[{name:'<script>evil</script>'}]}});
assert.match(element('toggle-pause').textContent, /Resume/, 'pause follows admission, even if accepted work can still encode');
assert.match(element('connection').innerHTML, /Paused/);
assert.match(element('notice-text').textContent, /assignments are paused/);
assert.equal(element('stat-encoding').textContent, 1);
assert.ok(!element('job-list').innerHTML.includes('<img'), 'job titles must be escaped');
assert.ok(!element('server-queue').innerHTML.includes('<script>'), 'server titles must be escaped');
assert.match(element('job-list').innerHTML, /25%/);
assert.match(element('app-version').textContent, /2\.8\.2-test/);
assert.match(element('job-list').innerHTML, /GPU · NVENC · AV1/);
assert.match(element('job-list').innerHTML, /1\.5 GB of 2\.0 GB · 20\.0 MB\/s/, 'download progress should use MB/GB units');
assert.match(element('job-list').innerHTML, /550\.0 MB of 3\.0 GB/, 'upload progress should use MB/GB units');

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
context.render({configured:true,update:{pending:true,phase:'downloading',targetVersion:'v99.1.0',downloaded:5*1024**2,total:10*1024**2},worker:{activeJobs:2,reservedJobs:1,jobs:[],progress:[]}});
assert.equal(element('update-alert').hidden,false,'mandatory update alert must be visible');
assert.match(element('update-title').textContent,/v99\.1\.0/);
assert.equal(element('settings-controls').disabled,true,'all configuration must be locked');
assert.equal(element('save').disabled,true);
assert.equal(element('show-token').disabled,true);
assert.equal(element('toggle-pause').disabled,true);
assert.equal(element('update-progress').style.width,'50%');
assert.match(element('update-detail').textContent,/2 accepted jobs/);
assert.ok(saved.has('transcoderd.pending-update'),'update lock must survive a page reload');
(async()=>{
  await assert.rejects(context.request('configure',{settings:{}}),/locked/);
  await assert.rejects(context.request('set-max-jobs',{cpuJobs:9}),/locked/);
  await element('settings-form').handlers.submit({preventDefault(){}});
  assert.equal(element('save').disabled,true,'keyboard submission must not unlock the form');
  context.renderUpdate();context.lockConfiguration(true);
  assert.equal(element('update-alert').hidden,false,'disconnect must retain pending update alert');
  elements.clear();
  const reloaded = {...context};
  vm.createContext(reloaded);
  vm.runInContext(fs.readFileSync(`${assetRoot}/activity.js`, 'utf8'), reloaded);
  vm.runInContext(fs.readFileSync(`${assetRoot}/app.js`, 'utf8'), reloaded);
  assert.equal(element('update-alert').hidden,false,'reload must display the cached pending alert before service status arrives');
  assert.equal(element('settings-controls').disabled,true,'reload must start with configuration locked');
  await assert.rejects(reloaded.request('configure',{settings:{}}),/locked/);
  context.render({configured:true,version:'99.1.0',update:{pending:false,phase:'updated'},worker:{jobs:[],progress:[]}});
  assert.equal(element('settings-controls').disabled,false,'verified service status releases the lock');
  assert.equal(saved.has('transcoderd.pending-update'),false);
  console.log('Dashboard UI: version, encoders, transfer units, filters, mandatory update alert and configuration lock passed.');
})().catch(error=>{console.error(error);process.exitCode=1;});
