import type { SkillDetail, SkillReviewResult } from '../../api/types';

export interface ChecklistItem {
  id: 'content' | 'triggers' | 'not_for' | 'scope';
  label: string;
  valid: boolean;
}

// What a draft needs before it can be activated; shared by the header and the Review tab so they agree.
export function activationChecklist(skill: SkillDetail, review?: SkillReviewResult): ChecklistItem[] {
  const isUntouched = review?.activation_readiness?.untouched_scaffold ?? false;
  const hasContent = !isUntouched && Boolean(skill.content && skill.content.trim().length > 0);
  const hasTriggers = (skill.routing?.triggers ?? []).length > 0;
  // Operations is optional. A skill needs "not for" or a routing review rationale; the rationale is only
  // visible through the review, so trust the review when it does not list this field as missing.
  const notForMissingInReview = review?.activation_readiness?.missing_fields?.some((f) => f.startsWith('not_for'));
  const hasNotFor =
    (skill.routing?.not_for ?? []).length > 0 || (review !== undefined && notForMissingInReview === false);
  const hasScope = Boolean(skill.routing?.min_scope);
  return [
    { id: 'content', label: 'Instructions (SKILL.md)', valid: hasContent },
    { id: 'triggers', label: 'Triggers', valid: hasTriggers },
    { id: 'not_for', label: 'Not for / Rationale', valid: hasNotFor },
    { id: 'scope', label: 'Min scope', valid: hasScope },
  ];
}
