#!/usr/bin/env node
// distill-lab/scripts/distill.mjs: validate, score, sort and update a skill's .meta/distill.yaml.
//
// Self-contained: it needs Node 18 or newer and, for the evidence check only, Git 2.45.0 or newer.
// It imports only Node built-ins and the YAML loader vendored next to it; nothing is installed.
// The script opens no network connection: its only child process is local git, run against the
// source mirrors with lazy fetching disabled.
//
// Vendored YAML loader: js-yaml 4.3.2, MIT license (lib/js-yaml.LICENSE), copied unmodified from
//   tarball    https://registry.npmjs.org/js-yaml/-/js-yaml-4.3.2.tgz
//   integrity  sha512-SFNOvSJ+Dgf/9An904Yx+CgSlIPCkIpao4qo51lpee25TIRejdH3rhR4EZMGoNx3/TP3O+wzWuiTFl4sqbltzA==
//   lib/js-yaml.mjs      <- package/dist/js-yaml.mjs, 104118 bytes,
//                           sha256 cea276c7e15f409a1adbe5d177aba7824398a474f7cf702bd57962c7d570636f
//   lib/js-yaml.LICENSE  <- package/LICENSE, 1084 bytes,
//                           sha256 a07bc24468b9654ce76a547d47a2db282d07733b715db4c73a98bd63961f9550
// To update it: pick the new version, download its tarball, check its sha512 against the registry's
// dist.integrity, copy the two files unmodified into scripts/lib/ of both copies of this skill
// (.agents/skills/distill-lab and internal/systemskills/distill-lab), update the digests above and in
// internal/systemskills/distill_script_test.go, regenerate the release notices
// (cd web && npm run notices), and keep the old file digests recognized as shipped.
//
// The loader and the writer reproduce the reference emitter that wrote the existing distill.yaml
// files (YAML 1.1 scalar resolution, its quoting, folding and line wrapping), so files written by
// the previous implementation load to the same values and re-format to the same bytes.

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { FAILSAFE_SCHEMA, Type, load as yamlLoad } from './lib/js-yaml.mjs';

const SCRIPT = 'distill.mjs';

const LAYERS = ['enforcement', 'content', 'history', 'validation', 'craft', 'wording', 'ecosystem'];
const CONTRASTS = ['new', 'extends', 'contradicts', 'already-covered'];
const STATES = ['candidate', 'planned', 'ported', 'rejected'];
const SCORE_RANGES = [['relevance', 0, 3], ['evidence', 1, 3], ['effort', 1, 3]];
const IMPACT_FACTS = ['a', 'b', 'c', 'd', 'e'];

const TOP_ORDER = ['goal', 'cursors', 'coverage', 'lessons'];
const GOAL_ORDER = ['status', 'purpose', 'in_scope', 'out_of_scope', 'failures_it_prevents'];
const LESSON_ORDER = ['key', 'layer', 'what', 'notable', 'where', 'contrast', 'score',
  'final_score', 'also_fits', 'status', 'found_by', 'decision'];
const SCORE_ORDER = ['relevance', 'facts', 'impact', 'evidence', 'effort', 'why'];
const DECISION_ORDER = ['state', 'reason', 'at'];

const GIT_MIN = [2, 45, 0];
const GIT_TIMEOUT_MS = 30000;

// ---------- errors and exit codes ----------

// A finished run: exit code, text for stderr and, for --help, text for stdout.
class ExitError extends Error {
  constructor(code, text, stdout = '') {
    super(text || stdout);
    this.code = code;
    this.text = text || '';
    this.stdout = stdout;
  }
}

function fatal(error, why, fix) {
  throw new ExitError(1, `ERROR: ${error}\nWHY: ${why}\nFIX: ${fix}\n`);
}

function fail(errors) {
  const lines = ['ERROR: distill.yaml is invalid; nothing was written', ...errors.map((e) => `  - ${e}`)];
  throw new ExitError(1, lines.join('\n') + '\n');
}

// ---------- values with the semantics of the reference loader ----------
// Mappings keep typed keys (an unquoted 1 and a quoted '1' are different keys), floats stay distinct
// from integers (3.0 is not an integer score), and timestamps keep their calendar fields.

class PyFloat {
  constructor(value) {
    this.value = value;
  }
}

class PyDate {
  constructor(year, month, day) {
    checkDate(year, month, day);
    Object.assign(this, { year, month, day });
  }
  iso() {
    return `${pad(this.year, 4)}-${pad(this.month, 2)}-${pad(this.day, 2)}`;
  }
}

class PyDateTime {
  constructor(year, month, day, hour, minute, second, micro, tzMinutes) {
    checkDate(year, month, day);
    if (hour > 23 || minute > 59 || second > 59) throw new Error('hour, minute or second out of range');
    if (tzMinutes !== null && Math.abs(tzMinutes) >= 24 * 60) throw new Error('offset must be a timedelta strictly between -timedelta(hours=24) and timedelta(hours=24)');
    Object.assign(this, { year, month, day, hour, minute, second, micro, tzMinutes });
  }
  iso(sep = 'T') {
    let text = `${pad(this.year, 4)}-${pad(this.month, 2)}-${pad(this.day, 2)}${sep}`
      + `${pad(this.hour, 2)}:${pad(this.minute, 2)}:${pad(this.second, 2)}`;
    if (this.micro) text += `.${pad(this.micro, 6)}`;
    if (this.tzMinutes !== null) {
      const sign = this.tzMinutes < 0 ? '-' : '+';
      const total = Math.abs(this.tzMinutes);
      text += `${sign}${pad(Math.floor(total / 60), 2)}:${pad(total % 60, 2)}`;
    }
    return text;
  }
  utcKey() {
    const ms = Date.UTC(this.year, this.month - 1, this.day, this.hour, this.minute, this.second)
      - this.tzMinutes * 60000;
    return `${ms}.${pad(this.micro, 6)}`;
  }
}

function pad(n, width) {
  return String(n).padStart(width, '0');
}

function checkDate(year, month, day) {
  if (year < 1 || year > 9999) throw new Error(`year ${year} is out of range`);
  if (month < 1 || month > 12) throw new Error('month must be in 1..12');
  const leap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
  if (day < 1 || day > days) throw new Error('day is out of range for month');
}

class PyDict {
  constructor(entries = []) {
    this.map = new Map();
    for (const [k, v] of entries) this.set(k, v);
  }
  get size() {
    return this.map.size;
  }
  has(key) {
    return this.map.has(eqKey(key));
  }
  get(key, fallback = undefined) {
    const entry = this.map.get(eqKey(key));
    return entry === undefined ? fallback : entry[1];
  }
  set(key, value) {
    const k = eqKey(key);
    const entry = this.map.get(k);
    if (entry) entry[1] = value;
    else this.map.set(k, [key, value]);
  }
  delete(key) {
    this.map.delete(eqKey(key));
  }
  keys() {
    return [...this.map.values()].map((e) => e[0]);
  }
  entries() {
    return [...this.map.values()].map((e) => [e[0], e[1]]);
  }
  copy() {
    return new PyDict(this.entries());
  }
}

function eqKey(key) {
  if (typeof key === 'string') return `s${key}`;
  if (key === null) return 'z';
  if (typeof key === 'boolean') return key ? 'n1' : 'n0';
  if (typeof key === 'number' || typeof key === 'bigint') return `n${key}`;
  if (key instanceof PyFloat) {
    if (Number.isInteger(key.value)) return `n${BigInt(key.value)}`;
    if (Number.isNaN(key.value)) return `nan${Math.random()}`;
    return `f${key.value}`;
  }
  if (key instanceof PyDate) return `d${key.iso()}`;
  if (key instanceof PyDateTime) return key.tzMinutes === null ? `t${key.iso()}` : `T${key.utcKey()}`;
  throw new Error(`unhashable type: '${pyTypeName(key)}'`);
}

function pyTypeName(v) {
  if (Array.isArray(v)) return 'list';
  if (v instanceof PyDict) return 'dict';
  return typeof v;
}

const isInt = (v) => typeof v === 'bigint' || (typeof v === 'number' && Number.isInteger(v));
const isNumeric = (v) => isInt(v) || typeof v === 'boolean' || v instanceof PyFloat;

function toNumber(v) {
  if (typeof v === 'boolean') return v ? 1 : 0;
  if (typeof v === 'number') return v;
  if (typeof v === 'bigint') return Number(v);
  if (v instanceof PyFloat) return v.value;
  throw new Error(`unsupported operand type: '${pyTypeName(v)}'`);
}

function pyEq(a, b) {
  if (isNumeric(a) && isNumeric(b)) {
    const x = typeof a === 'bigint' ? a : toNumber(a);
    const y = typeof b === 'bigint' ? b : toNumber(b);
    // eslint-disable-next-line eqeqeq
    return x == y;
  }
  if (typeof a === 'string' || typeof b === 'string') return a === b;
  if (a === null || b === null) return a === b;
  if (Array.isArray(a) && Array.isArray(b)) return a.length === b.length && a.every((x, i) => pyEq(x, b[i]));
  if (a instanceof PyDict && b instanceof PyDict) {
    return a.size === b.size && a.entries().every(([k, v]) => b.has(k) && pyEq(v, b.get(k)));
  }
  if (a instanceof PyDate && b instanceof PyDate) return a.iso() === b.iso();
  if (a instanceof PyDateTime && b instanceof PyDateTime) {
    if ((a.tzMinutes === null) !== (b.tzMinutes === null)) return false;
    return a.tzMinutes === null ? a.iso() === b.iso() : a.utcKey() === b.utcKey();
  }
  return false;
}

function pyTruthy(v) {
  if (v === null || v === undefined || v === false) return false;
  if (typeof v === 'number') return v !== 0;
  if (typeof v === 'bigint') return v !== 0n;
  if (v instanceof PyFloat) return v.value !== 0;
  if (typeof v === 'string' || Array.isArray(v)) return v.length > 0;
  if (v instanceof PyDict) return v.size > 0;
  return true;
}

function pyLen(v) {
  if (typeof v === 'string') return [...v].length;
  if (Array.isArray(v)) return v.length;
  if (v instanceof PyDict) return v.size;
  throw new Error(`object of type '${pyTypeName(v)}' has no len()`);
}

function pyIn(item, container) {
  if (typeof container === 'string') {
    if (typeof item !== 'string') throw new Error("'in <string>' requires string as left operand");
    return container.includes(item);
  }
  if (Array.isArray(container)) return container.some((x) => pyEq(x, item));
  if (container instanceof PyDict) return container.has(item);
  throw new Error(`argument of type '${pyTypeName(container)}' is not iterable`);
}

// Whitespace as the reference runtime defines it for strip(), split() and \s.
const PY_SPACE = '\t\n\x0b\x0c\r\x1c\x1d\x1e\x1f \x85\xa0       '
  + '         　';
const PY_SPACE_CLASS = [...PY_SPACE].map((c) => `\\u{${c.codePointAt(0).toString(16)}}`).join('');
const PY_STRIP_RE = new RegExp(`^[${PY_SPACE_CLASS}]+|[${PY_SPACE_CLASS}]+$`, 'gu');
const PY_SPLIT_RE = new RegExp(`[${PY_SPACE_CLASS}]+`, 'u');

const pyStrip = (s) => s.replace(PY_STRIP_RE, '');
const pySplit = (s) => s.split(PY_SPLIT_RE).filter((part) => part !== '');

function isText(v) {
  return typeof v === 'string' && pyStrip(v) !== '';
}

function isTextList(v) {
  return Array.isArray(v) && v.length > 0 && v.every(isText);
}

// The value of a decimal digit of any script (\d of the reference regexes).
function digitValue(ch) {
  let k = 0;
  let cp = ch.codePointAt(0);
  while (/\p{Nd}/u.test(String.fromCodePoint(cp - 1))) {
    cp -= 1;
    k += 1;
  }
  return k % 10;
}

function pyIntOf(text) {
  let value = 0n;
  for (const ch of text) value = value * 10n + BigInt(digitValue(ch));
  return value;
}

// int() of a command-line value: whitespace, sign, digits of any script, single underscores.
function parseIntArgument(text) {
  const match = new RegExp(`^[${PY_SPACE_CLASS}]*([+-]?)(\\p{Nd}(?:_?\\p{Nd})*)[${PY_SPACE_CLASS}]*$`, 'u').exec(text);
  if (!match) return null;
  const value = pyIntOf(match[2].replace(/_/g, ''));
  return Number(match[1] === '-' ? -value : value);
}

// re.match with a trailing $, which also matches before one final newline.
function pyMatch(re, s) {
  if (typeof s !== 'string') return null;
  return re.exec(s) ?? (s.endsWith('\n') ? re.exec(s.slice(0, -1)) : null);
}

// ---------- number formatting and rounding of the reference runtime ----------

function floatRepr(x) {
  if (Number.isNaN(x)) return 'nan';
  if (x === Infinity) return 'inf';
  if (x === -Infinity) return '-inf';
  if (x === 0) return Object.is(x, -0) ? '-0.0' : '0.0';
  const sign = x < 0 ? '-' : '';
  const [mantissa, expText] = Math.abs(x).toExponential().split('e');
  const digits = mantissa.replace('.', '');
  const exp = Number(expText);
  const point = exp + 1;
  let body;
  if (point > -4 && point <= 16) {
    if (point <= 0) body = `0.${'0'.repeat(-point)}${digits}`;
    else if (point >= digits.length) body = `${digits}${'0'.repeat(point - digits.length)}.0`;
    else body = `${digits.slice(0, point)}.${digits.slice(point)}`;
  } else {
    const rest = digits.length > 1 ? `.${digits.slice(1)}` : '';
    body = `${digits[0]}${rest}e${exp < 0 ? '-' : '+'}${pad(Math.abs(exp), 2)}`;
  }
  return sign + body;
}

// round(x, ndigits): round half to even on the exact binary value, as the reference runtime does.
function pyRound(x, ndigits) {
  if (!Number.isFinite(x) || x === 0) return x;
  const view = new DataView(new ArrayBuffer(8));
  view.setFloat64(0, x);
  const bits = view.getBigUint64(0);
  const negative = bits >> 63n === 1n;
  const exponentBits = Number((bits >> 52n) & 0x7ffn);
  let mantissa = bits & ((1n << 52n) - 1n);
  let exponent;
  if (exponentBits === 0) exponent = -1074;
  else {
    mantissa |= 1n << 52n;
    exponent = exponentBits - 1075;
  }
  if (exponent >= 0) return x;
  const scale = 10n ** BigInt(ndigits);
  const denominator = 1n << BigInt(-exponent);
  const scaled = mantissa * scale;
  let quotient = scaled / denominator;
  const twice = 2n * (scaled % denominator);
  if (twice > denominator || (twice === denominator && quotient % 2n === 1n)) quotient += 1n;
  return Number(`${negative ? '-' : ''}${quotient}e-${ndigits}`);
}

// ---------- str() and repr() of loaded values, for messages ----------

function pyStr(v) {
  if (typeof v === 'string') return v;
  if (v instanceof PyDate) return v.iso();
  if (v instanceof PyDateTime) return v.iso(' ');
  return pyRepr(v);
}

function pyRepr(v) {
  if (v === null || v === undefined) return 'None';
  if (v === true) return 'True';
  if (v === false) return 'False';
  if (typeof v === 'number' || typeof v === 'bigint') return String(v);
  if (v instanceof PyFloat) return floatRepr(v.value);
  if (typeof v === 'string') return strRepr(v);
  if (Array.isArray(v)) return `[${v.map(pyRepr).join(', ')}]`;
  if (v instanceof PyDict) return `{${v.entries().map(([k, x]) => `${pyRepr(k)}: ${pyRepr(x)}`).join(', ')}}`;
  if (v instanceof PyDate) return `datetime.date(${v.year}, ${v.month}, ${v.day})`;
  if (v instanceof PyDateTime) {
    let text = `datetime.datetime(${v.year}, ${v.month}, ${v.day}, ${v.hour}, ${v.minute}`;
    if (v.micro) text += `, ${v.second}, ${v.micro}`;
    else if (v.second) text += `, ${v.second}`;
    if (v.tzMinutes !== null) text += `, tzinfo=${timezoneRepr(v.tzMinutes)}`;
    return `${text})`;
  }
  return String(v);
}

function timezoneRepr(minutes) {
  if (minutes === 0) return 'datetime.timezone.utc';
  const seconds = minutes * 60;
  const days = Math.floor(seconds / 86400);
  const rest = seconds - days * 86400;
  const parts = [];
  if (days) parts.push(`days=${days}`);
  if (rest) parts.push(`seconds=${rest}`);
  return `datetime.timezone(datetime.timedelta(${parts.join(', ')}))`;
}

const NON_PRINTABLE_RE = /[\p{Cc}\p{Cf}\p{Cs}\p{Co}\p{Cn}\p{Zl}\p{Zp}\p{Zs}]/u;

function strRepr(s) {
  const quote = s.includes("'") && !s.includes('"') ? '"' : "'";
  let out = quote;
  for (const ch of s) {
    const c = ch.codePointAt(0);
    if (ch === quote || ch === '\\') out += `\\${ch}`;
    else if (ch === '\t') out += '\\t';
    else if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (c < 0x20 || c === 0x7f) out += `\\x${c.toString(16).padStart(2, '0')}`;
    else if (c < 0x7f) out += ch;
    else if (ch !== ' ' && NON_PRINTABLE_RE.test(ch)) {
      if (c <= 0xff) out += `\\x${c.toString(16).padStart(2, '0')}`;
      else if (c <= 0xffff) out += `\\u${c.toString(16).padStart(4, '0')}`;
      else out += `\\U${c.toString(16).padStart(8, '0')}`;
    } else out += ch;
  }
  return out + quote;
}

// ---------- loader: YAML 1.1 scalar resolution of the reference loader ----------

// Implicit resolvers in registration order, each with the first characters it applies to.
const RESOLVERS = [
  ['bool', /^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$/, [...'yYnNtTfFoO']],
  ['float', /^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$/, [...'-+0123456789.']],
  ['int', /^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$/, [...'-+0123456789']],
  ['merge', /^(?:<<)$/, ['<']],
  ['null', /^(?:~|null|Null|NULL|)$/, ['~', 'n', 'N', '']],
  ['timestamp', /^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$/, [...'0123456789']],
  ['value', /^(?:=)$/, ['=']],
];

function resolveScalar(value) {
  const first = value === '' ? '' : String.fromCodePoint(value.codePointAt(0));
  for (const [kind, re, firsts] of RESOLVERS) {
    if (firsts.includes(first) && pyMatch(re, value)) return kind;
  }
  return 'str';
}

const TIMESTAMP_RE = /^([0-9][0-9][0-9][0-9])-([0-9][0-9]?)-([0-9][0-9]?)(?:(?:[Tt]|[ \t]+)([0-9][0-9]?):([0-9][0-9]):([0-9][0-9])(?:\.([0-9]*))?(?:[ \t]*(Z|([-+])([0-9][0-9]?)(?::([0-9][0-9]))?))?)?$/;

function constructInt(text) {
  let value = text.replace(/_/g, '');
  let sign = 1n;
  if (value[0] === '-') sign = -1n;
  if ('+-'.includes(value[0])) value = value.slice(1);
  let result;
  if (value === '0') result = 0n;
  else if (value.startsWith('0b')) result = sign * BigInt(`0b${value.slice(2)}`);
  else if (value.startsWith('0x')) result = sign * BigInt(`0x${value.slice(2)}`);
  else if (value[0] === '0') result = sign * BigInt(`0o${value}`);
  else if (value.includes(':')) {
    const digits = value.split(':').map((part) => BigInt(part)).reverse();
    let base = 1n;
    result = 0n;
    for (const digit of digits) {
      result += digit * base;
      base *= 60n;
    }
    result *= sign;
  } else result = sign * BigInt(value);
  return result >= BigInt(Number.MIN_SAFE_INTEGER) && result <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(result) : result;
}

function constructFloat(text) {
  let value = text.replace(/_/g, '').toLowerCase();
  let sign = 1;
  if (value[0] === '-') sign = -1;
  if ('+-'.includes(value[0])) value = value.slice(1);
  if (value === '.inf') return new PyFloat(sign * Infinity);
  if (value === '.nan') return new PyFloat(NaN);
  if (value.includes(':')) {
    const digits = value.split(':').map(parseFloatStrict).reverse();
    let base = 1;
    let result = 0.0;
    for (const digit of digits) {
      result += digit * base;
      base *= 60;
    }
    return new PyFloat(sign * result);
  }
  return new PyFloat(sign * parseFloatStrict(value));
}

function parseFloatStrict(text) {
  if (!/^(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:e[-+]?[0-9]+)?$/.test(text)) throw new Error(`could not convert string to float: '${text}'`);
  return Number(text.endsWith('.') ? text.slice(0, -1) : text.replace(/\.e/, 'e'));
}

function constructTimestamp(text) {
  const m = TIMESTAMP_RE.exec(text);
  if (!m) throw new Error(`invalid timestamp: '${text}'`);
  const [year, month, day] = [m[1], m[2], m[3]].map(Number);
  if (!m[4]) return new PyDate(year, month, day);
  let micro = 0;
  if (m[7]) micro = Number(m[7].slice(0, 6).padEnd(6, '0'));
  let tz = null;
  if (m[9]) {
    tz = Number(m[10]) * 60 + Number(m[11] || 0);
    if (m[9] === '-') tz = -tz;
  } else if (m[8]) tz = 0;
  return new PyDateTime(year, month, day, Number(m[4]), Number(m[5]), Number(m[6]), micro, tz);
}

// js-yaml turns mapping keys into strings. Typed scalars are therefore built as wrappers whose string
// form is a unique token; after loading, tokens are turned back into typed keys.
let tokenState = null;

class Loaded {
  constructor(kind, value) {
    this.kind = kind;
    this.value = value;
    this.token = null;
  }
  get [Symbol.toStringTag]() {
    return 'LoadedScalar';
  }
  toString() {
    if (this.token === null) {
      this.token = `\u0000${tokenState.nonce}:${tokenState.tokens.size}`;
      tokenState.tokens.set(this.token, this);
    }
    return this.token;
  }
}

function scalarType(kind, construct) {
  return new Type(`tag:yaml.org,2002:${kind}`, {
    kind: 'scalar',
    // Explicit tags construct whatever their constructor accepts; plain scalars use the resolvers.
    resolve: (data, explicitTag) => (explicitTag !== undefined ? true : resolveScalar(data ?? '') === kind),
    construct: (data) => new Loaded(kind, construct(data ?? '')),
  });
}

function privateKeyType(kind) {
  return new Type(`tag:distill-lab.local,2026:${kind}`, {
    kind: 'scalar',
    resolve: (data) => resolveScalar(data ?? '') === kind,
    construct: () => new Loaded(kind, null),
  });
}

const LOADER_SCHEMA = FAILSAFE_SCHEMA.extend({
  implicit: [
    scalarType('bool', (d) => {
      const lower = d.toLowerCase();
      if (['yes', 'true', 'on'].includes(lower)) return true;
      if (['no', 'false', 'off'].includes(lower)) return false;
      throw new Error(`invalid boolean: '${d}'`);
    }),
    scalarType('float', constructFloat),
    scalarType('int', constructInt),
    privateKeyType('merge'),
    scalarType('null', () => null),
    scalarType('timestamp', constructTimestamp),
    privateKeyType('value'),
  ],
});

function isMapping(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v) && !(v instanceof Loaded);
}

function convertLoaded(root) {
  const memo = new Map();
  const flat = new Map();

  const decodeKey = (raw) => {
    const wrapped = tokenState.tokens.get(raw);
    if (wrapped) return wrapped;
    if (raw.includes(tokenState.nonce)) throw new Error('found unhashable key');
    return raw;
  };

  // Merge keys (<<) put the merged pairs first, the later of several merged mappings losing.
  const flattenPairs = (obj) => {
    if (flat.has(obj)) return flat.get(obj);
    const merge = [];
    const own = [];
    for (const raw of Object.keys(obj)) {
      const key = decodeKey(raw);
      const value = obj[raw];
      if (key instanceof Loaded && key.kind === 'merge') {
        if (isMapping(value)) merge.push(...flattenPairs(value));
        else if (Array.isArray(value)) {
          const parts = value.map((item) => {
            if (!isMapping(item)) throw new Error('expected a mapping for merging');
            return flattenPairs(item);
          });
          parts.reverse();
          for (const part of parts) merge.push(...part);
        } else throw new Error('expected a mapping or list of mappings for merging');
      } else if (key instanceof Loaded && key.kind === 'value') own.push(['=', value]);
      else own.push([key instanceof Loaded ? key.value : key, value]);
    }
    const pairs = merge.concat(own);
    flat.set(obj, pairs);
    return pairs;
  };

  const convert = (v) => {
    if (v === null || v === undefined) return null;
    if (v instanceof Loaded) {
      if (v.kind === 'merge' || v.kind === 'value') {
        throw new Error(`could not determine a constructor for the tag 'tag:yaml.org,2002:${v.kind}'`);
      }
      return v.value;
    }
    if (typeof v === 'string') return v;
    if (memo.has(v)) return memo.get(v);
    if (Array.isArray(v)) {
      const out = [];
      memo.set(v, out);
      for (const item of v) out.push(convert(item));
      return out;
    }
    const out = new PyDict();
    memo.set(v, out);
    for (const [key, value] of flattenPairs(v)) out.set(key, convert(value));
    return out;
  };
  return convert(root);
}

// Strings are wrapped too, so every key becomes a token and keeps its place in the file (a plain
// object would move integer-like keys such as '123' to the front).
function wrapStrings(event, state) {
  if (event === 'close' && typeof state.result === 'string') state.result = new Loaded('str', state.result);
}

function parseYaml(text) {
  tokenState = { nonce: crypto.randomBytes(8).toString('hex'), tokens: new Map() };
  try {
    return convertLoaded(yamlLoad(text, { schema: LOADER_SCHEMA, listener: wrapStrings }));
  } finally {
    tokenState = null;
  }
}

function displayPath(p) {
  const absolute = path.isAbsolute(p);
  const parts = p.split(/[\\/]+/).filter((part) => part !== '' && part !== '.');
  const joined = parts.join(path.sep);
  return absolute ? (process.platform === 'win32' ? joined : path.sep + joined) : (joined || '.');
}

function readText(file) {
  let bytes;
  try {
    bytes = fs.readFileSync(file);
  } catch (error) {
    fatal(`cannot read ${displayPath(file)}`, error.message, 'pass the path of an existing distill.yaml');
  }
  let text;
  try {
    text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
  } catch {
    fatal(`${displayPath(file)} is not valid UTF-8`, 'distill.yaml is read as UTF-8 text', 'save the file as UTF-8 and rerun');
  }
  // Universal newlines, as the previous implementation read the file.
  return text.replace(/\r\n?/g, '\n');
}

export function loadText(text, file = 'distill.yaml') {
  try {
    return parseYaml(text);
  } catch (error) {
    throw new ExitError(1, `ERROR: ${displayPath(file)} is not valid YAML\nWHY: ${error.message}\nFIX: correct the syntax and rerun\n`);
  }
}

export function load(file) {
  return loadText(readText(file), file);
}

// ---------- validation ----------

function impactOf(score) {
  const facts = pyTruthy(score.get('facts')) ? score.get('facts') : [];
  return pyIn('a', facts) ? pyLen(facts) : 0;
}

function finalScore(lesson) {
  const s = lesson.get('score');
  if (!(s instanceof PyDict)) throw new Error(`lesson ${pyStr(lesson.get('key'))}: score missing`);
  const effort = toNumber(s.get('effort'));
  if (effort === 0) throw new Error('division by zero');
  const value = (toNumber(s.get('relevance')) * impactOf(s) * toNumber(s.get('evidence'))) / effort;
  return new PyFloat(pyRound(value, 2));
}

function compareCodePoints(a, b) {
  const x = [...a];
  const y = [...b];
  for (let i = 0; i < Math.min(x.length, y.length); i += 1) {
    const d = x[i].codePointAt(0) - y[i].codePointAt(0);
    if (d !== 0) return d;
  }
  return x.length - y.length;
}

function sortLessons(lessons) {
  if (!Array.isArray(lessons)) throw new Error('lessons: must be a list');
  const decorated = lessons.map((lesson) => {
    if (!(lesson instanceof PyDict)) throw new Error('lesson: must be a mapping');
    const layer = LAYERS.findIndex((name) => pyEq(name, lesson.get('layer') ?? null));
    if (layer < 0) throw new Error(`${pyRepr(lesson.get('layer') ?? null)} is not a known layer`);
    return { lesson, score: -finalScore(lesson).value, layer, key: lesson.get('key') };
  });
  decorated.sort((a, b) => {
    if (a.score !== b.score) return a.score < b.score ? -1 : 1;
    if (a.layer !== b.layer) return a.layer - b.layer;
    if (typeof a.key === 'string' && typeof b.key === 'string') return compareCodePoints(a.key, b.key);
    if (isNumeric(a.key) && isNumeric(b.key)) return toNumber(a.key) - toNumber(b.key);
    throw new Error("'<' not supported between the lesson keys");
  });
  return decorated.map((d) => d.lesson);
}

const KEY_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const COMMIT_WHERE_RE = /^([a-z0-9-]+)@([0-9a-f]{7,40})$/;
const PATH_WHERE_RE = new RegExp(`^([a-z0-9-]+)@([0-9a-f]{7,40}):([^#${PY_SPACE_CLASS}]+)(?:#L(\\p{Nd}+)(?:-L(\\p{Nd}+))?)?$`, 'u');
const ALSO_FITS_RE = /^(hub|new-skill:[a-z0-9-]+|[a-z0-9]+(?:-[a-z0-9]+)*)$/;
const STATUS_RE = /^(removed|superseded-by:[a-z0-9-]+)$/;
const SHA_RE = /^[0-9a-f]{7,40}$/;

const matchWhere = (ref) => pyMatch(COMMIT_WHERE_RE, ref) || pyMatch(PATH_WHERE_RE, ref);

function validate(data, errors) {
  if (!(data instanceof PyDict)) {
    errors.push('top level must be a mapping');
    return;
  }
  for (const name of data.keys()) {
    if (!TOP_ORDER.some((n) => pyEq(n, name))) errors.push(`unknown top-level key: ${pyStr(name)}`);
  }
  const goal = data.get('goal');
  if (!(goal instanceof PyDict)) errors.push('goal: missing');
  else {
    const status = goal.get('status', null);
    if (!['draft', 'confirmed'].some((s) => pyEq(s, status))) errors.push('goal.status must be draft or confirmed');
    if (!isText(goal.get('purpose'))) errors.push('goal.purpose must be non-empty text');
    for (const field of GOAL_ORDER.slice(2)) {
      if (!isTextList(goal.get(field))) errors.push(`goal.${field} must be a non-empty list of text`);
    }
  }
  const cursors = data.get('cursors');
  if (!(cursors instanceof PyDict) || cursors.size === 0) errors.push('cursors: must map source id to commit');
  else {
    for (const [src, commit] of cursors.entries()) {
      if (!(typeof commit === 'string' && SHA_RE.test(commit))) errors.push(`cursors.${pyStr(src)}: not a commit sha`);
    }
  }
  const lessons = data.get('lessons');
  if (!Array.isArray(lessons)) {
    errors.push('lessons: must be a list');
    return;
  }
  const seen = new Set();
  lessons.forEach((lesson, i) => {
    const index = i + 1;
    let where = `lesson ${index}`;
    if (!(lesson instanceof PyDict)) {
      errors.push(`${where}: must be a mapping`);
      return;
    }
    const key = lesson.get('key');
    if (!(typeof key === 'string' && pyMatch(KEY_RE, key))) errors.push(`${where}: key must be kebab-case`);
    else {
      where = `lesson ${index} (${key})`;
      if (seen.has(key)) errors.push(`${where}: duplicate key`);
      seen.add(key);
    }
    for (const name of lesson.keys()) {
      if (!LESSON_ORDER.some((n) => pyEq(n, name))) errors.push(`${where}: unknown field ${pyStr(name)}`);
    }
    if (!LAYERS.some((n) => pyEq(n, lesson.get('layer', null)))) errors.push(`${where}: layer must be one of ${LAYERS.join(', ')}`);
    for (const name of ['what', 'notable']) {
      if (!isText(lesson.get(name))) errors.push(`${where}: ${name} must be non-empty text`);
    }
    const refs = lesson.get('where');
    if (!(Array.isArray(refs) && refs.length > 0)) errors.push(`${where}: where must be a non-empty list`);
    else {
      for (const ref of refs) {
        if (!(typeof ref === 'string' && matchWhere(ref))) errors.push(`${where}: bad where ${pyRepr(ref)}`);
      }
    }
    if (!CONTRASTS.some((n) => pyEq(n, lesson.get('contrast', null)))) {
      errors.push(`${where}: contrast must be one of ${[...CONTRASTS].sort(compareCodePoints).join(', ')}`);
    }
    const score = lesson.get('score');
    if (!(score instanceof PyDict)) errors.push(`${where}: score missing`);
    else {
      for (const [name, low, high] of SCORE_RANGES) {
        const value = score.get(name);
        if (!(isInt(value) && low <= value && value <= high)) errors.push(`${where}: score.${name} must be an integer ${low}-${high}`);
      }
      const facts = score.get('facts');
      if (!(Array.isArray(facts) && facts.every((f) => typeof f === 'string' && IMPACT_FACTS.includes(f))
        && new Set(facts).size === facts.length)) {
        errors.push(`${where}: score.facts must be a list of distinct letters from a-e`);
      } else if (facts.length > 0 && !facts.includes('a')) {
        errors.push(`${where}: score.facts without fact a count for nothing; add a or clear the list`);
      }
      if (!isText(score.get('why'))) errors.push(`${where}: score.why must explain the weights`);
      for (const name of score.keys()) {
        if (!SCORE_ORDER.some((n) => pyEq(n, name))) errors.push(`${where}: unknown score field ${pyStr(name)}`);
      }
    }
    const fits = lesson.has('also_fits') ? lesson.get('also_fits') : [];
    if (!Array.isArray(fits) || fits.some((f) => !(typeof f === 'string' && pyMatch(ALSO_FITS_RE, f)))) {
      errors.push(`${where}: also_fits entries must be hub, new-skill:<name> or a skill id`);
    }
    if (lesson.has('status') && !(typeof lesson.get('status') === 'string' && pyMatch(STATUS_RE, lesson.get('status')))) {
      errors.push(`${where}: status must be removed or superseded-by:<key>`);
    }
    if (lesson.has('found_by') && !pyEq(lesson.get('found_by'), 'human')) errors.push(`${where}: found_by may only be human`);
    const decision = lesson.get('decision');
    if (!(decision instanceof PyDict) || !STATES.some((s) => pyEq(s, decision.get('state', null)))) {
      errors.push(`${where}: decision.state must be one of ${STATES.join(', ')}`);
    } else {
      if (decision.get('state') === 'rejected' && !isText(decision.get('reason'))) errors.push(`${where}: a rejected decision needs a reason`);
      for (const name of decision.keys()) {
        if (!DECISION_ORDER.some((n) => pyEq(n, name))) errors.push(`${where}: unknown decision field ${pyStr(name)}`);
      }
    }
  });
  if (cursors instanceof PyDict) {
    for (const lesson of lessons) {
      const refs = lesson instanceof PyDict ? lesson.get('where') : null;
      for (const ref of Array.isArray(refs) ? refs : []) {
        const match = typeof ref === 'string' ? matchWhere(ref) : null;
        if (match && !cursors.has(match[1])) {
          errors.push(`lesson ${pyStr(lesson.get('key', null))}: where cites unknown source ${strRepr(match[1])}`);
        }
      }
    }
  }
}

// ---------- evidence check against the source mirrors ----------

function gitEnvironment() {
  const env = { ...process.env, GIT_NO_LAZY_FETCH: '1', GIT_TERMINAL_PROMPT: '0', GIT_CONFIG_NOSYSTEM: '1' };
  for (const name of ['GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR', 'GIT_OBJECT_DIRECTORY',
    'GIT_ALTERNATE_OBJECT_DIRECTORIES', 'GIT_NAMESPACE']) {
    delete env[name];
  }
  return env;
}

const GIT_FIX = 'install Git 2.45.0 or newer, or add --no-where to skip the evidence check';
let gitReady = false;

function ensureGit() {
  if (gitReady) return;
  const r = spawnSync('git', ['--version'], { env: gitEnvironment(), timeout: GIT_TIMEOUT_MS, windowsHide: true });
  if (r.error || r.status !== 0) {
    fatal('git was not found or did not run', 'the evidence check reads the source mirrors with git', GIT_FIX);
  }
  const text = r.stdout.toString().trim();
  const m = /(\d+)\.(\d+)(?:\.(\d+))?/.exec(text);
  const version = m ? [Number(m[1]), Number(m[2]), Number(m[3] || 0)] : [0, 0, 0];
  const older = version[0] !== GIT_MIN[0] ? version[0] < GIT_MIN[0]
    : version[1] !== GIT_MIN[1] ? version[1] < GIT_MIN[1] : version[2] < GIT_MIN[2];
  if (older) {
    fatal(`Git ${m ? m[0] : text} is too old for an offline evidence check`,
      'the check passes --no-lazy-fetch so a partial clone never fetches; Git supports it from 2.45.0', GIT_FIX);
  }
  gitReady = true;
}

// Local git only: explicit git dir, no lazy fetch, no prompts, no system config, no ssh or fsmonitor
// command from a planted config, and a timeout per call.
function git(args) {
  ensureGit();
  const r = spawnSync('git', ['--no-lazy-fetch', '-c', 'safe.bareRepository=explicit', '-c', 'core.sshCommand=',
    '-c', 'core.fsmonitor=', ...args], {
    env: gitEnvironment(), timeout: GIT_TIMEOUT_MS, maxBuffer: 1024 * 1024 * 1024, windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (r.error && r.error.code === 'ETIMEDOUT') return { timedOut: true };
  if (r.error) return { status: -1, stdout: Buffer.alloc(0) };
  return { status: r.status, stdout: r.stdout };
}

function lineCount(text) {
  // Line boundaries of the reference runtime's splitlines(); \r\n counts once.
  let count = 0;
  let last = '';
  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i];
    if (ch === '\r' && text[i + 1] === '\n') continue;
    if ('\n\r\x0b\x0c\x1c\x1d\x1e\x85  '.includes(ch)) count += 1;
  }
  if (text.length > 0) {
    last = text[text.length - 1];
    if (!'\n\r\x0b\x0c\x1c\x1d\x1e\x85  '.includes(last)) count += 1;
  }
  return count;
}

function resolveMirror(hub, src, cache) {
  if (cache.has(src)) return cache.get(src);
  let result;
  let current = hub;
  for (const part of ['runtime', 'distill-lab', 'mirrors', src]) {
    current = path.join(current, part);
    let stat;
    try {
      stat = fs.lstatSync(current);
    } catch {
      result = { missing: true };
      break;
    }
    if (stat.isSymbolicLink()) {
      result = { refused: `${path.relative(hub, current).split(path.sep).join('/')} is a symbolic link; a mirror must be a real folder inside the hub` };
      break;
    }
    if (!stat.isDirectory()) {
      result = { missing: true };
      break;
    }
  }
  if (!result) {
    const tracked = git(['-C', hub, 'ls-files', '-z', '--', `runtime/distill-lab/mirrors/${src}`]);
    if (tracked.timedOut) result = { timedOut: true };
    else if (tracked.status === 0 && tracked.stdout.length > 0) {
      result = { refused: 'the hub repository tracks it; a mirror must be an untracked clone' };
    }
  }
  if (!result) {
    let gitDir = null;
    try {
      if (fs.lstatSync(path.join(current, '.git')).isDirectory()) gitDir = path.join(current, '.git');
    } catch { /* not a non-bare clone */ }
    if (gitDir === null) {
      try {
        if (fs.lstatSync(path.join(current, 'HEAD')).isFile()) gitDir = current;
      } catch { /* not a bare clone */ }
    }
    result = gitDir === null ? { missing: true } : { gitDir };
  }
  cache.set(src, result);
  return result;
}

function blobLines(mirror, commit, file) {
  const dir = `--git-dir=${mirror.gitDir}`;
  const r = git([dir, 'cat-file', 'blob', `${commit}:${file}`]);
  if (r.timedOut) return { timedOut: true };
  if (r.status === 0) return { lines: lineCount(new TextDecoder('utf-8').decode(r.stdout)) };
  const tree = git([dir, 'ls-tree', '-z', commit, '--', file]);
  if (tree.timedOut) return { timedOut: true };
  if (tree.status === 0) {
    for (const entry of tree.stdout.toString('utf8').split('\0')) {
      const m = /^\d+ blob ([0-9a-f]+)\t(.*)$/s.exec(entry);
      if (m && m[2] === file) {
        const present = git([dir, 'cat-file', '-e', m[1]]);
        if (present.timedOut) return { timedOut: true };
        if (present.status !== 0) return { notFetched: true };
      }
    }
  }
  return { unresolved: true };
}

function checkWhere(data, hub, errors) {
  const mirrors = new Map();
  const cache = new Map();
  const lessons = data.get('lessons', []);
  for (const lesson of lessons) {
    const key = lesson.get('key');
    for (const ref of lesson.get('where', [])) {
      const commitOnly = pyMatch(COMMIT_WHERE_RE, ref);
      if (commitOnly) {
        const [, src, commit] = commitOnly;
        const mirror = resolveMirror(hub, src, mirrors);
        let kind = null;
        if (mirror.timedOut) {
          errors.push(`${key}: ${ref}: git timed out in mirror ${src}`);
          continue;
        }
        if (mirror.gitDir) {
          const r = git([`--git-dir=${mirror.gitDir}`, 'cat-file', '-t', commit]);
          if (r.timedOut) {
            errors.push(`${key}: ${ref}: git timed out in mirror ${src}`);
            continue;
          }
          if (r.status === 0) kind = pyStrip(r.stdout.toString('utf8'));
        }
        if (kind !== 'commit') errors.push(`${key}: ${ref} is not a commit in mirror ${src}${refusal(mirror)}`);
        continue;
      }
      const match = pyMatch(PATH_WHERE_RE, ref);
      if (!match) continue;
      const [, src, commit, file, start, end] = match;
      const cacheKey = JSON.stringify([src, commit, file]);
      if (!cache.has(cacheKey)) {
        const mirror = resolveMirror(hub, src, mirrors);
        if (mirror.gitDir) cache.set(cacheKey, blobLines(mirror, commit, file));
        else cache.set(cacheKey, mirror.timedOut ? { timedOut: true } : { unresolved: true, mirror });
      }
      const found = cache.get(cacheKey);
      if (found.timedOut) errors.push(`${key}: ${ref}: git timed out in mirror ${src}`);
      else if (found.notFetched) errors.push(`${key}: ${ref}: blob not fetched; run git fetch in the mirror`);
      else if (found.unresolved) errors.push(`${key}: ${ref} does not resolve in mirror ${src}${refusal(found.mirror)}`);
      else {
        const last = end || start;
        if ((last ? pyIntOf(last) : 0n) > BigInt(found.lines)) errors.push(`${key}: ${ref} line range exceeds ${found.lines} lines`);
        else if (start && end && pyIntOf(end) < pyIntOf(start)) errors.push(`${key}: ${ref} line range is reversed`);
      }
    }
  }
}

function refusal(mirror) {
  return mirror && mirror.refused ? ` (refused: ${mirror.refused})` : '';
}

// ---------- hub lookup ----------

function isHub(dir) {
  try {
    return fs.statSync(path.join(dir, '.skillhub', 'schema-version')).isFile();
  } catch {
    return false;
  }
}

function parentsUp(p, levels) {
  let current = p;
  for (let i = 0; i < levels; i += 1) current = path.dirname(current);
  return current;
}

// <hub>/skills/<collection>/<skill-id>/.meta/distill.yaml
function hubOf(file) {
  let real;
  try {
    real = fs.realpathSync(file);
  } catch {
    return null;
  }
  const hub = parentsUp(real, 5);
  return isHub(hub) ? hub : null;
}

function requireHub(file) {
  const hub = hubOf(file);
  if (hub === null) {
    fatal(`${displayPath(file)} is not inside a Skill Hub folder`,
      'the evidence check reads source mirrors under <hub>/runtime/distill-lab/mirrors, and the hub is found from '
      + "the file's own path (<hub>/skills/<collection>/<skill>/.meta/distill.yaml)",
      'run it on the distill.yaml inside the hub, or add --no-where');
  }
  return hub;
}

function assertHubOption(file, hubOption) {
  if (hubOption === null) return;
  let wanted;
  try {
    wanted = fs.realpathSync(hubOption);
  } catch {
    wanted = null;
  }
  const actual = hubOf(file);
  if (wanted === null || actual === null || wanted !== actual) {
    fatal(`--hub ${hubOption} is not the hub that holds ${displayPath(file)}`,
      actual === null ? 'the file is not inside a Skill Hub folder' : `the file's own path places it in the hub ${actual}`,
      'drop --hub, or pass the folder that holds the file as <hub>/skills/<collection>/<skill>/.meta/distill.yaml');
  }
}

// ---------- canonical writer ----------
// The layout is fixed: decision and score.facts in flow style, prose over 60 characters folded,
// everything else in block style. The emitter below is a port of the reference emitter restricted
// to block output with indent 2, width 88 and unicode allowed.

class Folded {
  constructor(text) {
    this.text = text;
  }
}

const FLOW = new WeakSet();

function flow(value) {
  FLOW.add(value);
  return value;
}

function ordered(mapping, order) {
  const out = new PyDict();
  for (const k of order) if (mapping.has(k)) out.set(k, mapping.get(k));
  for (const [k, v] of mapping.entries()) if (!out.has(k)) out.set(k, v);
  return out;
}

function prose(value) {
  return typeof value === 'string' && pyLen(value) > 60 ? new Folded(pySplit(value).join(' ')) : value;
}

const HEADER = `# Written by distill-lab/scripts/${SCRIPT}. Edit freely, then run \`${SCRIPT} format\`.\n`
  + '# impact = number of facts (a-e, a required); final_score = relevance x impact x evidence / effort;\n'
  + '# lessons are sorted by it,\n'
  + '# then by layer rank, then by key.\n';

function canonical(data) {
  const goal = ordered(data.get('goal'), GOAL_ORDER);
  goal.set('purpose', prose(goal.get('purpose')));
  const lessons = [];
  for (const lesson of sortLessons(data.get('lessons'))) {
    const item = lesson.copy();
    item.set('final_score', finalScore(lesson));
    item.set('what', prose(item.get('what')));
    item.set('notable', prose(item.get('notable')));
    item.set('score', item.get('score').copy());
    const facts = item.get('score').get('facts');
    item.get('score').set('facts', flow([...(pyTruthy(facts) ? facts : [])].sort(compareCodePoints)));
    item.get('score').set('impact', impactOf(item.get('score')));
    item.set('score', ordered(item.get('score'), SCORE_ORDER));
    item.get('score').set('why', prose(item.get('score').get('why')));
    item.set('decision', flow(ordered(item.get('decision'), DECISION_ORDER)));
    if (!item.has('also_fits')) item.set('also_fits', []);
    lessons.push(ordered(item, LESSON_ORDER));
  }
  const out = new PyDict([['goal', goal], ['cursors', data.get('cursors')]]);
  if (data.has('coverage')) out.set('coverage', data.get('coverage'));
  out.set('lessons', lessons);
  const body = new Emitter().emit(serialize(out)).replace(/\n( {2}- key: )/g, '\n\n$1');
  return HEADER + body;
}

const TAG = 'tag:yaml.org,2002:';

// Representation: values to nodes; repeated collections and timestamps share one node (an anchor).
function serialize(data) {
  const represented = new Map();
  const represent = (v) => {
    const tracked = Array.isArray(v) || v instanceof PyDict || v instanceof PyDate || v instanceof PyDateTime;
    if (tracked && represented.has(v)) return represented.get(v);
    let node;
    if (v === null || v === undefined) node = scalarNode('null', 'null');
    else if (typeof v === 'string') node = scalarNode('str', v);
    else if (v instanceof Folded) node = scalarNode('str', v.text, '>');
    else if (typeof v === 'boolean') node = scalarNode('bool', v ? 'true' : 'false');
    else if (typeof v === 'number' || typeof v === 'bigint') node = scalarNode('int', String(v));
    else if (v instanceof PyFloat) node = scalarNode('float', representFloat(v.value));
    else if (v instanceof PyDate) node = scalarNode('timestamp', v.iso());
    else if (v instanceof PyDateTime) node = scalarNode('timestamp', v.iso(' '));
    else if (Array.isArray(v)) {
      node = { kind: 'seq', tag: `${TAG}seq`, items: [], flow: FLOW.has(v) };
      represented.set(v, node);
      for (const item of v) node.items.push(represent(item));
    } else if (v instanceof PyDict) {
      node = { kind: 'map', tag: `${TAG}map`, pairs: [], flow: FLOW.has(v) };
      represented.set(v, node);
      for (const [k, x] of v.entries()) node.pairs.push([represent(k), represent(x)]);
    } else throw new Error(`cannot represent an object: ${String(v)}`);
    if (tracked) represented.set(v, node);
    return node;
  };
  const root = represent(data);

  const anchors = new Map();
  let lastAnchor = 0;
  const anchorNode = (node) => {
    if (anchors.has(node)) {
      if (anchors.get(node) === null) anchors.set(node, `id${pad(++lastAnchor, 3)}`);
      return;
    }
    anchors.set(node, null);
    if (node.kind === 'seq') node.items.forEach(anchorNode);
    if (node.kind === 'map') node.pairs.forEach(([k, x]) => { anchorNode(k); anchorNode(x); });
  };
  anchorNode(root);

  const events = [{ type: 'StreamStart' }, { type: 'DocumentStart' }];
  const serialized = new Set();
  const serializeNode = (node) => {
    const anchor = anchors.get(node);
    if (serialized.has(node)) {
      events.push({ type: 'Alias', anchor });
      return;
    }
    serialized.add(node);
    if (node.kind === 'scalar') {
      const detected = `${TAG}${resolveScalar(node.value)}`;
      events.push({ type: 'Scalar', anchor, tag: node.tag, implicit: [node.tag === detected, node.tag === `${TAG}str`], value: node.value, style: node.style });
    } else if (node.kind === 'seq') {
      events.push({ type: 'SequenceStart', anchor, tag: node.tag, implicit: true, flowStyle: node.flow });
      node.items.forEach(serializeNode);
      events.push({ type: 'SequenceEnd' });
    } else {
      events.push({ type: 'MappingStart', anchor, tag: node.tag, implicit: true, flowStyle: node.flow });
      for (const [k, x] of node.pairs) {
        serializeNode(k);
        serializeNode(x);
      }
      events.push({ type: 'MappingEnd' });
    }
  };
  serializeNode(root);
  events.push({ type: 'DocumentEnd' }, { type: 'StreamEnd' });
  return events;
}

function scalarNode(kind, value, style = null) {
  return { kind: 'scalar', tag: `${TAG}${kind}`, value, style };
}

function representFloat(x) {
  if (Number.isNaN(x)) return '.nan';
  if (x === Infinity) return '.inf';
  if (x === -Infinity) return '-.inf';
  let value = floatRepr(x).toLowerCase();
  if (!value.includes('.') && value.includes('e')) value = value.replace('e', '.0e');
  return value;
}

const isBreak = (ch) => ch === '\n' || ch === '\x85' || ch === ' ' || ch === ' ';
const isSpaceOrBreak = (ch) => ch === '\0' || ch === ' ' || ch === '\t' || ch === '\r' || isBreak(ch);
const isNodeEvent = (e) => ['Alias', 'Scalar', 'SequenceStart', 'MappingStart'].includes(e.type);
const isCollectionStart = (e) => e.type === 'SequenceStart' || e.type === 'MappingStart';

const ESCAPES = {
  '\0': '0', '\x07': 'a', '\x08': 'b', '\x09': 't', '\x0A': 'n', '\x0B': 'v', '\x0C': 'f', '\x0D': 'r',
  '\x1B': 'e', '"': '"', '\\': '\\', '\x85': 'N', '\xA0': '_', ' ': 'L', ' ': 'P',
};

class Emitter {
  constructor() {
    this.out = '';
    this.states = [];
    this.events = [];
    this.next = 0;
    this.event = null;
    this.indents = [];
    this.indent = null;
    this.flowLevel = 0;
    this.rootContext = false;
    this.sequenceContext = false;
    this.mappingContext = false;
    this.simpleKeyContext = false;
    this.column = 0;
    this.whitespace = true;
    this.indention = true;
    this.openEnded = false;
    this.bestIndent = 2;
    this.bestWidth = 88;
    this.preparedAnchor = null;
    this.preparedTag = null;
    this.analysis = null;
    this.style = null;
  }

  emit(events) {
    this.events = events;
    this.state = () => this.expectStreamStart();
    while (this.next < events.length) {
      this.event = events[this.next];
      this.next += 1;
      this.state();
      this.event = null;
    }
    return this.out;
  }

  peek() {
    return this.events[this.next];
  }

  increaseIndent(flowContext = false, indentless = false) {
    this.indents.push(this.indent);
    if (this.indent === null) this.indent = flowContext ? this.bestIndent : 0;
    else if (!indentless) this.indent += this.bestIndent;
  }

  expectStreamStart() {
    this.state = () => this.expectDocumentStart(true);
  }

  expectDocumentStart(first = false) {
    if (this.event.type === 'DocumentStart') {
      const implicit = first && !this.checkEmptyDocument();
      if (!implicit) {
        this.writeIndent();
        this.writeIndicator('---', true);
      }
      this.state = () => this.expectDocumentRoot();
    } else if (this.event.type === 'StreamEnd') {
      if (this.openEnded) {
        this.writeIndicator('...', true);
        this.writeIndent();
      }
      this.state = () => { throw new Error('expected nothing'); };
    }
  }

  expectDocumentEnd() {
    this.writeIndent();
    this.state = () => this.expectDocumentStart();
  }

  expectDocumentRoot() {
    this.states.push(() => this.expectDocumentEnd());
    this.expectNode({ root: true });
  }

  expectNode({ root = false, sequence = false, mapping = false, simpleKey = false } = {}) {
    this.rootContext = root;
    this.sequenceContext = sequence;
    this.mappingContext = mapping;
    this.simpleKeyContext = simpleKey;
    const { type } = this.event;
    if (type === 'Alias') {
      this.processAnchor('*');
      this.state = this.states.pop();
    } else if (type === 'Scalar' || isCollectionStart(this.event)) {
      this.processAnchor('&');
      this.processTag();
      if (type === 'Scalar') {
        this.increaseIndent(true);
        this.processScalar();
        this.indent = this.indents.pop();
        this.state = this.states.pop();
      } else if (type === 'SequenceStart') {
        if (this.flowLevel || this.event.flowStyle || this.checkEmptySequence()) this.expectFlowSequence();
        else this.expectBlockSequence();
      } else if (this.flowLevel || this.event.flowStyle || this.checkEmptyMapping()) this.expectFlowMapping();
      else this.expectBlockMapping();
    } else throw new Error(`expected NodeEvent, but got ${type}`);
  }

  expectFlowSequence() {
    this.writeIndicator('[', true, true);
    this.flowLevel += 1;
    this.increaseIndent(true);
    this.state = () => this.expectFlowSequenceItem(true);
  }

  expectFlowSequenceItem(first = false) {
    if (this.event.type === 'SequenceEnd') {
      this.indent = this.indents.pop();
      this.flowLevel -= 1;
      this.writeIndicator(']', false);
      this.state = this.states.pop();
    } else {
      if (!first) this.writeIndicator(',', false);
      if (this.column > this.bestWidth) this.writeIndent();
      this.states.push(() => this.expectFlowSequenceItem());
      this.expectNode({ sequence: true });
    }
  }

  expectFlowMapping() {
    this.writeIndicator('{', true, true);
    this.flowLevel += 1;
    this.increaseIndent(true);
    this.state = () => this.expectFlowMappingKey(true);
  }

  expectFlowMappingKey(first = false) {
    if (this.event.type === 'MappingEnd') {
      this.indent = this.indents.pop();
      this.flowLevel -= 1;
      this.writeIndicator('}', false);
      this.state = this.states.pop();
    } else {
      if (!first) this.writeIndicator(',', false);
      if (this.column > this.bestWidth) this.writeIndent();
      if (this.checkSimpleKey()) {
        this.states.push(() => this.expectFlowMappingSimpleValue());
        this.expectNode({ mapping: true, simpleKey: true });
      } else {
        this.writeIndicator('?', true);
        this.states.push(() => this.expectFlowMappingValue());
        this.expectNode({ mapping: true });
      }
    }
  }

  expectFlowMappingSimpleValue() {
    this.writeIndicator(':', false);
    this.states.push(() => this.expectFlowMappingKey());
    this.expectNode({ mapping: true });
  }

  expectFlowMappingValue() {
    if (this.column > this.bestWidth) this.writeIndent();
    this.writeIndicator(':', true);
    this.states.push(() => this.expectFlowMappingKey());
    this.expectNode({ mapping: true });
  }

  expectBlockSequence() {
    // Sequences are indented under their key (the reference dumper never uses indentless sequences).
    this.increaseIndent(false, false);
    this.state = () => this.expectBlockSequenceItem(true);
  }

  expectBlockSequenceItem(first = false) {
    if (!first && this.event.type === 'SequenceEnd') {
      this.indent = this.indents.pop();
      this.state = this.states.pop();
    } else {
      this.writeIndent();
      this.writeIndicator('-', true, false, true);
      this.states.push(() => this.expectBlockSequenceItem());
      this.expectNode({ sequence: true });
    }
  }

  expectBlockMapping() {
    this.increaseIndent(false);
    this.state = () => this.expectBlockMappingKey(true);
  }

  expectBlockMappingKey(first = false) {
    if (!first && this.event.type === 'MappingEnd') {
      this.indent = this.indents.pop();
      this.state = this.states.pop();
    } else {
      this.writeIndent();
      if (this.checkSimpleKey()) {
        this.states.push(() => this.expectBlockMappingSimpleValue());
        this.expectNode({ mapping: true, simpleKey: true });
      } else {
        this.writeIndicator('?', true, false, true);
        this.states.push(() => this.expectBlockMappingValue());
        this.expectNode({ mapping: true });
      }
    }
  }

  expectBlockMappingSimpleValue() {
    this.writeIndicator(':', false);
    this.states.push(() => this.expectBlockMappingKey());
    this.expectNode({ mapping: true });
  }

  expectBlockMappingValue() {
    this.writeIndent();
    this.writeIndicator(':', true, false, true);
    this.states.push(() => this.expectBlockMappingKey());
    this.expectNode({ mapping: true });
  }

  checkEmptySequence() {
    return this.event.type === 'SequenceStart' && this.peek() && this.peek().type === 'SequenceEnd';
  }

  checkEmptyMapping() {
    return this.event.type === 'MappingStart' && this.peek() && this.peek().type === 'MappingEnd';
  }

  checkEmptyDocument() {
    const e = this.peek();
    return this.event.type === 'DocumentStart' && !!e && e.type === 'Scalar' && e.anchor == null
      && e.tag == null && e.implicit && e.value === '';
  }

  checkSimpleKey() {
    let length = 0;
    if (isNodeEvent(this.event) && this.event.anchor != null) {
      if (this.preparedAnchor === null) this.preparedAnchor = this.event.anchor;
      length += this.preparedAnchor.length;
    }
    if ((this.event.type === 'Scalar' || isCollectionStart(this.event)) && this.event.tag != null) {
      if (this.preparedTag === null) this.preparedTag = prepareTag(this.event.tag);
      length += [...this.preparedTag].length;
    }
    if (this.event.type === 'Scalar') {
      if (this.analysis === null) this.analysis = analyzeScalar(this.event.value);
      length += this.analysis.chars.length;
    }
    return length < 128 && (this.event.type === 'Alias'
      || (this.event.type === 'Scalar' && !this.analysis.empty && !this.analysis.multiline)
      || this.checkEmptySequence() || this.checkEmptyMapping());
  }

  processAnchor(indicator) {
    if (this.event.anchor == null) {
      this.preparedAnchor = null;
      return;
    }
    if (this.preparedAnchor === null) this.preparedAnchor = this.event.anchor;
    if (this.preparedAnchor) this.writeIndicator(indicator + this.preparedAnchor, true);
    this.preparedAnchor = null;
  }

  processTag() {
    let tag = this.event.tag;
    if (this.event.type === 'Scalar') {
      if (this.style === null) this.style = this.chooseScalarStyle();
      if ((this.style === '' && this.event.implicit[0]) || (this.style !== '' && this.event.implicit[1])) {
        this.preparedTag = null;
        return;
      }
      if (this.event.implicit[0] && tag == null) {
        tag = '!';
        this.preparedTag = null;
      }
    } else if (this.event.implicit) {
      this.preparedTag = null;
      return;
    }
    if (this.preparedTag === null) this.preparedTag = prepareTag(tag);
    if (this.preparedTag) this.writeIndicator(this.preparedTag, true);
    this.preparedTag = null;
  }

  chooseScalarStyle() {
    if (this.analysis === null) this.analysis = analyzeScalar(this.event.value);
    const { style, implicit } = this.event;
    const a = this.analysis;
    if (style === '"') return '"';
    if (!style && implicit[0]) {
      if (!(this.simpleKeyContext && (a.empty || a.multiline))
        && ((this.flowLevel && a.allowFlowPlain) || (!this.flowLevel && a.allowBlockPlain))) return '';
    }
    if (style && '|>'.includes(style)) {
      if (!this.flowLevel && !this.simpleKeyContext && a.allowBlock) return style;
    }
    if (!style || style === "'") {
      if (a.allowSingleQuoted && !(this.simpleKeyContext && a.multiline)) return "'";
    }
    return '"';
  }

  processScalar() {
    if (this.analysis === null) this.analysis = analyzeScalar(this.event.value);
    if (this.style === null) this.style = this.chooseScalarStyle();
    const split = !this.simpleKeyContext;
    const chars = this.analysis.chars;
    if (this.style === '"') this.writeDoubleQuoted(chars, split);
    else if (this.style === "'") this.writeSingleQuoted(chars, split);
    else if (this.style === '>') this.writeFolded(chars);
    else if (this.style === '|') this.writeLiteral(chars);
    else this.writePlain(chars, split);
    this.analysis = null;
    this.style = null;
  }

  write(data, length) {
    this.out += data;
    this.column += length;
  }

  writeIndicator(indicator, needWhitespace, whitespace = false, indention = false) {
    const data = this.whitespace || !needWhitespace ? indicator : ` ${indicator}`;
    this.whitespace = whitespace;
    this.indention = this.indention && indention;
    this.write(data, [...data].length);
    this.openEnded = false;
  }

  writeIndent() {
    const indent = this.indent || 0;
    if (!this.indention || this.column > indent || (this.column === indent && !this.whitespace)) this.writeLineBreak();
    if (this.column < indent) {
      this.whitespace = true;
      this.write(' '.repeat(indent - this.column), indent - this.column);
    }
  }

  writeLineBreak(data = '\n') {
    this.whitespace = true;
    this.indention = true;
    this.column = 0;
    this.out += data;
  }

  writeChunk(chars, start, end) {
    this.write(chars.slice(start, end).join(''), end - start);
  }

  writeBreaks(chars, start, end) {
    for (const br of chars.slice(start, end)) {
      if (br === '\n') this.writeLineBreak();
      else this.writeLineBreak(br);
    }
  }

  writeSingleQuoted(t, split) {
    this.writeIndicator("'", true);
    let spaces = false;
    let breaks = false;
    let start = 0;
    let end = 0;
    while (end <= t.length) {
      const ch = end < t.length ? t[end] : null;
      if (spaces) {
        if (ch === null || ch !== ' ') {
          if (start + 1 === end && this.column > this.bestWidth && split && start !== 0 && end !== t.length) this.writeIndent();
          else this.writeChunk(t, start, end);
          start = end;
        }
      } else if (breaks) {
        if (ch === null || !isBreak(ch)) {
          if (t[start] === '\n') this.writeLineBreak();
          this.writeBreaks(t, start, end);
          this.writeIndent();
          start = end;
        }
      } else if (ch === null || ch === ' ' || isBreak(ch) || ch === "'") {
        if (start < end) {
          this.writeChunk(t, start, end);
          start = end;
        }
      }
      if (ch === "'") {
        this.write("''", 2);
        start = end + 1;
      }
      if (ch !== null) {
        spaces = ch === ' ';
        breaks = isBreak(ch);
      }
      end += 1;
    }
    this.writeIndicator("'", false);
  }

  writeDoubleQuoted(t, split) {
    this.writeIndicator('"', true);
    let start = 0;
    let end = 0;
    while (end <= t.length) {
      const ch = end < t.length ? t[end] : null;
      const c = ch === null ? -1 : ch.codePointAt(0);
      if (ch === null || '"\\\x85  ﻿'.includes(ch)
        || !((c >= 0x20 && c <= 0x7e) || (c >= 0xa0 && c <= 0xd7ff) || (c >= 0xe000 && c <= 0xfffd))) {
        if (start < end) {
          this.writeChunk(t, start, end);
          start = end;
        }
        if (ch !== null) {
          let data;
          if (ESCAPES[ch] !== undefined) data = `\\${ESCAPES[ch]}`;
          else if (c <= 0xff) data = `\\x${c.toString(16).toUpperCase().padStart(2, '0')}`;
          else if (c <= 0xffff) data = `\\u${c.toString(16).toUpperCase().padStart(4, '0')}`;
          else data = `\\U${c.toString(16).toUpperCase().padStart(8, '0')}`;
          this.write(data, data.length);
          start = end + 1;
        }
      }
      if (end > 0 && end < t.length - 1 && (ch === ' ' || start >= end)
        && this.column + (end - start) > this.bestWidth && split) {
        const data = `${t.slice(start, end).join('')}\\`;
        const length = Math.max(end - start, 0) + 1;
        if (start < end) start = end;
        this.write(data, length);
        this.writeIndent();
        this.whitespace = false;
        this.indention = false;
        if (t[start] === ' ') this.write('\\', 1);
      }
      end += 1;
    }
    this.writeIndicator('"', false);
  }

  blockHints(t) {
    let hints = '';
    if (t.length) {
      if (t[0] === ' ' || isBreak(t[0])) hints += String(this.bestIndent);
      if (!isBreak(t[t.length - 1])) hints += '-';
      else if (t.length === 1 || isBreak(t[t.length - 2])) hints += '+';
    }
    return hints;
  }

  writeFolded(t) {
    const hints = this.blockHints(t);
    this.writeIndicator(`>${hints}`, true);
    if (hints.endsWith('+')) this.openEnded = true;
    this.writeLineBreak();
    let leadingSpace = true;
    let spaces = false;
    let breaks = true;
    let start = 0;
    let end = 0;
    while (end <= t.length) {
      const ch = end < t.length ? t[end] : null;
      if (breaks) {
        if (ch === null || !isBreak(ch)) {
          if (!leadingSpace && ch !== null && ch !== ' ' && t[start] === '\n') this.writeLineBreak();
          leadingSpace = ch === ' ';
          this.writeBreaks(t, start, end);
          if (ch !== null) this.writeIndent();
          start = end;
        }
      } else if (spaces) {
        if (ch !== ' ') {
          if (start + 1 === end && this.column > this.bestWidth) this.writeIndent();
          else this.writeChunk(t, start, end);
          start = end;
        }
      } else if (ch === null || ch === ' ' || isBreak(ch)) {
        this.writeChunk(t, start, end);
        if (ch === null) this.writeLineBreak();
        start = end;
      }
      if (ch !== null) {
        breaks = isBreak(ch);
        spaces = ch === ' ';
      }
      end += 1;
    }
  }

  writeLiteral(t) {
    const hints = this.blockHints(t);
    this.writeIndicator(`|${hints}`, true);
    if (hints.endsWith('+')) this.openEnded = true;
    this.writeLineBreak();
    let breaks = true;
    let start = 0;
    let end = 0;
    while (end <= t.length) {
      const ch = end < t.length ? t[end] : null;
      if (breaks) {
        if (ch === null || !isBreak(ch)) {
          this.writeBreaks(t, start, end);
          if (ch !== null) this.writeIndent();
          start = end;
        }
      } else if (ch === null || isBreak(ch)) {
        this.out += t.slice(start, end).join('');
        if (ch === null) this.writeLineBreak();
        start = end;
      }
      if (ch !== null) breaks = isBreak(ch);
      end += 1;
    }
  }

  writePlain(t, split) {
    if (this.rootContext) this.openEnded = true;
    if (!t.length) return;
    if (!this.whitespace) this.write(' ', 1);
    this.whitespace = false;
    this.indention = false;
    let spaces = false;
    let breaks = false;
    let start = 0;
    let end = 0;
    while (end <= t.length) {
      const ch = end < t.length ? t[end] : null;
      if (spaces) {
        if (ch !== ' ') {
          if (start + 1 === end && this.column > this.bestWidth && split) {
            this.writeIndent();
            this.whitespace = false;
            this.indention = false;
          } else this.writeChunk(t, start, end);
          start = end;
        }
      } else if (breaks) {
        if (ch === null || !isBreak(ch)) {
          if (t[start] === '\n') this.writeLineBreak();
          this.writeBreaks(t, start, end);
          this.writeIndent();
          this.whitespace = false;
          this.indention = false;
          start = end;
        }
      } else if (ch === null || ch === ' ' || isBreak(ch)) {
        this.writeChunk(t, start, end);
        start = end;
      }
      if (ch !== null) {
        spaces = ch === ' ';
        breaks = isBreak(ch);
      }
      end += 1;
    }
  }
}

function prepareTag(tag) {
  if (tag === '!') return tag;
  let handle = null;
  let suffix = tag;
  for (const [prefix, h] of [['!', '!'], [TAG, '!!']]) {
    if (tag.startsWith(prefix) && (prefix === '!' || prefix.length < tag.length)) {
      handle = h;
      suffix = tag.slice(prefix.length);
    }
  }
  let text = '';
  for (const ch of suffix) {
    if (/[0-9A-Za-z\-;/?:@&=+$,_.~*'()[\]]/.test(ch) || (ch === '!' && handle !== '!')) text += ch;
    else for (const byte of Buffer.from(ch, 'utf8')) text += `%${byte.toString(16).toUpperCase().padStart(2, '0')}`;
  }
  return handle ? `${handle}${text}` : `!<${text}>`;
}

function analyzeScalar(scalar) {
  const chars = [...scalar];
  const n = chars.length;
  if (n === 0) {
    return { chars, empty: true, multiline: false, allowFlowPlain: false, allowBlockPlain: true, allowSingleQuoted: true, allowBlock: false };
  }
  let blockIndicators = false;
  let flowIndicators = false;
  let lineBreaks = false;
  let specialCharacters = false;
  let leadingSpace = false;
  let leadingBreak = false;
  let trailingSpace = false;
  let trailingBreak = false;
  let breakSpace = false;
  let spaceBreak = false;
  if (scalar.startsWith('---') || scalar.startsWith('...')) {
    blockIndicators = true;
    flowIndicators = true;
  }
  let precededByWhitespace = true;
  let followedByWhitespace = n === 1 || isSpaceOrBreak(chars[1]);
  let previousSpace = false;
  let previousBreak = false;
  for (let index = 0; index < n;) {
    const ch = chars[index];
    if (index === 0) {
      if ('#,[]{}&*!|>\'"%@`'.includes(ch)) {
        flowIndicators = true;
        blockIndicators = true;
      }
      if (ch === '?' || ch === ':') {
        flowIndicators = true;
        if (followedByWhitespace) blockIndicators = true;
      }
      if (ch === '-' && followedByWhitespace) {
        flowIndicators = true;
        blockIndicators = true;
      }
    } else {
      if (',?[]{}'.includes(ch)) flowIndicators = true;
      if (ch === ':') {
        flowIndicators = true;
        if (followedByWhitespace) blockIndicators = true;
      }
      if (ch === '#' && precededByWhitespace) {
        flowIndicators = true;
        blockIndicators = true;
      }
    }
    if (isBreak(ch)) lineBreaks = true;
    const c = ch.codePointAt(0);
    if (!(ch === '\n' || (c >= 0x20 && c <= 0x7e))) {
      const unicode = (c === 0x85 || (c >= 0xa0 && c <= 0xd7ff) || (c >= 0xe000 && c <= 0xfffd)
        || (c >= 0x10000 && c < 0x10ffff)) && c !== 0xfeff;
      if (!unicode) specialCharacters = true;
    }
    if (ch === ' ') {
      if (index === 0) leadingSpace = true;
      if (index === n - 1) trailingSpace = true;
      if (previousBreak) breakSpace = true;
      previousSpace = true;
      previousBreak = false;
    } else if (isBreak(ch)) {
      if (index === 0) leadingBreak = true;
      if (index === n - 1) trailingBreak = true;
      if (previousSpace) spaceBreak = true;
      previousSpace = false;
      previousBreak = true;
    } else {
      previousSpace = false;
      previousBreak = false;
    }
    index += 1;
    precededByWhitespace = isSpaceOrBreak(ch);
    followedByWhitespace = index + 1 >= n || isSpaceOrBreak(chars[index + 1]);
  }
  let allowFlowPlain = true;
  let allowBlockPlain = true;
  let allowSingleQuoted = true;
  let allowBlock = true;
  if (leadingSpace || leadingBreak || trailingSpace || trailingBreak) {
    allowFlowPlain = false;
    allowBlockPlain = false;
  }
  if (trailingSpace) allowBlock = false;
  if (breakSpace) {
    allowFlowPlain = false;
    allowBlockPlain = false;
    allowSingleQuoted = false;
  }
  if (spaceBreak || specialCharacters) {
    allowFlowPlain = false;
    allowBlockPlain = false;
    allowSingleQuoted = false;
    allowBlock = false;
  }
  if (lineBreaks) {
    allowFlowPlain = false;
    allowBlockPlain = false;
  }
  if (flowIndicators) allowFlowPlain = false;
  if (blockIndicators) allowBlockPlain = false;
  return { chars, empty: false, multiline: lineBreaks, allowFlowPlain, allowBlockPlain, allowSingleQuoted, allowBlock };
}

// ---------- safe write ----------

function assertNoSymlinks(file) {
  const absolute = path.resolve(file);
  const hub = parentsUp(absolute, 5);
  const chain = [absolute];
  if (isHub(hub)) {
    for (let p = path.dirname(absolute); p !== hub && p !== path.dirname(p); p = path.dirname(p)) chain.push(p);
  } else chain.push(path.dirname(absolute));
  for (const p of chain) {
    let stat;
    try {
      stat = fs.lstatSync(p);
    } catch {
      continue;
    }
    if (stat.isSymbolicLink()) {
      fatal(`${p} is a symbolic link; nothing was written`,
        'distill.yaml is rewritten in place, and a link could redirect the write to a file outside the skill',
        'replace the link with the real file or folder and rerun');
    }
  }
}

function writeAtomic(file, text) {
  assertNoSymlinks(file);
  const absolute = path.resolve(file);
  const tmp = path.join(path.dirname(absolute), `.${path.basename(absolute)}.${crypto.randomBytes(8).toString('hex')}.tmp`);
  let fd = null;
  let created = false;
  try {
    fd = fs.openSync(tmp, 'wx', 0o644);
    created = true;
    const bytes = Buffer.from(text, 'utf8');
    let offset = 0;
    while (offset < bytes.length) offset += fs.writeSync(fd, bytes, offset, bytes.length - offset);
    fs.fsyncSync(fd);
    fs.closeSync(fd);
    fd = null;
    fs.renameSync(tmp, absolute);
  } catch (error) {
    if (fd !== null) {
      try { fs.closeSync(fd); } catch { /* already failing */ }
    }
    if (created) {
      try { fs.unlinkSync(tmp); } catch { /* already failing */ }
    }
    fatal(`could not write ${displayPath(file)}`, error.message, 'check the folder permissions and free space, then rerun');
  }
}

function write(file, data) {
  const text = canonical(data);
  let reparsed;
  try {
    reparsed = parseYaml(text);
  } catch (error) {
    fail([`canonical output failed validation (bug in ${SCRIPT})`, `not valid YAML: ${error.message}`]);
  }
  const errors = [];
  validate(reparsed, errors);
  if (errors.length) fail([`canonical output failed validation (bug in ${SCRIPT})`, ...errors]);
  writeAtomic(file, text);
}

// ---------- commands ----------

function dropDerived(data) {
  if (!(data instanceof PyDict)) return;
  const lessons = data.get('lessons');
  if (!Array.isArray(lessons)) return;
  for (const lesson of lessons) {
    if (lesson instanceof PyDict) {
      lesson.delete('final_score');
      if (lesson.get('score') instanceof PyDict) lesson.get('score').delete('impact');
    }
  }
}

function runChecks(file, data, withWhere) {
  const errors = [];
  validate(data, errors);
  if (!errors.length && withWhere) checkWhere(data, requireHub(file), errors);
  return errors;
}

function cmdCheck(args, out) {
  const data = load(args.file);
  assertHubOption(args.file, args.hub);
  const errors = runChecks(args.file, data, !args.noWhere);
  if (errors.length) fail(errors);
  const lessons = data.get('lessons');
  const sortedKeys = sortLessons(lessons).map((l) => l.get('key'));
  const stale = lessons.filter((l) => !pyEq(l.get('final_score', null), finalScore(l))
    || !pyEq(l.get('score').get('impact', null), impactOf(l.get('score'))));
  const currentKeys = lessons.map((l) => l.get('key'));
  if (!pyEq(currentKeys, sortedKeys) || stale.length) out(`WARN: order or final_score is out of date; run \`${SCRIPT} format\``);
  out(`OK: ${lessons.length} lessons, goal ${pyStr(data.get('goal').get('status'))}`);
}

function cmdFormat(args, out) {
  const data = load(args.file);
  assertHubOption(args.file, args.hub);
  dropDerived(data);
  const errors = runChecks(args.file, data, !args.noWhere);
  if (errors.length) fail(errors);
  write(args.file, data);
  out(`OK: wrote ${data.get('lessons').length} lessons in priority order`);
}

function padCodePoints(text, width, left) {
  const fill = ' '.repeat(Math.max(0, width - [...text].length));
  return left ? fill + text : text + fill;
}

function cmdList(args, out) {
  const data = load(args.file);
  if (!(data instanceof PyDict)) throw new Error('top level must be a mapping');
  const lessons = sortLessons(data.get('lessons'));
  const shown = args.all ? lessons : lessons.slice(0, args.top);
  shown.forEach((lesson, i) => {
    const decision = lesson.get('decision');
    if (!(decision instanceof PyDict) || !decision.has('state')) throw new Error(`lesson ${pyStr(lesson.get('key'))}: decision.state missing`);
    out(`${padCodePoints(String(i + 1), 3, true)}  ${padCodePoints(floatRepr(finalScore(lesson).value), 5, true)}  `
      + `${padCodePoints(pyStr(decision.get('state')), 9, false)}  ${padCodePoints(pyStr(lesson.get('layer')), 11, false)}  `
      + `${padCodePoints(pyStr(lesson.get('contrast')), 15, false)}  ${pyStr(lesson.get('key'))}`);
  });
}

function cmdDecide(args, out) {
  const data = load(args.file);
  dropDerived(data);
  const errors = runChecks(args.file, data, false);
  if (errors.length) fail(errors);
  const lessons = sortLessons(data.get('lessons'));
  const byKey = new Map(lessons.map((l) => [l.get('key'), l]));
  const chosen = [];
  for (const selector of args.selectors) {
    if (/^\p{Nd}+$/u.test(selector)) {
      const number = Number(pyIntOf(selector));
      if (!(number >= 1 && number <= lessons.length)) fail([`no lesson number ${number}; there are ${lessons.length}`]);
      chosen.push(lessons[number - 1]);
    } else if (byKey.has(selector)) chosen.push(byKey.get(selector));
    else fail([`no lesson with key ${strRepr(selector)}`]);
  }
  if (args.state === 'rejected' && !args.reason) fail(['--reason is required for rejected']);
  for (const lesson of chosen) {
    const source = matchWhere(lesson.get('where')[0])[1];
    const decision = new PyDict([['state', args.state]]);
    if (args.reason) decision.set('reason', args.reason);
    if (args.state !== 'candidate') decision.set('at', [...data.get('cursors').get(source)].slice(0, 12).join(''));
    lesson.set('decision', decision);
  }
  write(args.file, data);
  for (const lesson of chosen) out(`${lesson.get('key')}: ${args.state}`);
}

// ---------- command line ----------

const MAIN_USAGE = `usage: ${SCRIPT} [-h] {check,format,list,decide} ...`;

const MAIN_HELP = `${MAIN_USAGE}

Validate, score, sort and update a skill's .meta/distill.yaml.

Commands:
  check  FILE [--no-where] [--hub DIR]
                                    validate schema and evidence; exit 1 on any error
  format FILE [--no-where] [--hub DIR]
                                    validate, compute final_score, sort, rewrite
  list   FILE [--top N] [--all]     numbered lessons in priority order
  decide FILE SELECTOR... --state STATE [--reason TEXT]
                                    record a human decision (SELECTOR = 1-based number or key)

The file is always rewritten in one canonical layout, so hand edits and agent
edits converge. Evidence (\`where\`) is checked against the source mirrors at
<hub>/runtime/distill-lab/mirrors/<source-id>, where <hub> is found from the
file's own path (<hub>/skills/<collection>/<skill>/.meta/distill.yaml);
--hub DIR only asserts that DIR is that hub. The evidence check runs local git
(2.45.0 or newer) and never fetches; list, decide and --no-where need neither
git nor a hub.

positional arguments:
  {check,format,list,decide}

options:
  -h, --help            show this help message and exit
`;

const DECIDE_USAGE = `usage: ${SCRIPT} decide [-h] --state {candidate,planned,ported,rejected}
${' '.repeat(`usage: ${SCRIPT} decide `.length)}[--reason REASON]
${' '.repeat(`usage: ${SCRIPT} decide `.length)}file selectors [selectors ...]`;

const COMMANDS = {
  check: {
    usage: `usage: ${SCRIPT} check [-h] [--no-where] [--hub HUB] file`,
    positionals: [{ name: 'file', many: false }],
    options: [
      { flag: '--no-where', dest: 'noWhere', takesValue: false, help: 'skip evidence checks against mirrors' },
      { flag: '--hub', dest: 'hub', takesValue: true, metavar: 'HUB', help: 'assert that HUB is the hub that holds the file' },
    ],
    run: cmdCheck,
  },
  format: {
    usage: `usage: ${SCRIPT} format [-h] [--no-where] [--hub HUB] file`,
    positionals: [{ name: 'file', many: false }],
    options: [
      { flag: '--no-where', dest: 'noWhere', takesValue: false, help: 'skip evidence checks against mirrors' },
      { flag: '--hub', dest: 'hub', takesValue: true, metavar: 'HUB', help: 'assert that HUB is the hub that holds the file' },
    ],
    run: cmdFormat,
  },
  list: {
    usage: `usage: ${SCRIPT} list [-h] [--top TOP] [--all] file`,
    positionals: [{ name: 'file', many: false }],
    options: [
      { flag: '--top', dest: 'top', takesValue: true, metavar: 'TOP', type: 'int', default: 15 },
      { flag: '--all', dest: 'all', takesValue: false },
    ],
    run: cmdList,
  },
  decide: {
    usage: DECIDE_USAGE,
    positionals: [{ name: 'file', many: false }, { name: 'selectors', many: true }],
    options: [
      { flag: '--state', dest: 'state', takesValue: true, metavar: '{candidate,planned,ported,rejected}', choices: STATES, required: true },
      { flag: '--reason', dest: 'reason', takesValue: true, metavar: 'REASON' },
    ],
    run: cmdDecide,
  },
};

function commandHelp(spec) {
  const rows = [['-h, --help', 'show this help message and exit'],
    ...spec.options.map((o) => [o.takesValue ? `${o.flag} ${o.metavar}` : o.flag, o.help || ''])];
  const width = Math.max(...rows.map(([left]) => left.length));
  const column = width + 4 > 24 ? 24 : width + 4;
  const lines = rows.map(([left, help]) => {
    if (!help) return `  ${left}`;
    if (left.length + 4 > column) return `  ${left}\n${' '.repeat(column)}${help}`;
    return `  ${left.padEnd(column - 2)}${help}`;
  });
  return `${spec.usage}\n\npositional arguments:\n${spec.positionals.map((p) => `  ${p.name}`).join('\n')}\n\noptions:\n${lines.join('\n')}\n`;
}

function usageError(usage, prefix, message) {
  throw new ExitError(2, `${usage}\n${prefix}: error: ${message}\n`);
}

const NEGATIVE_NUMBER_RE = /^-\d+$|^-\d*\.\d+$/;

function matchOption(spec, text, usage, prefix) {
  const all = [{ flag: '--help', dest: 'help', takesValue: false }, ...spec.options];
  const exact = all.find((o) => o.flag === text);
  if (exact) return exact;
  const candidates = all.filter((o) => o.flag.startsWith(text));
  if (candidates.length === 1) return candidates[0];
  if (candidates.length > 1) usageError(usage, prefix, `ambiguous option: ${text} could match ${candidates.map((o) => o.flag).join(', ')}`);
  return null;
}

export function parseArguments(argv) {
  let index = 0;
  let command = null;
  const extras = [];
  // Options before the command.
  while (index < argv.length) {
    const arg = argv[index];
    if (arg === '-h' || (arg.startsWith('--') && arg.length > 2 && '--help'.startsWith(arg))) {
      throw new ExitError(0, null, MAIN_HELP);
    }
    if (arg.startsWith('-') && arg !== '-' && arg !== '--') {
      extras.push(arg);
      index += 1;
      continue;
    }
    if (arg === '--') index += 1;
    break;
  }
  if (index >= argv.length) usageError(MAIN_USAGE, SCRIPT, 'the following arguments are required: command');
  command = argv[index];
  index += 1;
  const spec = COMMANDS[command];
  if (!spec) usageError(MAIN_USAGE, SCRIPT, `argument command: invalid choice: ${strRepr(command)} (choose from check, format, list, decide)`);
  const prefix = `${SCRIPT} ${command}`;
  const args = { command };
  for (const o of spec.options) args[o.dest] = o.default !== undefined ? o.default : (o.takesValue ? null : false);
  const seen = new Set();
  const positionals = spec.positionals.map((p) => ({ ...p, values: null }));
  let chunk = [];
  const flushChunk = () => {
    let i = 0;
    for (const p of positionals) {
      if (p.values !== null) continue;
      if (i >= chunk.length) break;
      if (p.many) {
        p.values = chunk.slice(i);
        i = chunk.length;
      } else {
        p.values = [chunk[i]];
        i += 1;
      }
    }
    extras.push(...chunk.slice(i));
    chunk = [];
  };
  let onlyPositionals = false;
  while (index < argv.length) {
    const arg = argv[index];
    index += 1;
    if (!onlyPositionals && arg === '--') {
      onlyPositionals = true;
      continue;
    }
    const looksLikeOption = !onlyPositionals && arg.startsWith('-') && arg !== '-' && !NEGATIVE_NUMBER_RE.test(arg);
    if (!looksLikeOption) {
      chunk.push(arg);
      continue;
    }
    flushChunk();
    if (arg === '-h') throw new ExitError(0, null, commandHelp(spec));
    const [name, inline] = arg.startsWith('--') && arg.includes('=') ? [arg.slice(0, arg.indexOf('=')), arg.slice(arg.indexOf('=') + 1)] : [arg, null];
    const option = arg.startsWith('--') ? matchOption(spec, name, spec.usage, prefix) : null;
    if (!option) {
      extras.push(arg);
      continue;
    }
    if (option.dest === 'help') throw new ExitError(0, null, commandHelp(spec));
    if (!option.takesValue) {
      if (inline !== null) usageError(spec.usage, prefix, `argument ${option.flag}: ignored explicit argument ${strRepr(inline)}`);
      args[option.dest] = true;
      seen.add(option.flag);
      continue;
    }
    let value = inline;
    if (value === null) {
      const next = argv[index];
      if (next === undefined || (next.startsWith('-') && next !== '-' && !NEGATIVE_NUMBER_RE.test(next))) {
        usageError(spec.usage, prefix, `argument ${option.flag}: expected one argument`);
      }
      value = next;
      index += 1;
    }
    if (option.type === 'int') {
      const parsed = parseIntArgument(value);
      if (parsed === null) usageError(spec.usage, prefix, `argument ${option.flag}: invalid int value: ${strRepr(value)}`);
      value = parsed;
    }
    if (option.choices && !option.choices.includes(value)) {
      usageError(spec.usage, prefix, `argument ${option.flag}: invalid choice: ${strRepr(value)} (choose from ${option.choices.join(', ')})`);
    }
    args[option.dest] = value;
    seen.add(option.flag);
  }
  flushChunk();
  const missing = [
    ...positionals.filter((p) => p.values === null).map((p) => p.name),
    ...spec.options.filter((o) => o.required && !seen.has(o.flag)).map((o) => o.flag),
  ];
  if (missing.length) usageError(spec.usage, prefix, `the following arguments are required: ${missing.join(', ')}`);
  if (extras.length) usageError(MAIN_USAGE, SCRIPT, `unrecognized arguments: ${extras.join(' ')}`);
  for (const p of positionals) args[p.name] = p.many ? p.values : p.values[0];
  return { args, spec };
}

// Runs one command line and returns { code, stdout, stderr }; it never throws.
export function main(argv) {
  let stdout = '';
  const out = (line) => {
    stdout += `${line}\n`;
  };
  try {
    const { args, spec } = parseArguments(argv);
    spec.run(args, out);
    return { code: 0, stdout, stderr: '' };
  } catch (error) {
    if (error instanceof ExitError) return { code: error.code, stdout: stdout + error.stdout, stderr: error.text };
    const message = error && error.message ? error.message : String(error);
    return {
      code: 1,
      stdout,
      stderr: `ERROR: ${message}\nWHY: ${SCRIPT} stopped on input it could not process\n`
        + `FIX: run \`${SCRIPT} check <file> --no-where\` to find the problem in the file, then rerun\n`,
    };
  }
}

function runFromCommandLine() {
  const result = main(process.argv.slice(2));
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  process.exitCode = result.code;
}

function isEntryModule() {
  if (!process.argv[1]) return false;
  try {
    return fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));
  } catch {
    return false;
  }
}

if (isEntryModule()) runFromCommandLine();

export { PyDict, PyFloat, PyDate, PyDateTime, pyRound, floatRepr };
