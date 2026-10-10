import { describe, expect, it } from 'vitest';
import type { SkillDetail } from '../../api/types';
import { activationChecklist } from './activation-checklist';

function skill(overrides: Partial<SkillDetail> = {}): SkillDetail {
  return {
    skill_id: 'demo',
    name: 'Demo',
    description: '',
    status: 'draft',
    path: '',
    catalog_snapshot: '',
    content: '# Demo\n\nSteps.',
    content_digest: '',
    state_basis: '',
    lifecycle_state: 'draft',
    routing_eligible: false,
    diverged: false,
    routing: {},
    resources: [],
    ...overrides,
  } as SkillDetail;
}

describe('activationChecklist', () => {
  it('lists what a bare draft is missing', () => {
    const missing = activationChecklist(skill()).filter((i) => !i.valid).map((i) => i.label);
    expect(missing).toEqual(['Triggers', 'Operations & Rationale', 'Min scope']);
  });

  it('is complete when routing fields are filled in', () => {
    const ready = skill({ routing: { operations: ['review'], triggers: ['when asked'], not_for: ['unrelated work'], min_scope: 'workspace' } as SkillDetail['routing'] });
    expect(activationChecklist(ready).every((i) => i.valid)).toBe(true);
  });

  it('treats untouched scaffold instructions as missing content', () => {
    const items = activationChecklist(skill(), { activation_readiness: { ready: false, untouched_scaffold: true } } as never);
    expect(items.find((i) => i.id === 'content')?.valid).toBe(false);
  });
});
