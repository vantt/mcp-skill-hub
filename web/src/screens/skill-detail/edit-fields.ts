import type { SkillDetail } from '../../api/types';

// The fields the Editor lets a person change, as the strings they type.
export interface EditFields {
  name: string;
  description: string;
  content: string;
  operations: string;
  triggers: string;
  notFor: string;
  minScope: string;
}

export type EditField = keyof EditFields;

export const EDIT_FIELD_LABEL: Record<EditField, string> = {
  name: 'Name',
  description: 'Description',
  content: 'Instructions (SKILL.md)',
  operations: 'Operations',
  triggers: 'Triggers',
  notFor: 'Not for / Rationale',
  minScope: 'Min scope',
};

const FIELDS = Object.keys(EDIT_FIELD_LABEL) as EditField[];

export function fieldsFromSkill(skill: SkillDetail): EditFields {
  return {
    name: skill.name || '',
    description: skill.description || '',
    content: skill.content || '',
    operations: (skill.routing?.operations || []).join(', '),
    triggers: (skill.routing?.triggers || []).join(', '),
    notFor: (skill.routing?.not_for || []).join(', '),
    minScope: skill.routing?.min_scope || '',
  };
}

export function splitList(value: string): string[] {
  return value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

export function changedFields(base: EditFields, next: EditFields): EditField[] {
  return FIELDS.filter((f) => {
    if (f === 'operations' || f === 'triggers' || f === 'notFor') {
      return splitList(base[f]).join('\n') !== splitList(next[f]).join('\n');
    }
    return base[f] !== next[f];
  });
}

// Keeps what the person changed (against the version they opened) and takes everything else from the
// latest saved version, so an edit made elsewhere is not undone by a field they never touched.
export function mergeOntoLatest(base: EditFields, mine: EditFields, latest: EditFields): EditFields {
  const changed = new Set(changedFields(base, mine));
  const out = { ...latest };
  for (const f of FIELDS) {
    if (changed.has(f)) out[f] = mine[f];
  }
  return out;
}

function short(value: string, max = 70): string {
  const flat = value.replace(/\s+/g, ' ').trim();
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat;
}

function quoted(value: string): string {
  return value.trim() === '' ? 'empty' : `"${short(value)}"`;
}

function lineCounts(base: string, next: string): { added: number; removed: number } {
  const count = (text: string) => {
    const m = new Map<string, number>();
    for (const line of text.split('\n')) m.set(line, (m.get(line) ?? 0) + 1);
    return m;
  };
  const a = count(base);
  const b = count(next);
  let added = 0;
  let removed = 0;
  for (const [line, n] of b) added += Math.max(0, n - (a.get(line) ?? 0));
  for (const [line, n] of a) removed += Math.max(0, n - (b.get(line) ?? 0));
  return { added, removed };
}

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

// One plain sentence per changed field, so a preview says what will happen without reading a patch.
export function describeChanges(base: EditFields, next: EditFields): string[] {
  const out: string[] = [];
  for (const f of changedFields(base, next)) {
    const label = EDIT_FIELD_LABEL[f];
    if (f === 'content') {
      const { added, removed } = lineCounts(base.content, next.content);
      out.push(`${label}: ${plural(added, 'line', 'lines')} added, ${plural(removed, 'line', 'lines')} removed.`);
    } else if (f === 'operations' || f === 'triggers' || f === 'notFor') {
      const before = splitList(base[f]);
      const after = splitList(next[f]);
      const added = after.filter((x) => !before.includes(x));
      const removed = before.filter((x) => !after.includes(x));
      const parts: string[] = [];
      if (added.length > 0) parts.push(`added ${added.map((x) => `"${short(x, 40)}"`).join(', ')}`);
      if (removed.length > 0) parts.push(`removed ${removed.map((x) => `"${short(x, 40)}"`).join(', ')}`);
      out.push(`${label}: ${parts.join('; ') || 'reordered'}.`);
    } else {
      out.push(`${label}: ${quoted(base[f])} becomes ${quoted(next[f])}.`);
    }
  }
  return out;
}
