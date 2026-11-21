export interface Item {
 id:string; tool:string; label:string; path:string; value:string; error:string; bytes:number; partial:boolean; cache:boolean;
 command:string; cleanCommand:string; description:string; risk:string; effect:string; userValue:string; machineValue:string; processValue:string;
 migrate:boolean; clean:boolean; rollback:boolean; packageActions:string[]; executable:string; docs:string
}
type Row={id:string;tool:string;title:string;command:string;variable:string;cleanup:string;description:string;docs:string;packageID:string}
type Snap={value:string;path:string;user:string;machine:string;inherited:string;sizeBytes:number;sizeStatus:string;error:string}
type Drive={name:string;totalBytes:number;freeBytes:number}
export type Plan={token:string;summary:string;lines:string[];error:string}
export type PackageState={available:boolean;registered:boolean;version:string;evidence:string;error:string}
type Result={message:string;error:string}
interface Backend{
 Catalog():Promise<Row[]>;Inspect(id:string,wd:string):Promise<Snap>;DriveStats():Promise<Drive[]>;
 PlanConfig(id:string,target:string):Promise<Plan>;ApplyConfig(token:string):Promise<Result>;
 PlanCleanup(ids:string[],wd:string):Promise<Plan>;ApplyCleanup(token:string):Promise<Result>;
 PlanPackage(id:string,verb:string):Promise<Plan>;ApplyPackage(token:string):Promise<Result>;
 Rollback(id:string):Promise<Result>;RollbackGoRoot():Promise<Result>;GoRootRollbackAvailable?():Promise<boolean>;PlanGoRoot(root:string, ids:string[]):Promise<Plan>; DetectPackage?(id:string):Promise<PackageState>;ChooseDirectory?():Promise<string>
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
 command:row.command||(row.variable?'set '+row.variable:''),cleanCommand:row.cleanup,description:row.description,risk:row.variable?'仅改当前用户变量，不搬运或删除旧目录。已有项目设置可能覆盖。':'',
 effect:row.variable?'重开工具、终端和 IDE 后复查，通常不需重启 Windows。':'',userValue:s.user,machineValue:s.machine,processValue:s.inherited,migrate:!!row.variable,clean:!!row.cleanup&&!s.error,rollback:!!row.variable,packageActions:row.packageID?['list','install','upgrade','uninstall']:[],executable:s.path,docs:row.docs}
}
export const api={
 async scan(wd:string,preview:boolean){
  if(import.meta.env.DEV&&preview){const rows=examples.map(([id,tool,title,value,n,query,variable])=>item({id,tool,title,command:query,variable,cleanup:id==='npm-cache'?'npm cache clean --force':'',description:'示例数据，仅预览界面。',docs:'',packageID:''},{value,path:'',user:'',machine:'',inherited:'',sizeBytes:n,sizeStatus:'逻辑大小',error:''}));return {rows,free:58e9,total:256e9,workdir:''}}
  const a=backend(), [rows,drives]=await Promise.all([a.Catalog(),a.DriveStats().catch(()=>[])])
  const mapped:Item[]=new Array(rows.length)
  let index=0
  await Promise.all(Array.from({length:5},async()=>{while(index<rows.length){const i=index++; const r=rows[i]!;const s=await a.Inspect(r.id,wd);mapped[i]=item(r,s)}}))
  const c=drives?.find(d=>d.name?.toUpperCase().startsWith('C:'))
  return {rows:mapped,free:c?.freeBytes||0,total:c?.totalBytes||0,workdir:''}
 },
 async plan(kind:string,r:Item,target:string,wd:string,selection:string[]=[]):Promise<Plan>{
  const a=backend()
  if(kind==='rollback'||kind==='go-rollback')return {token:'',summary:'恢复原有用户配置',lines:['仅恢复本项由工具记录的旧设置，不删除新目录。'],error:''}
  const v=await(kind==='go-root'?a.PlanGoRoot(target,selection):kind==='migrate'?a.PlanConfig(r.id,target):kind==='bulk'?a.PlanCleanup([],wd):kind==='clean'?a.PlanCleanup([r.id],wd):a.PlanPackage(r.id,kind))
  if(v.error)throw new Error(v.error)
  return v
 },
 async action(kind:string,r:Item,token:string):Promise<string>{
  const a=backend(), v=await(kind==='go-rollback'?a.RollbackGoRoot():kind==='rollback'?a.Rollback(r.id):kind==='go-root'||kind==='migrate'?a.ApplyConfig(token):kind==='clean'||kind==='bulk'?a.ApplyCleanup(token):a.ApplyPackage(token))
  if(v.error)throw new Error(v.error);return v.message
 },
 async goCanUndo(){return (await backend().GoRootRollbackAvailable?.())===true},
 async detect(id:string){return (await backend().DetectPackage?.(id))??null},
 async folder(){return (await backend().ChooseDirectory?.())||''},
 canBrowse(){return !!w.go?.main?.App?.ChooseDirectory}
}
