const fs = require('node:fs');
const {spawnSync} = require('node:child_process');
const root = '/tmp/skillhub-cli-eval-FJVolj';
if (process.env.EVAL_RACE === '1') {
  const writer = spawnSync(root+'/skillhub', ['skill','edit','reliability-review','--description','Writer B description','--yes','--workspace',root+'/hub'], {encoding:'utf8'});
  fs.writeFileSync(root+'/writer-b-output.txt', writer.stdout+writer.stderr);
  if (writer.status !== 0) process.exit(writer.status);
}
fs.appendFileSync(process.argv[2], '\nEditor A added this instruction.\n');
