const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const elements = new Map();
function element(id) {
  if (!elements.has(id)) elements.set(id, {id, textContent:'', innerHTML:'', value:'', disabled:false, dataset:{}, classList:{add(){}, remove(){}, toggle(){}}, addEventListener(){}});
  return elements.get(id);
}
const context = {
  document:{getElementById:element, querySelectorAll:() => []},
  window:{chrome:{webview:{addEventListener(){}, postMessage(){}}}},
  setTimeout(){}, clearTimeout(){}, setInterval(){}, console
};
vm.createContext(context);
vm.runInContext(fs.readFileSync('windows/desktop/wwwroot/app.js','utf8'), context);
context.render({configured:true, worker:{maxActiveJobs:0,maxEncodes:1,activeEncodes:1,jobs:[{jobId:'1',name:'<img src=x onerror=alert(1)>',phase:'FFMPEG',state:'running',downloaded:true}],progress:[{jobId:'1',phase:'FFMPEG',percent:25}]},queue:{total:1,jobs:[{name:'<script>evil</script>'}]}});
assert.match(element('toggle-pause').textContent, /Reanudar/, 'pause follows admission, even if accepted work can still encode');
assert.equal(element('stat-encoding').textContent, 1);
assert.ok(!element('job-list').innerHTML.includes('<img'), 'job titles must be escaped');
assert.ok(!element('server-queue').innerHTML.includes('<script>'), 'server titles must be escaped');
assert.match(element('job-list').innerHTML, /25%/);
context.render({configured:true,worker:{maxActiveJobs:2,maxEncodes:2,activeEncodes:0,jobs:[{jobId:'2',name:'film.mkv',phase:'FFMPEG',state:'queued',downloaded:true}],progress:[]},queueError:'unsupported'});
assert.match(element('toggle-pause').textContent, /Pausar/);
assert.equal(element('stat-ready').textContent, 1);
assert.match(element('server-queue').innerHTML, /Actualiza el servidor/);
console.log('Desktop dashboard: admission state, prefetched work, old server fallback and escaping passed.');
