<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {api, type Item, type Plan, type PackageState} from './api'
const active=ref('overview'), selected=ref<Item|null>(null), intent=ref(''), folder=ref(''), error=ref(''), busy=ref(false), status=ref(''), scan=ref(false), rows=ref<Item[]>([]), total=ref(0), free=ref(0)
const filters=[{id:'overview',label:'空间总览',ico:'◈'},{id:'Node.js',label:'Node.js',ico:'⬡'},{id:'npm',label:'npm',ico:'▣'},{id:'pnpm',label:'pnpm',ico:'▦'},{id:'Python',label:'Python',ico:'◇'},{id:'Go',label:'Go',ico:'↗'}]
const query=ref('')
watch(active,()=>query.value='')
const cItems=computed(()=>rows.value.filter(r=>r.cache&&r.path?.toUpperCase().startsWith('C:')))
const shown=computed(()=>{const q=query.value.trim().toLowerCase();const t=active.value==='overview'?cItems.value.slice().sort((a,b)=>b.bytes-a.bytes):rows.value.filter(r=>r.tool===active.value&&!r.id.endsWith('-version'));return q?t.filter(r=>[r.label,r.tool,r.path,r.command,r.value].some(x=>x?.toLowerCase().includes(q))):t})
const estimate=computed(()=>cItems.value.reduce((n,r)=>n+Math.max(r.bytes||0,0),0))
const usedPct=computed(()=>total.value>0?Math.max(0,Math.min(100,100*(1-free.value/total.value))):0)
const preview = import.meta.env.DEV && new URLSearchParams(location.search).get('preview') === '1'
const plan=ref<Plan|null>(null), direct=ref(false)
const pathInput=ref<HTMLInputElement|null>(null)
const theme=ref('system')
try{const t=localStorage.getItem('workspace-theme'); if(t&&['system','light','dark'].includes(t))theme.value=t}catch{}
type HostTheme = {WindowSetSystemDefaultTheme?():void;WindowSetLightTheme?():void;WindowSetDarkTheme?():void}
const applyTheme=(v:string)=>{
 document.documentElement.dataset.theme=v
 try{localStorage.setItem('workspace-theme',v)}catch{}
 const host=(window as Window&{runtime?:HostTheme}).runtime
 if(v==='dark')host?.WindowSetDarkTheme?.()
 else if(v==='light')host?.WindowSetLightTheme?.()
 else host?.WindowSetSystemDefaultTheme?.()
}
watch(theme,applyTheme,{immediate:true})
const goOptions=[{id:'go-cache',name:'构建缓存',dir:'build'},{id:'go-mod',name:'模块缓存',dir:'mod'},{id:'go-temp',name:'临时目录',dir:'temp'}]
const goSelected=ref<string[]>(goOptions.map(x=>x.id))
const goCanUndo=ref(false)
const childPaths=computed(()=> {
 const root=folder.value.trim().replace(/\//g,'\\').replace(/\\+$/,'')
 return goOptions.map(x=>({...x,path:(root||'…')+'\\'+x.dir}))
})
const versionItem=computed(()=>rows.value.find(r=>r.tool===active.value&&r.id.endsWith('-version')))
const pkg=ref<PackageState|null>(null), packageLoading=ref(false)
let packageQuery=0
watch(versionItem,async r=>{const id=++packageQuery;pkg.value=null;packageLoading.value=false;if(!r?.packageActions.length||preview)return;packageLoading.value=true;const p=await api.detect(r.id).catch(()=>null);if(id===packageQuery){pkg.value=p;packageLoading.value=false}})
const packageNote=computed(()=>{
 const r=versionItem.value
 if(!r)return '扫描后显示本机版本'
 if(r.tool==='npm')return 'npm 随 Node.js 管理'
 if(r.tool==='pnpm')return '来源可能是 Corepack、npm 等，本工具不接管'
 if(!r.packageActions.length)return '未提供受支持的安装源'
 if(!pkg.value)return packageLoading.value?'正在识别 WinGet 精确包':'无法确认 WinGet 安装状态'
 if(!pkg.value.available||pkg.value.error)return pkg.value.error||'WinGet 不可用'
 if(pkg.value.registered)return 'WinGet 匹配版本：'+(pkg.value.version||'未识别')+'，可能与当前版本不同'
 if(r.value||r.executable)return '已找到本机入口，请通过原安装渠道管理'
 return '可通过 WinGet 安装'
})
const verbs=computed(()=>{const r=versionItem.value;if(!r?.packageActions.length||!pkg.value?.available||pkg.value.error)return [];return pkg.value.registered?['upgrade','uninstall']:r.value||r.executable?[]:['install']})
function openGo(kind:string){const r=rows.value.find(x=>x.id==='go-cache');if(r){open({...r,label:'Go 目录设置',path:'',value:'',command:'',description:'',risk:'',effect:'',userValue:'',machineValue:'',processValue:'',executable:'',docs:''});goSelected.value=goOptions.map(x=>x.id);begin(kind,true)}}
function change(r:Item){open(r);begin('migrate',true)}
function cleanRow(r:Item){open(r);begin('clean',true)}
function pkgAction(verb:string){if(versionItem.value){open(versionItem.value);begin(verb,true)}}
const canBrowse=api.canBrowse()
const bases=['B','KB','MB','GB','TB']
function size(v:number) { if(!Number.isFinite(v)||v<0)return '—'; let i=0;while(v>=1024&&i<4){v/=1024;i++}return v.toFixed(i===0?0:1)+' '+bases[i] }
async function refresh(){if(busy.value)return;busy.value=true;error.value='';try{const [a,canUndo]=await Promise.all([api.scan('',preview),preview?false:api.goCanUndo().catch(()=>false)]);rows.value=a.rows;total.value=a.total;free.value=a.free;goCanUndo.value=canUndo;scan.value=true}catch(e){error.value=String(e)}finally{busy.value=false}}
const closeButton=ref<HTMLButtonElement|null>(null)
let invoker:HTMLElement|null=null
watch(selected,async v=>{await nextTick();if(v){if(direct.value&&(intent.value==='migrate'||intent.value==='go-root'))pathInput.value?.focus();else closeButton.value?.focus()}else invoker?.focus()})
function open(r:Item){invoker=document.activeElement as HTMLElement;direct.value=false;selected.value=r;intent.value='';folder.value='';plan.value=null;error.value=''}
function begin(i:string,fromRow=false){intent.value=i;direct.value=fromRow;plan.value=null;error.value='';if(i==='migrate'||i==='go-root')void nextTick(()=>pathInput.value?.focus())}
function cancel(){if(direct.value)close();else begin('')}
function bulk(){const r=rows.value.find(x=>x.clean);if(r){open({...r,label:'一键清理',tool:'全部工具',path:'',value:'',command:'',description:'',risk:'',effect:'',userValue:'',machineValue:'',processValue:'',executable:'',docs:''});begin('bulk',true)}}
function close(){if(!busy.value){selected.value=null;intent.value='';error.value=''}}
async function perform(){if(!selected.value||busy.value)return;busy.value=true;error.value='';try{const r=selected.value;if(!plan.value){plan.value=await api.plan(intent.value,r,folder.value,'',goSelected.value);return};const token=plan.value.token;plan.value=null;status.value=await api.action(intent.value,r,token);selected.value=null;intent.value='';plan.value=null}catch(e){error.value=String(e)}finally{busy.value=false};if(!selected.value)void refresh()}
async function browse(){try{const p=await api.folder();if(p)folder.value=p}catch(e){error.value=String(e)}}
async function copy(v:string){try{await navigator.clipboard.writeText(v);status.value='已复制'}catch(e){error.value=String(e)}}
onMounted(()=>void refresh())
</script>

<template>
<div class="shell" @keydown.esc="close">
<aside class="rail">
  <div class="brand"><div class="logo"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="4" y="4" width="16" height="16" rx="4"/><path d="m8 9 3 3-3 3m5 0h3"/></svg></div><div><strong>环境管家</strong><small>开发环境管理</small></div></div>
  <p class="nav-label">总览</p>
  <button class="nav" :class="{chosen:active==='overview'}" @click="active='overview'"><span>◈</span>空间总览</button>
  <p class="nav-label lower">开发环境</p>
  <button v-for="f in filters.slice(1)" :key="f.id" class="nav" :class="{chosen:active===f.id}" @click="active=f.id"><span>{{f.ico}}</span>{{f.label}}</button>
  <div class="themes" aria-label="外观"><button v-for="t in [{id:'system',label:'跟随系统',icon:'系统'},{id:'light',label:'浅色',icon:'浅色'},{id:'dark',label:'深色',icon:'深色'}]" :key="t.id" :title="t.label" :aria-label="t.label" :aria-pressed="theme===t.id" @click="theme=t.id">{{t.icon}}</button></div>
  <div class="railfoot"><i></i> 本机环境 <span>v0.3</span></div>
</aside>
<main>
<header><div class="crumb">工作台 <span>/</span> {{filters.find(f=>f.id===active)?.label}}</div><button class="sub" :disabled="busy" @click="refresh"><span>⟳</span> {{busy?'正在扫描…':'重新扫描'}}</button></header>
<section class="content">
<div class="title titlebar"><div><div class="eyebrow">本地开发环境</div><h1>{{active==='overview'?'空间总览':active}}</h1><p>{{active==='overview'?'找出仍占用 C 盘的开发缓存，再决定迁移或清理。':'查看当前环境的设置、生效路径和维护方式。'}}</p></div><div v-if="active!=='overview'" class="toolhead"><div class="package-meta"><span class="tag">当前版本 · {{versionItem?.value || pkg?.version || (busy?'检测中':'未识别')}}</span><small :title="packageNote">{{packageNote}}</small></div><button v-for="v in verbs" :key="v" class="sub" :disabled="busy||preview" @click="pkgAction(v)">{{({install:'安装',upgrade:'更新',uninstall:'卸载'} as Record<string,string>)[v]}}</button></div></div>
<div v-if="preview" class="notice">界面预览 · 以下均为示例数据，不操作系统。</div>
<div v-if="active==='overview'" class="stats">
  <article class="disk"><div class="stathead"><span class="metricicon">▤</span><span>系统盘 C:</span></div><strong>{{total>0?size(free):'—'}} <small>可用空间</small></strong><div class="track"><i :style="{width:usedPct+'%'}"></i></div><div class="diskmeta">总容量 {{total>0?size(total):'—'}} <span>{{total>0?Math.round(usedPct)+'% 已用':''}}</span></div></article>
  <article class="metric"><div class="stathead"><span class="metricicon aqua">◫</span><span>C 盘开发缓存</span></div><strong>{{scan?size(estimate):'—'}} <small>逻辑估算</small></strong><p>{{cItems.length}} 个已识别目录 · 不等于可释放空间</p></article>
</div>

<div v-if="error&&!selected" class="error" role="alert">{{error}}</div>
<div v-if="status" class="notice" role="status">{{status}} <button @click="status=''">关闭</button></div>
<section class="tablecard">
 <div class="tabletitle"><div><h2>{{active==='overview'?'C 盘缓存目录':'目录与配置'}}</h2><p>{{active==='overview'?'按估算大小排列，点击详情查看操作风险。':'项目独立配置可能覆盖用户设置。'}}</p></div><div class="actions"><input class="filter" v-model="query" placeholder="查找目录或变量" aria-label="查找目录或变量"><span class="chip">{{shown.length}} 项</span><button v-if="active==='Go'" class="sub" :disabled="busy||preview" @click="openGo('go-root')">批量改目录</button><button v-if="active==='Go'" class="plain" :disabled="busy||preview||!goCanUndo" :title="goCanUndo?'恢复本工具记录的上一批设置':'暂无批量配置可撤销'" @click="openGo('go-rollback')">撤销批量配置</button><button v-if="active==='overview'" class="sub" :disabled="preview||busy||!rows.some(x=>x.clean)" @click="bulk">一键清理</button></div></div>
 <div v-if="shown.length" class="cols heading"><span>{{active==='overview'?'工具 / 用途':'项目'}}</span><span>当前路径 / 值</span><span>大小</span><span></span></div>
 <div v-for="r in shown" :key="r.id" class="cols row">
  <div class="rowname"><span class="toolicon">{{r.tool.slice(0,2)}}</span><div><strong>{{r.label}}</strong><small>{{r.tool}}</small></div></div>
  <div class="value" :title="r.path || r.value || r.error">{{r.path || r.value || r.error || '未检测到'}}<small v-if="r.userValue&&r.processValue!==r.userValue" :title="r.userValue">已设置：{{r.userValue}}（重新启动后复查）</small></div>
  <div class="bytes">{{r.bytes>=0?size(r.bytes):'—'}}<small v-if="r.partial">部分估算</small></div>
  <div class="rowactions"><button v-if="r.migrate" class="plain" :disabled="preview||busy" @click="change(r)">改目录</button><button v-if="r.clean" class="plain cleanrow" :disabled="preview||busy" @click="cleanRow(r)">清理</button><button class="plain" @click="open(r)">详情 <span>›</span></button></div>
 </div>
 <div v-if="!shown.length" class="empty"><span>◫</span><strong>{{busy?'正在读取本机环境':query?'没有符合条件的项目':active==='overview'?'暂无 C 盘缓存结果':'暂无检测结果'}}</strong><p>{{busy?'第一次扫描可能需要一些时间。':'请重新扫描，或切换工具查看详细状态。'}}</p></div>
</section>
</section></main>
<Transition><div v-if="selected" class="shade" @click.self="close"><section class="drawer" role="dialog" aria-modal="true" :aria-label="selected.label"><div class="drawerhead"><div><div class="eyebrow">{{selected.tool}} / {{direct?'操作':'详情'}}</div><h2>{{selected.label}}</h2></div><button ref="closeButton" class="sub" aria-label="关闭" :disabled="busy" @click="close">×</button></div>
<div class="drawerbody"><template v-if="!direct"><div v-if="intent!=='bulk'&&intent!=='go-root'&&intent!=='go-rollback'" class="field"><label>当前生效值</label><code>{{selected.path||selected.value||selected.error||'未检测到'}}</code></div>
<div class="field" v-if="selected.command"><label>查询命令 <button class="plain" @click="copy(selected.command)">复制</button></label><code>{{selected.command}}</code></div>
<div class="field" v-if="selected.userValue||selected.machineValue||selected.processValue"><label>环境变量</label><p>进程：{{selected.processValue||'未设置'}}<br>用户：{{selected.userValue||'未设置'}}<br>系统：{{selected.machineValue||'未设置'}}</p></div>
<div class="field" v-if="selected.executable"><label>命令入口</label><code>{{selected.executable}}</code></div><div v-if="selected.docs" class="field"><label>官方文档 <button class="plain" @click="copy(selected.docs)">复制链接</button></label><code>{{selected.docs}}</code></div><div class="field" v-if="selected.description"><label>用途</label><p>{{selected.description}}</p></div>
<div class="risk" v-if="selected.risk||selected.effect"><strong>风险与生效条件</strong><p>{{selected.risk}} {{selected.effect}}</p></div>
<div v-if="!intent" class="actions">
<button class="primary" v-if="selected.migrate" :disabled="preview" @click="begin('migrate')">配置新目录</button>
<button class="sub" v-if="selected.rollback" :disabled="preview" @click="begin('rollback')">撤销配置</button>
<button class="sub" v-if="selected.clean" :disabled="preview" @click="begin('clean')">清理缓存</button>

</div></template>
<div v-if="intent" class="field confirm"><label>{{({'go-rollback':'撤销 Go 批量配置','go-root':'批量配置 Go 目录',bulk:'确认清理',migrate:'迁移设置',clean:'确认清理',rollback:'确认回滚',install:'确认安装',upgrade:'确认更新',uninstall:'确认卸载'} as Record<string,string>)[intent]}}</label>
<template v-if="intent==='go-root'&&!plan"><p>从一个根目录生成专用子目录，只修改勾选项，不删除旧文件。</p><div class="pick"><input ref="pathInput" v-model="folder" aria-label="Go 根目录" placeholder="例如 D:\DevCache\Go"><button v-if="canBrowse" class="sub" @click="browse()">选择</button></div><div class="checklist"><label v-for="opt in childPaths" :key="opt.id"><input type="checkbox" v-model="goSelected" :value="opt.id"><span>{{opt.name}}<small>{{opt.path}}</small></span></label></div></template>
<template v-else-if="intent==='migrate'&&!plan"><p>填写新盘的专用空目录，只更新配置，不删除旧文件。重开相关进程后复查。</p><div class="pick"><input ref="pathInput" v-model="folder" aria-label="目标目录" placeholder="例如 D:\\DevCache\\npm"><button v-if="canBrowse" class="sub" @click="browse()">选择</button></div></template>
<p v-else-if="intent==='clean'||intent==='bulk'">将调用工具自己的缓存清理命令。清理不可撤销，后续可能需要重新下载或编译。</p>
<p v-else-if="intent==='rollback'||intent==='go-rollback'">恢复此前的用户配置，不删除新目录里的文件。</p>
<p v-else-if="intent!=='migrate'&&intent!=='go-root'&&intent!=='go-rollback'">调用 WinGet 处理精确软件包。可能联网并弹出系统确认，请先核对软件包和安装来源。</p>
<div v-if="intent==='clean'&&selected.cleanCommand" class="field"><code>{{selected.cleanCommand}}</code></div>
<div v-if="plan" class="field"><label>{{plan.summary}}</label><div class="planline" v-for="(line,i) in plan.lines" :key="i">{{line}}</div></div><div class="actions"><button class="sub" :disabled="busy" @click="cancel">取消</button><button class="primary" :disabled="busy||((intent==='migrate'||intent==='go-root')&&!folder.trim())||(intent==='go-root'&&!goSelected.length)" @click="perform">{{busy?'处理中…':plan?'确认执行':'查看操作预览'}}</button></div></div>
<div v-if="error" class="error" role="alert">{{error}}</div>
</div></section></div></Transition>
</div>
</template>
