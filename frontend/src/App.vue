<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {api, type Item, type Plan, type Release, type Policy} from './api'
const active=ref('overview'), selected=ref<Item|null>(null), intent=ref(''), folder=ref(''), error=ref(''), busy=ref(false), status=ref(''), scan=ref(false), rows=ref<Item[]>([]), total=ref(0), free=ref(0)
const filters=[{id:'overview',label:'空间总览',ico:'◈'},{id:'Node.js',label:'Node.js',ico:'⬡'},{id:'npm',label:'npm',ico:'▣'},{id:'pnpm',label:'pnpm',ico:'▦'},{id:'Python',label:'Python',ico:'◇'},{id:'Go',label:'Go',ico:'↗'}]
const query=ref('')
watch(active,()=>query.value='')
const cItems=computed(()=>rows.value.filter(r=>r.cache&&r.path?.toUpperCase().startsWith('C:')))
const shown=computed(()=>{const q=query.value.trim().toLowerCase();const t=active.value==='overview'?cItems.value.slice().sort((a,b)=>b.bytes-a.bytes):rows.value.filter(r=>r.tool===active.value&&!r.id.endsWith('-version'));return q?t.filter(r=>[r.label,r.tool,r.variable,r.path,r.command,r.value].some(x=>x?.toLowerCase().includes(q))):t})
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
const selecting=ref(false), goSelected=ref<string[]>([]), batchRecords=ref<string[]>([])
const policy=ref<Policy>('migrate'), keepBackup=ref(true)
const visibleEditable=computed(()=>shown.value.filter(r=>r.migrate))
const allChecked=computed(()=>visibleEditable.value.length>0&&visibleEditable.value.every(r=>goSelected.value.includes(r.id)))
const canBatch=computed(()=>rows.value.some(r=>r.migrate&&(active.value==='overview'?cItems.value.includes(r):r.tool===active.value)))
const canUndo=computed(()=>shown.value.some(r=>batchRecords.value.includes(r.id)))
const batchLabels=computed(()=>rows.value.filter(r=>goSelected.value.includes(r.id)).map(r=>r.label).join('、'))
const deleteAllowed=computed(()=>intent.value==='go-root'
 ?rows.value.filter(r=>goSelected.value.includes(r.id)).every(r=>r.id!=='pnpm-store'&&(!!r.cleanCommand||r.id==='go-mod'))
 :selected.value?.id!=='pnpm-store'&&(!!selected.value?.cleanCommand||selected.value?.id==='go-mod'))
function toggleAll(){const visible=visibleEditable.value.map(r=>r.id);goSelected.value=allChecked.value?goSelected.value.filter(id=>!visible.includes(id)):[...new Set([...goSelected.value,...visible])]}
function stopSelecting(){selecting.value=false;goSelected.value=[]}
watch(active,stopSelecting)
const versionItem=computed(()=>rows.value.find(r=>r.tool===active.value&&r.id.endsWith('-version')))
const latest=ref<Release|null>(null), latestBusy=ref(false), refreshEpoch=ref(0)
let releaseRequest=0
watch([()=>versionItem.value?.id,refreshEpoch],async ([id])=>{
 const seq=++releaseRequest;latest.value=null;latestBusy.value=false
 if(typeof id!=='string'||preview)return
 latestBusy.value=true
 try{const result=await api.latest(id);if(seq===releaseRequest)latest.value=result}
 catch{if(seq===releaseRequest)latest.value={version:'',channel:'',error:'发布源暂不可用'}}
 finally{if(seq===releaseRequest)latestBusy.value=false}
})
const installed=computed(()=>!!(versionItem.value?.value||versionItem.value?.executable||versionItem.value?.registeredVersion))
const verbs=computed(()=>!versionItem.value?[]:installed.value?['upgrade','uninstall']:['install'])
function openBatch(kind:string){
 const r=rows.value.find(x=>(kind==='go-root'?goSelected.value:batchRecords.value).includes(x.id))
 if(r){open({...r,label:kind==='go-root'?'批量修改目录':'撤销批量设置'});begin(kind,true)}
}
function change(r:Item){open(r);begin('migrate',true)}
function cleanRow(r:Item){open(r);begin('clean',true)}
function pkgAction(verb:string){if(versionItem.value){open({...versionItem.value,label:active.value+' · '+({install:'安装',upgrade:'更新',uninstall:'卸载'}[verb]||'维护')});begin(verb,true);void perform()}}
const canBrowse=api.canBrowse()
const bases=['B','KB','MB','GB','TB']
function size(v:number) { if(!Number.isFinite(v)||v<0)return '—'; let i=0;while(v>=1024&&i<4){v/=1024;i++}return v.toFixed(i===0?0:1)+' '+bases[i] }
async function refresh(){if(busy.value)return;busy.value=true;error.value='';try{refreshEpoch.value++;const [a,records]=await Promise.all([api.scan('',preview,items=>rows.value=items),preview?[]:api.batchItems().catch(()=>[])]);rows.value=a.rows;total.value=a.total;free.value=a.free;batchRecords.value=records;scan.value=true}catch(e){error.value=String(e)}finally{busy.value=false}}
const closeButton=ref<HTMLButtonElement|null>(null)
let invoker:HTMLElement|null=null
watch(selected,async v=>{await nextTick();if(v){if(direct.value&&(intent.value==='migrate'||intent.value==='go-root'))pathInput.value?.focus();else closeButton.value?.focus()}else invoker?.focus()})
function trapFocus(event:KeyboardEvent){
 const dialog=event.currentTarget as HTMLElement
 const focusable=Array.from(dialog.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled)'))
 const first=focusable[0], last=focusable[focusable.length-1]
 if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}
 else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
}
function open(r:Item){invoker=document.activeElement as HTMLElement;direct.value=false;selected.value=r;intent.value='';policy.value='migrate';keepBackup.value=true;folder.value='';plan.value=null;error.value=''}
function begin(i:string,fromRow=false){intent.value=i;direct.value=fromRow;plan.value=null;error.value='';if(i==='migrate'||i==='go-root')void nextTick(()=>pathInput.value?.focus())}
function cancel(){if(direct.value)close();else begin('')}
function bulk(){const r=rows.value.find(x=>x.clean);if(r){open({...r,label:'一键清理',tool:'全部工具',path:'',value:'',command:'',description:'',risk:'',effect:'',userValue:'',machineValue:'',processValue:'',executable:'',docs:''});begin('bulk',true)}}
function close(){if(!busy.value){selected.value=null;intent.value='';error.value=''}}
async function perform(){
 const row=selected.value
 if(!row||busy.value)return
 busy.value=true
 error.value=''
 try{
  if(!plan.value){
   const mode=policy.value==='migrate'&&deleteAllowed.value&&!keepBackup.value?'migrate-clean':policy.value
   plan.value=await api.plan(intent.value,row,folder.value,'',goSelected.value,mode)
   return
  }
  status.value=await api.action(intent.value,row,plan.value.token)
  if(intent.value==='go-root')stopSelecting()
  selected.value=null
  intent.value=''
  plan.value=null
 }catch(e){
  plan.value=null
  error.value=String(e)
 }finally{busy.value=false}
 if(!selected.value)void refresh()
}
function policyOptions(){return [
 {id:'migrate' as const,label:'迁移到新目录',note:deleteAllowed.value?'将旧数据复制到目标目录。':'将旧数据复制到目标目录，保留原文件。'},
 {id:'keep' as const,label:'仅切换目录',note:'使用目标目录已有内容，保留旧数据。'},
 {id:'delete' as const,label:'删除旧缓存',note:deleteAllowed.value?'配置成功后，调用原工具清除旧缓存。':'仅适用于支持清理的缓存。'}
]}
async function browse(){try{const p=await api.folder();if(p)folder.value=p}catch(e){error.value=String(e)}}
async function copy(v:string){try{await navigator.clipboard.writeText(v);status.value='已复制'}catch(e){error.value=String(e)}}
onMounted(()=>void refresh())
</script>

<template>
<div class="shell" @keydown.esc="close">
<aside class="rail" :inert="!!selected">
  <div class="brand"><div class="logo"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="4" y="4" width="16" height="16" rx="4"/><path d="m8 9 3 3-3 3m5 0h3"/></svg></div><div><strong>环境管家</strong><small>开发环境管理</small></div></div>
  <p class="nav-label">总览</p>
  <button class="nav" :class="{chosen:active==='overview'}" @click="active='overview'"><span>◈</span>空间总览</button>
  <p class="nav-label lower">开发环境</p>
  <button v-for="f in filters.slice(1)" :key="f.id" class="nav" :class="{chosen:active===f.id}" @click="active=f.id"><span>{{f.ico}}</span>{{f.label}}</button>
  <div class="themes" aria-label="外观"><button v-for="t in [{id:'system',label:'跟随系统',icon:'系统'},{id:'light',label:'浅色',icon:'浅色'},{id:'dark',label:'深色',icon:'深色'}]" :key="t.id" :title="t.label" :aria-label="t.label" :aria-pressed="theme===t.id" @click="theme=t.id">{{t.icon}}</button></div>
  <div class="railfoot"><i></i> 本机环境 <span>v0.4.1</span></div>
</aside>
<main :inert="!!selected">
<header><div class="crumb">工作台 <span>/</span> {{filters.find(f=>f.id===active)?.label}}</div><button class="sub" :disabled="busy" @click="refresh"><span>⟳</span> {{busy&&!selected?'正在扫描…':'重新扫描'}}</button></header>
<section class="content">
<div class="title titlebar"><div><div class="eyebrow">本地开发环境</div><h1>{{active==='overview'?'空间总览':active}}</h1><p>{{active==='overview'?'找出仍占用 C 盘的开发缓存，再决定迁移或清理。':'查看当前环境的设置、生效路径和维护方式。'}}</p></div></div>
<div v-if="active!=='overview'" class="versionbar">
 <div class="versionfact"><small>{{versionItem?.registeredVersion?'已登记版本 · PATH 未命中':'本机版本'}}</small><strong :title="versionItem?.value">{{versionItem?.value || versionItem?.registeredVersion || (busy?'检测中…':'未检测到')}}</strong></div>
 <div class="versionfact"><small>最新版本 <span v-if="latest?.channel">· {{latest.channel}}</span></small><strong :title="latest?.error">{{latest?.version || (latestBusy?'查询中…':latest?.error?'暂不可用':'—')}}<small v-if="latest?.error&&latest?.version">上次查询</small></strong></div>
 <div v-if="latest?.lts" class="versionfact"><small>最新 LTS</small><strong>{{latest.lts}}</strong></div>
 <div class="toolhead"><button v-for="v in verbs" :key="v" :class="v==='uninstall'?'sub':'primary'" :disabled="busy||preview" @click="pkgAction(v)">{{({install:'安装',upgrade:'更新',uninstall:'卸载'} as Record<string,string>)[v]}}</button></div>
</div>
<div v-if="preview" class="notice">界面预览 · 以下均为示例数据，不操作系统。</div>
<div v-if="active==='overview'" class="stats">
  <article class="disk"><div class="stathead"><span class="metricicon">▤</span><span>系统盘 C:</span></div><strong>{{total>0?size(free):'—'}} <small>可用空间</small></strong><div class="track"><i :style="{width:usedPct+'%'}"></i></div><div class="diskmeta">总容量 {{total>0?size(total):'—'}} <span>{{total>0?Math.round(usedPct)+'% 已用':''}}</span></div></article>
  <article class="metric"><div class="stathead"><span class="metricicon aqua">◫</span><span>C 盘开发缓存</span></div><strong>{{scan?size(estimate):'—'}} <small>逻辑估算</small></strong><p>{{cItems.length}} 个已识别目录 · 不等于可释放空间</p></article>
</div>

<div v-if="error&&!selected" class="error" role="alert">{{error}}</div>
<div v-if="status" class="notice" role="status">{{status}} <button @click="status=''">关闭</button></div>
<section class="tablecard">
 <div class="tabletitle"><div><h2>{{active==='overview'?'C 盘缓存目录':'目录与配置'}}</h2><p>{{active==='overview'?'按估算大小排列，点击详情查看操作风险。':'项目独立配置可能覆盖用户设置。'}}</p></div><div class="actions"><input class="filter" v-model="query" placeholder="查找目录或变量" aria-label="查找目录或变量"><span class="chip">{{shown.length}} 项</span><button v-if="canBatch&&!selecting" class="sub" :disabled="busy||preview" @click="selecting=true">批量改目录</button><button v-if="canUndo&&!selecting" class="plain" :disabled="busy||preview" @click="openBatch('go-rollback')">撤销批量配置</button><button v-if="active==='overview'" class="sub" :disabled="preview||busy||!rows.some(x=>x.clean)" @click="bulk">一键清理</button></div></div>
 <div v-if="selecting" class="selectionbar"><label><input type="checkbox" :checked="allChecked" :indeterminate="!allChecked&&visibleEditable.some(r=>goSelected.includes(r.id))" @change="toggleAll">全选可修改项</label><span aria-live="polite">已选 {{goSelected.length}} 项</span><div><button class="sub" @click="stopSelecting">取消</button><button class="primary" :disabled="!goSelected.length||busy" @click="openBatch('go-root')">设置目录</button></div></div>
 <div v-if="shown.length" class="cols heading"><span>{{active==='overview'?'工具 / 用途':'项目'}}</span><span>当前路径 / 值</span><span>大小</span><span></span></div>
 <div v-for="r in shown" :key="r.id" class="cols row" :class="{picked:selecting&&goSelected.includes(r.id)}">
  <div class="rowname"><input v-if="selecting" type="checkbox" v-model="goSelected" :value="r.id" :aria-label="'选择'+r.label" :disabled="!r.migrate"><span v-else class="toolicon">{{r.tool.slice(0,2)}}</span><div><strong>{{r.label}}</strong><small>{{r.variable || r.tool}}</small></div></div>
  <div class="value" :title="r.path || r.value || r.error">{{r.path || r.value || r.error || (r.id==='go-temp'?'使用系统临时目录':'未配置')}}<small v-if="r.userValue&&r.processValue!==r.userValue" :title="r.userValue">已设置：{{r.userValue}}（重新启动后复查）</small></div>
  <div class="bytes">{{r.bytes>=0?size(r.bytes):'—'}}<small v-if="r.partial">部分估算</small></div>
  <div class="rowactions"><span v-if="!r.migrate" class="readonly" :title="r.description">{{['go-file','go-root','npm-config','npm-prefix','node-exe','python-user'].includes(r.id)?'建议保留':'只读'}}</span><button v-if="r.migrate&&!selecting" class="plain" :disabled="preview||busy" @click="change(r)">改目录</button><button v-if="r.clean&&!selecting" class="plain cleanrow" :disabled="preview||busy" @click="cleanRow(r)">清理</button><button class="plain" @click="open(r)">详情 <span>›</span></button></div>
 </div>
 <div v-if="!shown.length" class="empty"><span>◫</span><strong>{{busy?'正在读取本机环境':query?'没有符合条件的项目':active==='overview'?'暂无 C 盘缓存结果':'暂无检测结果'}}</strong><p>{{busy?'第一次扫描可能需要一些时间。':'请重新扫描，或切换工具查看详细状态。'}}</p></div>
</section>
</section></main>
<Transition><div v-if="selected" class="shade" @click.self="close"><section class="drawer" role="dialog" @keydown.tab="trapFocus" aria-modal="true" :aria-label="selected.label"><div class="drawerhead"><div><div class="eyebrow">{{selected.tool}} / {{direct?'操作':'详情'}}</div><h2>{{selected.label}}</h2></div><button ref="closeButton" class="sub" aria-label="关闭" :disabled="busy" @click="close">×</button></div>
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
<div v-if="intent" class="field confirm"><label v-if="!plan">{{({'go-rollback':'撤销批量配置','go-root':'设置所选目录',bulk:'确认清理',migrate:'迁移设置',clean:'确认清理',rollback:'确认回滚',install:'确认安装',upgrade:'确认更新',uninstall:'确认卸载'} as Record<string,string>)[intent]}}</label>
<template v-if="(intent==='go-root'||intent==='migrate')&&!plan">
 <p v-if="intent==='go-root'" class="batchsummary">{{batchLabels}}。每项将使用根目录下独立的子目录。</p>
 <p v-else>当前目录</p><code v-if="intent==='migrate'">{{selected.value||'未配置专用目录'}}</code>
 <label for="destination">{{intent==='go-root'?'目标根目录':'目标目录'}}</label>
 <div class="pick"><input id="destination" ref="pathInput" v-model="folder" :disabled="busy" aria-label="目标目录" placeholder="输入绝对路径或选择文件夹" @keydown.enter.prevent="folder.trim()&&!busy&&perform()"><button v-if="canBrowse" class="sub" :disabled="busy" @click="browse()">选择</button></div>
 <fieldset class="policies" :disabled="busy"><legend>旧目录数据</legend><label v-for="option in policyOptions()" :key="option.id"><input type="radio" v-model="policy" :value="option.id" :disabled="option.id==='delete'&&!deleteAllowed"><span>{{option.label}}<small>{{option.note}}</small></span></label></fieldset>
 <label v-if="policy==='migrate'&&deleteAllowed" class="backup-choice"><input type="checkbox" v-model="keepBackup" :disabled="busy">保留旧缓存副本</label><p v-if="policy==='migrate'&&deleteAllowed&&!keepBackup">迁移成功后清理旧缓存，释放原位置的空间。</p>
 </template>
<p v-else-if="intent==='clean'||intent==='bulk'">将调用工具自己的缓存清理命令。清理不可撤销，后续可能需要重新下载或编译。</p>
<p v-else-if="intent==='rollback'||intent==='go-rollback'">恢复此前的用户配置。撤销后仍可在新目录找到后来写入的数据。</p>
<p v-else-if="intent!=='migrate'&&intent!=='go-root'&&intent!=='go-rollback'">核对下方的维护渠道、目标和操作。</p>
<div v-if="intent==='clean'&&selected.cleanCommand" class="field"><code>{{selected.cleanCommand}}</code></div>
<div v-if="plan" class="field"><label>{{plan.summary}}</label><article v-for="p in plan.directories" :key="p.variable" class="dest-preview"><strong>{{p.label}}</strong><small>{{p.variable}}</small><dl><dt>当前</dt><dd>{{p.current}}</dd><dt>设置后</dt><dd>{{p.target}}</dd></dl></article><div class="planline" v-for="(line,i) in plan.lines" :key="i">{{line}}</div></div><div class="actions"><button v-if="plan&&(intent==='migrate'||intent==='go-root')" class="sub" :disabled="busy" @click="plan=null">返回修改</button><button class="sub" :disabled="busy" @click="cancel">取消</button><button class="primary" :disabled="busy||((intent==='migrate'||intent==='go-root')&&!folder.trim())||(intent==='go-root'&&!goSelected.length)" @click="perform">{{busy?'处理中…':plan?(plan.summary.startsWith('打开')?'打开维护入口':'确认执行'):'确定'}}</button></div></div>
<div v-if="error" class="error" role="alert">{{error}}</div>
</div></section></div></Transition>
</div>
</template>
