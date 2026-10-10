import type { SkillDetail, SkillReviewResult } from '../../api/types';

export interface ChecklistItem {
  id: 'content' | 'triggers' | 'operations' | 'scope';
  label: string;
  valid: boolean;
}

// What a draft needs before it can be activated; shared by the header and the Review tab so they agree.
export function activationChecklist(skill: SkillDetail, review?: SkillReviewResult): ChecklistItem[] {
  const isUntouched = review?.activation_readiness?.untouched_scaffold ?? false;
  const hasContent = !isUntouched && Boolean(skill.content && skill.content.trim().length > 0);
  const hasTriggers = (skill.routing?.triggers ?? []).length > 0;
  const hasOperations = (skill.routing?.operations ?? []).length > 0 || Boolean(skill.routing?.not_for);
  const hasScope = Boolean(skill.routing?.min_scope);
  return [
    { id: 'content', label: 'Instructions (SKILL.md)', valid: hasContent },
    { id: 'triggers', label: 'Triggers', valid: hasTriggers },
    { id: 'operations', label: 'Operations & Rationale', valid: hasOperations },
    { id: 'scope', label: 'Min scope', valid: hasScope },
  ];
}
