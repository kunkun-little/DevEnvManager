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
 {id:'go-path',tool:'Go',title:'工作区路径',value:C+'\\go'},
 {id:'npm-version',tool:'npm',title:'版本',value:'11.4.0'},
 {id:'npm-cache',tool:'npm',title:'下载缓存',variable:'NPM_CONFIG_CACHE',cleanup:'npm cache clean --force',value:C+'\\AppData\\Local\\npm-cache'},
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
 DetectPackage:async id=>{calls.push(['detect',id]);return {available:true,registered:id==='go-version',version:id==='go-version'?'1.24.11':'',evidence:'',error:''}},
 PlanGoRoot:async (root,ids)=>{calls.push(['batchPlan',root,[...ids]]);return plan('batch-token','统一设置所选 Go 目录',ids.map(id=>id+' → '+root+'\\'+({'go-cache':'build','go-mod':'mod','go-temp':'temp'})[id]))},
 ApplyConfig:async token=>{calls.push(['apply',token]);if(token==='batch-token'&&updateFail){updateFail=false;return {error:'目标目录不再为空'}}return {message:'配置完成',error:''}},
 PlanConfig:async (id,dir)=>{calls.push(['singlePlan',id,dir]);return plan('single-token','改目录',[dir])},
 PlanPackage:async (id,verb)=>{calls.push(['packagePlan',id,verb]);return plan('package-token','运行 WinGet',[id,verb])},
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
 w.eval('('+fixture.toString()+')()');w.eval(source);
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
 nav('Go');await tick();assert(d.querySelector('.toolhead').textContent.includes('go1.24.11'));assert(button('更新',d.querySelector('.toolhead')));assert(button('卸载',d.querySelector('.toolhead')));assert(!d.querySelector('.tablecard').textContent.includes('go1.24.11'));
 click('批量改目录');await tick();
 let root=dlg().querySelector('input[aria-label="Go 根目录"]');assert(root);input(root,'D:/test/go/');await tick();
 assert(dlg().textContent.includes('D:\\test\\go\\temp'));assert(dlg().textContent.includes('D:\\test\\go\\mod'));
 for(const input of dlg().querySelectorAll('input[type=checkbox]')){if(input.value==='go-cache')input.click();}
 await tick();click('查看操作预览',dlg());await tick();
 const batchCall=calls().find(x=>x[0]==='batchPlan');assert.deepEqual(batchCall.slice(0,2),['batchPlan','D:/test/go/']);assert.deepEqual(batchCall[2].sort(),['go-mod','go-temp']);assert(!calls().find(x=>x[0]==='apply'));
 click('确认执行',dlg());await tick();assert(dlg().textContent.includes('目标目录不再为空'));assert(button('查看操作预览',dlg()));
 click('查看操作预览',dlg());await tick();click('确认执行',dlg());await tick();assert(d.body.textContent.includes('配置完成'));
 const goCache=[...d.querySelectorAll('.row')].find(e=>e.textContent.includes('构建缓存'));assert(goCache);click('改目录',goCache);await tick();
 let target=dlg().querySelector('[aria-label="目标目录"]');assert(target);assert.equal(d.activeElement,target);
 click('取消',dlg());await tick();assert(!calls().some(c=>c[0]==='singlePlan'));
 nav('Node.js');await tick();assert(button('安装',d.querySelector('.toolhead')));click('安装',d.querySelector('.toolhead'));await tick();click('查看操作预览',dlg());await tick();assert(calls().some(c=>c[0]==='packagePlan'&&c[2]==='install'));assert(!calls().some(c=>c[0]==='packageApply'));click('取消',dlg());await tick();
 nav('Python');await tick();assert(!button('更新',d.querySelector('.toolhead')));assert(!button('安装',d.querySelector('.toolhead')));
 nav('npm');await tick();assert(d.querySelector('.toolhead').textContent.includes('npm 随 Node.js'));
 dom.window.close();console.log('PASS: production Vue DOM with mocked Windows bridge: theme persistence and native calls, navigation, version placement, package gate and plan, batch plan/application and retry, direct edit focus/cancel.');
}
run().catch(e=>{console.error(e);process.exit(1)})
