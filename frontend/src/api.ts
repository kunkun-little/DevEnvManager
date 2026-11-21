export interface Item {
 id:string; tool:string; label:string; path:string; value:string; error:string; bytes:number; partial:boolean; cache:boolean;
 registeredVersion?:string; variable:string; command:string; cleanCommand:string; description:string; risk:string; effect:string; userValue:string; machineValue:string; processValue:string;
 migrate:boolean; clean:boolean; rollback:boolean; executable:string; docs:string
}
type Row={id:string;tool:string;title:string;command:string;variable:string;cleanup:string;description:string;docs:string;packageID:string}
type Snap={settingVariable?:string;value:string;path:string;user:string;machine:string;inherited:string;sizeBytes:number;sizeStatus:string;error:string;registeredVersion?:string;rollback?:boolean}
type Drive={name:string;totalBytes:number;freeBytes:number}
export type Plan={directories?:{label:string;variable:string;current:string;target:string}[];token:string;summary:string;lines:string[];error:string}
export type Policy='migrate'|'keep'|'delete'|'migrate-clean'
export type Release={lts?:string;version:string;channel:string;error:string}
type Result={message:string;error:string}
interface Backend{
 LatestVersion(id:string):Promise<Release>; PlanDirectories(target:string,ids:string[],policy:Policy,batch:boolean):Promise<Plan>; PlanMaintenance(id:string,verb:string):Promise<Plan>; BatchItems():Promise<string[]|null>; InspectPnpm(wd:string):Promise<Record<string,Snap>>; InspectGo(wd:string):Promise<Record<string,Snap>>;
 Catalog():Promise<Row[]>;Inspect(id:string,wd:string):Promise<Snap>;DriveStats():Promise<Drive[]>;
 ApplyConfig(token:string):Promise<Result>;
 PlanCleanup(ids:string[],wd:string):Promise<Plan>;ApplyCleanup(token:string):Promise<Result>;
 ApplyPackage(token:string):Promise<Result>;
 Rollback(id:string):Promise<Result>;RollbackGoRoot():Promise<Result>;ChooseDirectory?():Promise<string>
}
const w=window as Window & {go?:{main?:{App?:Backend}}}
function backend(){ const a=w.go?.main?.App;if(!a)throw new Error('请从 Windows 桌面程序打开工具，而不是直接打开网页。');return a}
const cacheIDs=new Set(['node-cache','npm-cache','pnpm-store','pnpm-cache','pip-cache','python-pycache','go-cache','go-mod','go-temp'])
const examples = [
 ['npm-cache','npm','下载缓存','C:\\Users\\Dev\\AppData\\Local\\npm-cache',4740000000,'npm config get cache','NPM_CONFIG_CACHE'],
 ['go-cache','Go','构建缓存','C:\\Users\\Dev\\AppData\\Local\\go-build',2440000000,'go env GOCACHE','GOCACHE'],
 ['pip-cache','Python','pip 下载缓存','C:\\Users\\Dev\\AppData\\Local\\pip\\Cache',1150000000,'python -I -B -m pip cache dir','PIP_CACHE_DIR'],
 ['pnpm-store','pnpm','共享 store','C:\\Users\\Dev\\AppData\\Local\\pnpm\\store',765000000,'pnpm store path',''],
 ['node-cache','Node.js','JS 编译缓存','C:\\DevCache\\Node',88000000,'','NODE_COMPILE_CACHE'],
] as const
function item(row:Row,s:Snap):Item{
 const cache=cacheIDs.has(row.id)
 return {id:row.id,tool:row.tool,label:row.title,path:cache?s.value:'',value:s.value,error:s.error,bytes:cache&&s.sizeStatus?.includes('逻辑大小')?s.sizeBytes:-1,partial:s.sizeStatus?.includes('部分')||false,cache,
 registeredVersion:s.registeredVersion,variable:s.settingVariable||row.variable,command:row.command||(row.variable?'set '+row.variable:''),cleanCommand:row.cleanup,description:row.description,risk:row.variable?'设置作用于当前用户。已有项目设置可能覆盖此值。':'',
 effect:row.variable?'重开工具、终端和 IDE 后复查，通常不需重启 Windows。':'',userValue:s.user,machineValue:s.machine,processValue:s.inherited,migrate:!!row.variable,clean:!!row.cleanup&&!s.error,rollback:s.rollback===true,executable:s.path,docs:row.docs}
}
export const api={
 async scan(wd:string,preview:boolean,onRows?:(items:Item[])=>void){
  if(import.meta.env.DEV&&preview){const rows=examples.map(([id,tool,title,value,n,query,variable])=>item({id,tool,title,command:query,variable,cleanup:id==='npm-cache'?'npm cache clean --force':'',description:'示例数据，仅预览界面。',docs:'',packageID:''},{value,path:'',user:'',machine:'',inherited:'',sizeBytes:n,sizeStatus:'逻辑大小',error:''}));return {rows,free:58e9,total:256e9,workdir:''}}
  const a=backend(), [rows,drives]=await Promise.all([a.Catalog(),a.DriveStats().catch(()=>[])])
  const mapped:Item[]=new Array(rows.length)
  let index=0
  const goScan=rows.some(r=>r.tool==='Go')?a.InspectGo(wd):null
  const pnpmScan=rows.some(r=>r.tool==='pnpm')?a.InspectPnpm(wd):null
  const ordered=rows.map((r,i)=>({r,i})).sort((a,b)=>Number(b.r.id.endsWith('-version'))-Number(a.r.id.endsWith('-version')))
  await Promise.all(Array.from({length:5},async()=>{while(index<ordered.length){
   const {r,i}=ordered[index++]!
   const snapshot=r.tool==='Go'?(await goScan)![r.id]!:r.tool==='pnpm'?(await pnpmScan)![r.id]!:await a.Inspect(r.id,wd)
   mapped[i]=item(r,snapshot)
   onRows?.(mapped.filter(Boolean))
  }}))
  const c=drives?.find(d=>d.name?.toUpperCase().startsWith('C:'))
  return {rows:mapped,free:c?.freeBytes||0,total:c?.totalBytes||0,workdir:''}
 },
 async plan(kind:string,r:Item,target:string,wd:string,selection:string[]=[],policy:Policy="migrate"):Promise<Plan>{
  const a=backend()
  if(kind==='rollback'||kind==='go-rollback')return {token:'',summary:'恢复原有用户配置',lines:['仅恢复本项由工具记录的旧设置，不删除新目录。'],error:''}
  const v=await(kind==='go-root'?a.PlanDirectories(target,selection,policy,true):kind==='migrate'?a.PlanDirectories(target,[r.id],policy,false):kind==='bulk'?a.PlanCleanup([],wd):kind==='clean'?a.PlanCleanup([r.id],wd):a.PlanMaintenance(r.id,kind))
  if(v.error)throw new Error(v.error)
  return v
 },
 async action(kind:string,r:Item,token:string):Promise<string>{
  const a=backend(), v=await(kind==='go-rollback'?a.RollbackGoRoot():kind==='rollback'?a.Rollback(r.id):kind==='go-root'||kind==='migrate'?a.ApplyConfig(token):kind==='clean'||kind==='bulk'?a.ApplyCleanup(token):a.ApplyPackage(token))
  if(v.error)throw new Error(v.error);return v.message
 },
 async batchItems(){return (await backend().BatchItems())||[]},
 async latest(id:string){return backend().LatestVersion(id)},
 async folder(){return (await backend().ChooseDirectory?.())||''},
 canBrowse(){return !!w.go?.main?.App?.ChooseDirectory}
}
