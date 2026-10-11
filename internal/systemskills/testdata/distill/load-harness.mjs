// Test harness for the distill-lab script's loader and rounding (used by distill_script_test.go).
//
//   node load-harness.mjs <distill.mjs> load <file>     typed JSON of the loaded value
//   node load-harness.mjs <distill.mjs> round <x>...    round(x, 2) of each number, one per line
//
// Typed JSON keeps every distinction the validator relies on: strings are JSON strings, integers
// {"int": "<digits>"}, floats {"float": "<repr>"}, dates {"date": ...}, datetimes {"datetime": ...},
// mappings {"map": [[key, value], ...]} in file order, sequences arrays, booleans and null as is.
import { pathToFileURL } from 'node:url';

const [script, mode, ...rest] = process.argv.slice(2);
const lab = await import(pathToFileURL(script).href);

function typed(value) {
  if (value === null || typeof value === 'boolean') return value;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'bigint') return { int: String(value) };
  if (value instanceof lab.PyFloat) return { float: lab.floatRepr(value.value) };
  if (value instanceof lab.PyDate) return { date: value.iso() };
  if (value instanceof lab.PyDateTime) return { datetime: value.iso(' ') };
  if (Array.isArray(value)) return value.map(typed);
  if (value instanceof lab.PyDict) return { map: value.entries().map(([k, v]) => [typed(k), typed(v)]) };
  throw new Error(`unexpected loaded value ${String(value)}`);
}

if (mode === 'load') {
  process.stdout.write(`${JSON.stringify(typed(lab.load(rest[0])), null, 1)}\n`);
} else if (mode === 'round') {
  for (const x of rest) process.stdout.write(`${lab.floatRepr(lab.pyRound(Number(x), 2))}\n`);
} else {
  process.stderr.write(`unknown mode ${mode}\n`);
  process.exitCode = 2;
}
