const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('jsdom');
const front=path.resolve(process.argv[2]||path.join(__dirname,'../frontend/dist'));
const index=fs.readFileSync(path.join(front,'index.html'),'utf8');
const base=index.match(/src="(.*?\.js)"/)[1].replace(/^\.?\//,'');
const source=fs.readFileSync(path.join(front,base),'utf8');
const fixture=()=>{
 const C='C:\\Users\\Dev', ev=[
 {id:'node-version',tool:'Node.js',title:'版本',packageID:'OpenJS.NodeJS.LTS',value:''},
 {id:'go-version',tool:'Go',title:'版本',packageID:'GoLang.Go',value:'go1.24.11'},
 {id:'go-cache',tool:'Go',title:'构建缓存',variable:'GOCACHE',cleanup:'go clean -cache',value:C+'\\AppData\\Local\\go-build'},
 {id:'go-mod',tool:'Go',title:'模块缓存',variable:'GOMODCACHE',value:C+'\\go\\pkg\\mod'},
 {id:'go-temp',tool:'Go',title:'构建临时工作目录',variable:'GOTMPDIR',value:C+'\\AppData\\Local\\Temp'},
 {id:'go-path',tool:'Go',title:'工作区路径',variable:'GOPATH',value:C+'\\go'},
 {id:'go-file',tool:'Go',title:'Go 环境配置文件',value:C+'\\go-env'},
 {id:'npm-version',tool:'npm',title:'版本',value:'11.4.0'},
 {id:'npm-cache',tool:'npm',title:'下载缓存',variable:'NPM_CONFIG_CACHE',cleanup:'npm cache clean --force',value:C+'\\AppData\\Local\\npm-cache'},
 {id:'npm-prefix',tool:'npm',title:'全局前缀',variable:'NPM_CONFIG_PREFIX',value:'D:/Data/npm/prefix'},
 ...['store','state','cache','global'].map(key=>({id:'pnpm-'+key,tool:'pnpm',title:key+' 目录',variable:'NPM_CONFIG_'+key.toUpperCase()+'_DIR',value:'D:/Data/pnpm/'+key})),
 {id:'pnpm-version',tool:'pnpm',title:'版本',value:'10.11.0'},
 {id:'python-version',tool:'Python',title:'版本',packageID:'Python.Python.3.13',value:'3.13.4'},
 ];
 const calls=[];
 let updateFail=true;
 const plan=(token,summary,lines)=>({token,summary,lines,error:''});
 window.calls=calls;
 window.go={main:{App:{
 Catalog:async()=>ev.map(x=>({command:'go env '+(x.variable||''),variable:'',cleanup:'',description:'目录查询测试',docs:'https://go.dev',packageID:'',...x})),
 DriveStats:async()=>[{name:'C:',totalBytes:256e9,freeBytes:49e9}],
 Inspect:async(id,wd)=> {calls.push(['inspect',id,wd]);return {value:ev.find(x=>x.id===id).value,path:'',user:'',machine:'',inherited:'',sizeBytes:1e9,sizeStatus:'逻辑大小',error:''}},
 InspectPnpm:async()=>Object.fromEntries(await Promise.all(ev.filter(r=>r.tool==='pnpm').map(async r=>[r.id,await window.go.main.App.Inspect(r.id,'')]))),
 InspectGo:async()=>Object.fromEntries(await Promise.all(ev.filter(r=>r.tool==='Go').map(async r=>[r.id,await window.go.main.App.Inspect(r.id,'')]))),
 LatestVersion:async id=>({version:'1.25.3',channel:id==='go-version'?'Go 稳定版':'正式版',error:''}),
 BatchItems:async()=>['go-cache','go-mod'],
 PlanDirectories:async (root,ids,policy,batch)=>{
  calls.push([batch?'batchPlan':'singlePlan',root,[...ids],policy]);
  const dirs=ids.map(id=>({label:ev.find(r=>r.id===id).title,variable:ev.find(r=>r.id===id).variable,current:ev.find(r=>r.id===id).value,target:batch?root.replace(/[\\/]+$/,'')+'\\'+({'go-cache':'build','go-mod':'mod','go-temp':'temp','go-path':'workspace'})[id]:root}));
  return {...plan(batch?'batch-token':'single-token','确认设置后的目录',['旧数据处理：'+policy]),directories:dirs};
 },
 ApplyConfig:async token=>{calls.push(['apply',token]);if(token==='batch-token'&&updateFail){updateFail=false;return {error:'目标目录不再为空'}}return {message:'配置完成',error:''}},
 PlanConfig:async (id,dir)=>{calls.push(['singlePlan',id,dir]);return plan('single-token','改目录',[dir])},
 PlanMaintenance:async (id,verb)=>{calls.push(['packagePlan',id,verb]);return plan('package-token','运行 WinGet',[id,verb])},
 ApplyPackage:async token=>{calls.push(['packageApply',token]);return {message:'软件包操作完成',error:''}},
 ChooseDirectory:async()=> 'D:\\test\\go',
 GoRootRollbackAvailable:async()=>true,
 RollbackGoRoot:async()=>({message:'批量恢复完成',error:''}),
 }}};
 window.runtime={
 WindowSetSystemDefaultTheme:()=>calls.push(['theme','system']),
 WindowSetLightTheme:()=>calls.push(['theme','light']),
 WindowSetDarkTheme:()=>calls.push(['theme','dark']),
 };
};
async function run() {
 const dom=new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>',{url:'http://wails.localhost',runScripts:'outside-only',pretendToBeVisual:true,beforeParse(w){w.localStorage.setItem('workspace-theme','dark')}});
 const w=dom.window,d=w.document, calls=()=>JSON.parse(JSON.stringify(w.calls));
 w.eval('('+fixture.toString()+')()');w.eval('(()=>{'+source+'\n})()');
 const button=(txt,scope=d)=>Array.from(scope.querySelectorAll('button')).find(x=>x.textContent.trim()===txt||x.getAttribute('aria-label')===txt);
 const click=(txt,scope=d)=>{const b=button(txt,scope);assert(b,txt);assert(!b.disabled,txt+' disabled');b.focus();b.click()};
 const tick=()=>new Promise(r=>setTimeout(r,55));
 const dlg=()=>{const e=d.querySelector('[role=dialog]');assert(e);return e};
 const input=(el,value)=>{el.value=value;el.dispatchEvent(new w.Event('input',{bubbles:true}));};
 await tick();
 assert.equal(d.documentElement.dataset.theme,'dark');assert(calls().some(x=>x.join()==='theme,dark'));
 click('浅色');await tick();assert.equal(d.documentElement.dataset.theme,'light');assert.equal(w.localStorage.getItem('workspace-theme'),'light');
 click('跟随系统');await tick();assert(calls().some(x=>x.join()==='theme,system'));
 assert(!d.querySelector('header').textContent.includes('工作目录'));assert(d.body.textContent.includes('C 盘开发缓存'));
 const nav=(name)=>{const b=Array.from(d.querySelectorAll('aside .nav')).find(x=>x.textContent.trim().endsWith(name));assert(b,name);b.click()};
 nav('Go');await tick();assert(d.querySelector('.versionbar').textContent.includes('go1.24.11'));assert(d.querySelector('.versionbar').textContent.includes('1.25.3'));assert(button('更新'));assert(button('卸载'));
 click('批量改目录');await tick();assert(!d.querySelector('[role=dialog]'));assert(button('设置目录').disabled);
 const row=id=>Array.from(d.querySelectorAll('.row')).find(e=>e.querySelector('input')?.value===id);
 assert(row('go-file').querySelector('input').disabled);assert(row('go-file').textContent.includes('建议保留'));
 for(const id of ['go-mod','go-temp']){row(id).querySelector('input').click();await tick()}
 await tick();click('设置目录');await tick();
 let root=dlg().querySelector('[aria-label="目标目录"]');input(root,'D:/test/go/');await tick();
 assert(dlg().querySelector('input[value="migrate"]').checked);assert(dlg().querySelector('input[value="delete"]').disabled);
 click('确定',dlg());await tick();
 const batchCall=calls().find(x=>x[0]==='batchPlan');assert.equal(batchCall[1],'D:/test/go/');assert.deepEqual(batchCall[2].sort(),['go-mod','go-temp']);assert.equal(batchCall[3],'migrate');assert(!calls().find(x=>x[0]==='apply'));assert(dlg().textContent.includes('\\temp'));
 click('返回修改',dlg());await tick();assert.equal(dlg().querySelector('[aria-label="目标目录"]').value,'D:/test/go/');
 click('确定',dlg());await tick();click('确认执行',dlg());await tick();assert(dlg().textContent.includes('目标目录不再为空'));
 click('确定',dlg());await tick();click('确认执行',dlg());await tick();assert(d.body.textContent.includes('配置完成'));
 const workspace=[...d.querySelectorAll('.row')].find(e=>e.textContent.includes('工作区路径'));click('改目录',workspace);await tick();
 let target=dlg().querySelector('[aria-label="目标目录"]');assert.equal(d.activeElement,target);input(target,'F:\\golang\\workspace');dlg().querySelector('input[value="keep"]').click();await tick();click('确定',dlg());await tick();
 assert(calls().some(c=>c[0]==='singlePlan'&&c[2][0]==='go-path'&&c[3]==='keep'));click('取消',dlg());await tick();assert.equal(calls().filter(c=>c[0]==='apply').length,2);
 nav('Node.js');await tick();click('安装');await tick();assert(calls().some(c=>c[0]==='packagePlan'&&c[2]==='install'));assert(!calls().some(c=>c[0]==='packageApply'));click('取消',dlg());await tick();
 for(const name of ['Python','npm','pnpm']){nav(name);await tick();assert(button('更新'));assert(button('卸载'));}click('卸载');await tick();assert(calls().some(c=>c[0]==='packagePlan'&&c[1]==='pnpm-version'));click('取消',dlg());await tick();
 nav('npm');await tick();const prefix=[...d.querySelectorAll('.row')].find(x=>x.textContent.includes('全局前缀'));click('改目录',prefix);await tick();click('取消',dlg());await tick();
 nav('pnpm');await tick();assert.equal([...d.querySelectorAll('.row')].filter(row=>button('改目录',row)).length,4);click('批量改目录');await tick();
 for(const r of d.querySelectorAll('.row')){r.querySelector('input[type=checkbox]').click();await tick()}
 click('设置目录');await tick();input(dlg().querySelector('[aria-label="目标目录"]'),'E:/pnpm');await tick();click('确定',dlg());await tick();
 assert.deepEqual(calls().filter(c=>c[0]==='batchPlan').at(-1)[2].sort(),['pnpm-cache','pnpm-global','pnpm-state','pnpm-store']);click('取消',dlg());await tick();
 dom.window.close();console.log('PASS: production Vue DOM with mocked Windows bridge: theme persistence and native calls, navigation, version placement, package gate and plan, batch plan/application and retry, direct edit focus/cancel.');
}
run().catch(e=>{console.error(e);process.exit(1)})
