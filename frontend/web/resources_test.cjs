const {test}=require('node:test');
const assert=require('node:assert/strict');
const r=require('./dashboard/resources.js');
const now=Date.parse('2026-11-04T12:00:00Z');
function worker(at=now){return {name:'node / ñ?<&',online:true,runtime:{resources:{sampledAt:new Date(at).toISOString(),cpu:{percent:0,cores:8,scope:'host'},memory:{usedBytes:4,totalBytes:8},gpus:[{uuid:'gpu-a',name:'Test GPU',percent:0,encoderPercent:85}]}}};}
test('zero, missing, invalid and multi-device values retain different meanings',()=>{
 const w=worker();assert.equal(r.percent(0),0);for(const v of [null,undefined,-1,101,NaN,Infinity,'3'])assert.equal(r.percent(v),null);
 assert.equal(r.memoryPercent({usedBytes:0,totalBytes:8}),0);assert.equal(r.memoryPercent({usedBytes:10,totalBytes:8}),null);
 const html=r.summary(w,now);assert.match(html,/CPU <b>0%/);assert.match(html,/NVENC <b>85%/);
 w.runtime.resources.gpus.push({uuid:'gpu-b',percent:70,encoderPercent:null});assert.equal(r.busiest(w.runtime.resources.gpus,'percent'),70);assert.match(r.summary(w,now),/GPU max <b>70%/);
 w.runtime.resources.cpu.percent=null;assert.match(r.summary(w,now),/CPU <b>—/);
 assert.equal(new URLSearchParams(r.link(w.name).split('?')[1]).get('worker'),w.name);
});
test('freshness is separate from presence and stale numbers are not live summaries',()=>{
 const w=worker(now-21000);assert.equal(r.state(w,now).live,false);assert.match(r.summary(w,now),/Last reported/);assert.doesNotMatch(r.summary(w,now),/85%/);
 w.runtime.resources.sampledAt=new Date(now).toISOString();w.online=false;assert.equal(r.state(w,now).live,false);
 assert.match(r.summary({name:'old',online:true,runtime:{}},now),/No hardware telemetry/);
 w.online=true;w.runtime.resources.sampledAt=new Date(now+60000).toISOString();assert.equal(r.state(w,now).label,'Sample clock mismatch');
});
test('history deduplicates, bounds, expires and keeps missing readings as gaps',()=>{
 const h=new r.History(),w=worker();h.observe([w],now);h.observe([w],now+1000);assert.equal(h.get(w.name).length,1);
 for(let i=1;i<=140;i++){w.runtime.resources.sampledAt=new Date(now+i*5000).toISOString();w.runtime.resources.cpu.percent=i===140?null:25;h.observe([w],now+i*5000)}
 assert.equal(h.get(w.name).length,120);assert.equal(h.get(w.name).at(-1).cpu,null);assert.equal(h.get(w.name)[0].at,now+21*5000);
 h.observe([w],now+140*5000+601000);assert.equal(h.get(w.name).length,0);h.observe([],now);assert.equal(h.workers.size,0);
});
test('plots do not bridge unknown samples or fabricate data and escape device labels',()=>{
 const points=[{at:now,cpu:20},{at:now+5000,cpu:null},{at:now+10000,cpu:40}];
 const svg=r.chart(points,[{name:'CPU',value:p=>p.cpu}],'CPU history');const path=svg.match(/<path d="([^"]*)"/)[1];assert.equal((path.match(/M/g)||[]).length,2);assert.equal((path.match(/L/g)||[]).length,0);
 assert.match(r.chart([{at:now,cpu:null}],[{name:'CPU',value:p=>p.cpu}],'CPU'),/Waiting for resource samples/);
 const w=worker();w.runtime.resources.gpus[0].name='<img src=x onerror=alert(1)>';const h=new r.History();h.observe([w],now);const html=r.details(w,h,now);assert.ok(!html.includes('<img'));assert.ok(html.includes('&lt;img'));assert.ok(html.includes('NVENC'));
});
