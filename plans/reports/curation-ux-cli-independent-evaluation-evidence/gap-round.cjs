const fs = require('node:fs');
const {spawnSync} = require('node:child_process');
const root='/tmp/skillhub-cli-eval-FJVolj', hub=root+'/hub', bin=root+'/skillhub';
const log=[];
function run(exe,args){const r=spawnSync(exe,args,{encoding:'utf8'});const e={command:[exe,...args],exit:r.status,stdout:r.stdout,stderr:r.stderr};log.push(e);console.log(JSON.stringify(e));return r;}
function cli(args){return run(bin,[...args,'--workspace',hub]);}
function json(args){return JSON.parse(cli([...args,'--json']).stdout);}
const active=hub+'/skills/software/reliability-review/SKILL.md';
const meta=hub+'/skills/software/reliability-review/skill.meta.yaml';
const pointer=hub+'/runtime/catalog/current.json';
const goodMD=fs.readFileSync(active,'utf8'), goodMeta=fs.readFileSync(meta,'utf8');
cli(['skill','show','reliability-review']);
const before=fs.readFileSync(pointer,'utf8');
fs.writeFileSync(meta,goodMeta.replace('min_scope: multi_step','min_scope: huge'));
cli(['skill','show','reliability-review']);cli(['validate']);
console.log(JSON.stringify({structural_invalid_pointer_unchanged:before===fs.readFileSync(pointer,'utf8')}));
fs.writeFileSync(meta,goodMeta);cli(['skill','show','reliability-review']);
const c=json(['source','capture','runtime/incoming/fixture/references','--reason','Monitor disabled test']);
const p=json(['source','triage',c.candidate.id,'--decision','accept','--source-id','no-monitor-fixture','--no-monitor']);
console.log(JSON.stringify({no_monitor_requested:p.source.monitoring}));
json(['skill','create','--id','json-draft','--collection','core','--name','JSON draft','--description','Inspect draft effect','--yes']);
run('git',['-C',hub,'add','-A']);
// The index is a complete valid baseline; change only one staged file.
fs.writeFileSync(active,'---\nname: wrong-name\ndescription: Example\n---\n\nInstructions.\n');
run('git',['-C',hub,'add','skills/software/reliability-review/SKILL.md']);
fs.writeFileSync(active,goodMD);cli(['validate']);
const snap=root+'/staged-complete';fs.mkdirSync(snap,{recursive:true});
run('git',['-C',hub,'checkout-index','--all','--prefix='+snap+'/']);
run('git',['init','--template=',snap]);
// Git cannot store empty directories: materialize layout scaffolding only.
for(const dir of ['config/schemas','distill/comparisons','distill/skills','distill/sources','evals/routing/cases','evals/routing/suites','history/operations','registry/collections','sources/catalog','sources/intake','sources/skills','skills'])fs.mkdirSync(snap+'/'+dir,{recursive:true});
run(bin,['validate','--workspace',snap]);
// Inverse: valid index, invalid unstaged metadata.
run('git',['-C',hub,'add','skills/software/reliability-review/SKILL.md']);
fs.writeFileSync(meta,goodMeta.replace('min_scope: multi_step','min_scope: huge'));cli(['validate']);
const inverse=root+'/staged-inverse';fs.mkdirSync(inverse,{recursive:true});
run('git',['-C',hub,'checkout-index','--all','--prefix='+inverse+'/']);run('git',['init','--template=',inverse]);
for(const dir of ['config/schemas','distill/comparisons','distill/skills','distill/sources','evals/routing/cases','evals/routing/suites','history/operations','registry/collections','sources/catalog','sources/intake','sources/skills','skills'])fs.mkdirSync(inverse+'/'+dir,{recursive:true});
run(bin,['validate','--workspace',inverse]);
fs.writeFileSync(meta,goodMeta);
cli(['skill','show','reliability-review']);
fs.writeFileSync(root+'/gap-transcript.json',JSON.stringify(log,null,2));
