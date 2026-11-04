const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/dashboard/sw.js','utf8');
function worker(overrides={}) {
 const listeners={},deleted=[],precached=[];
 const cache={addAll:async reqs=>precached.push(...reqs),match:async()=>new Response('offline')};
 const context={URL,Request,Response,Promise,Error,
  self:{location:{origin:'https://example.test'},addEventListener:(name,fn)=>listeners[name]=fn,skipWaiting:async()=>{},clients:{claim:async()=>{}}},
  caches:{match:async()=>new Response("offline"),open:async()=>cache,keys:async()=>['other-app-cache','transcoderd-dashboard-public-old','transcoderd-dashboard-public-test'],delete:async key=>deleted.push(key)},
  fetch:async()=>{throw new TypeError('offline')},...overrides};
 // Native Request requires absolute URLs in Node; browser Request resolves relative paths.
 context.Request=class extends Request {constructor(path,options){super(new URL(path,'https://example.test'),options)}};
 vm.runInNewContext(source.replace('__DASHBOARD_CACHE_VERSION__','test').replace('__DASHBOARD_PUBLIC_PATHS__',JSON.stringify(['/dashboard/offline.html','/dashboard/styles.css'])),context);
 return {listeners,deleted,precached};
}
function event(path,options={}) {
 return {request:{method:'GET',url:'https://example.test'+path,mode:'cors',headers:new Headers(),...options},respondWith(promise){this.response=promise}};
}
test('install caches only explicit public assets without credentials',async()=>{
 const w=worker();let done;w.listeners.install({waitUntil:p=>done=p});await done;
 assert.deepEqual(w.precached.map(r=>new URL(r.url).pathname),['/dashboard/offline.html','/dashboard/styles.css']);assert.ok(w.precached.every(r=>r.credentials==='omit'));
});
test('activation removes only older dashboard caches',async()=>{
 const w=worker();let done;w.listeners.activate({waitUntil:p=>done=p});await done;assert.deepEqual(w.deleted,['transcoderd-dashboard-public-old']);
});
test('private APIs, credentials, mutations, videos and unrelated origins bypass interception',()=>{
 const w=worker();const tests=[
  event('/dashboard/api/stats'),event('/dashboard/api/media/file/video'),event('/api/v1/worker/settings'),
  event('/dashboard/auth/login',{mode:'navigate'}),event('/dashboard/auth/callback?code=private&state=private',{mode:'navigate'}),
  event('/dashboard/api/login',{method:'POST'}),event('/dashboard/styles.css',{method:'HEAD'}),
  event('/dashboard/styles.css?token=private'),event('/dashboard/styles.css',{headers:new Headers({Authorization:'Bearer private'})}),
  event('/dashboard/styles.css',{url:'https://another.test/dashboard/styles.css'}),event('/private-file.mkv')];
 for(const e of tests){w.listeners.fetch(e);assert.equal(e.response,undefined,e.request.url)}
});
test('offline navigation returns only the generic reconnect page',async()=>{
 const w=worker();const e=event('/dashboard/',{mode:'navigate'});w.listeners.fetch(e);assert.equal(await(await e.response).text(),'offline');
});
test('online authorization errors stay authoritative and are never replaced by offline content',async()=>{
 const w=worker({fetch:async()=>new Response('login required',{status:401})});const e=event('/dashboard/',{mode:'navigate'});w.listeners.fetch(e);assert.equal((await e.response).status,401);
});
test('public assets fall back offline without writing any runtime responses',async()=>{
 const w=worker();const e=event('/dashboard/styles.css');w.listeners.fetch(e);assert.equal(await(await e.response).text(),'offline');assert.equal(w.precached.length,0);
});

test('runtime fallbacks cannot recreate a deleted previous-version cache',async()=>{
 const w=worker({caches:{open:async()=>{throw new Error('runtime cache creation')},match:async()=>new Response('offline')}});
 for(const e of [event('/dashboard/',{mode:'navigate'}),event('/dashboard/styles.css')]){w.listeners.fetch(e);assert.equal(await(await e.response).text(),'offline')}
});

test('update UI ignores the initial controller and offers reload only for a replacement',()=>{
 const events={},swEvents={},buttons={};let reloads=0;
 for(const id of ['pwa-install','pwa-update'])buttons[id]={hidden:true,addEventListener:(name,fn)=>events[id+':'+name]=fn};
 const initial={scriptURL:'/dashboard/sw.js'};
 const navigator={serviceWorker:{controller:initial,addEventListener:(name,fn)=>swEvents[name]=fn,register:async()=>{}}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/dashboard/pwa.js','utf8'),{
  document:{getElementById:id=>buttons[id],readyState:'complete'},navigator,
  window:{isSecureContext:true,addEventListener:(name,fn)=>events[name]=fn},
  matchMedia:()=>({matches:false}),location:{reload:()=>reloads++},Boolean,
 });
 swEvents.controllerchange();assert.equal(buttons['pwa-update'].hidden,true);
 navigator.serviceWorker.controller={scriptURL:'/dashboard/sw.js'};swEvents.controllerchange();
 assert.equal(buttons['pwa-update'].hidden,false);assert.equal(reloads,0);
 events['pwa-update:click']();assert.equal(reloads,1);
});
