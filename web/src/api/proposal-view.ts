import type { SkillProposal } from './types';

export interface ProposalFiles {
  added: string[];
  modified: string[];
  deleted: string[];
}

// The API sends `diff` as {added, modified, deleted} path lists and the patch as `full_diff`.
export function proposalFiles(p: Pick<SkillProposal, 'diff'>): ProposalFiles {
  const d = p.diff;
  if (!d || typeof d !== 'object' || Array.isArray(d)) {
    return { added: [], modified: [], deleted: [] };
  }
  return { added: d.added ?? [], modified: d.modified ?? [], deleted: d.deleted ?? [] };
}

// Paths in words: "SKILL.md" rather than the full canonical path, with what happens to each.
export function proposalPaths(p: Pick<SkillProposal, 'diff'>): string[] {
  const f = proposalFiles(p);
  const line = (verb: string, path: string) => `${verb}: ${path}`;
  return [
    ...f.added.map((x) => line('new', x)),
    ...f.modified.map((x) => line('changed', x)),
    ...f.deleted.map((x) => line('removed', x)),
  ];
}

export function proposalPatch(p: Pick<SkillProposal, 'diff' | 'full_diff'>): string {
  if (typeof p.full_diff === 'string') return p.full_diff;
  return typeof p.diff === 'string' ? p.diff : '';
}

export function proposalWarning(p: Pick<SkillProposal, 'routing_impact'>): string | undefined {
  const warnings = p.routing_impact?.warnings ?? [];
  return warnings.length > 0 ? warnings.join(' ') : undefined;
}
