import { useQuery } from '@tanstack/react-query';
import { apiFetch } from './client';
import type {
  ConfirmationPins,
  CurationHome,
  SessionResponse,
  SkillAddProposal,
  SkillAddResult,
  SkillDetail,
  SkillListResult,
  SkillMutationResult,
  SkillProposal,
  SkillReviewResult,
} from './types';

export function useSession() {
  return useQuery({
    queryKey: ['session'],
    queryFn: () => apiFetch<SessionResponse>('/session'),
  });
}

export function useHome() {
  return useQuery({
    queryKey: ['home'],
    queryFn: () => apiFetch<CurationHome>('/home'),
  });
}

export function useSkills(state?: string) {
  return useQuery({
    queryKey: state ? ['skills', state] : ['skills'],
    queryFn: () =>
      apiFetch<SkillListResult>(state ? `/skills?state=${encodeURIComponent(state)}` : '/skills'),
  });
}

export function useSkillDetail(id: string) {
  return useQuery({
    queryKey: ['skill', id],
    queryFn: () => apiFetch<SkillDetail>(`/skills/${encodeURIComponent(id)}`),
    enabled: Boolean(id),
  });
}

export function useSkillReview(id: string) {
  return useQuery({
    queryKey: ['skill-review', id],
    queryFn: () => apiFetch<SkillReviewResult>(`/skills/${encodeURIComponent(id)}/review`),
    enabled: Boolean(id),
  });
}

export async function previewSkillCreate(body: {
  id: string;
  name: string;
  description: string;
  content: string;
  collection?: string;
  routing?: {
    operations: string[];
    triggers: string[];
    not_for: string[];
    min_scope: string;
  };
  rationale?: string;
  idempotency_key?: string;
}): Promise<SkillProposal> {
  return apiFetch<SkillProposal>('/skills/create/preview', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function confirmSkillMutation(
  proposalId: string,
  pins: { proposalDigest: string; baseVersion: string },
): Promise<SkillMutationResult> {
  return apiFetch<SkillMutationResult>(
    `/skills/proposals/${encodeURIComponent(proposalId)}/confirm`,
    {
      method: 'POST',
      body: JSON.stringify({
        proposal_digest: pins.proposalDigest,
        base_version: pins.baseVersion,
      }),
    },
  );
}

export async function previewSkillUpdate(
  id: string,
  body: {
    expected_content_digest: string;
    name?: string;
    description?: string;
    content?: string;
    routing?: {
      operations: string[];
      triggers: string[];
      not_for: string[];
      min_scope: string;
    };
    rationale?: string;
    idempotency_key?: string;
  },
): Promise<SkillProposal> {
  return apiFetch<SkillProposal>(`/skills/${encodeURIComponent(id)}/update/preview`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function previewSkillTransition(
  id: string,
  target: 'active' | 'deprecated' | 'archived',
  idempotency_key?: string,
): Promise<SkillProposal> {
  return apiFetch<SkillProposal>(`/skills/${encodeURIComponent(id)}/transitions/preview`, {
    method: 'POST',
    body: JSON.stringify({ target, idempotency_key }),
  });
}

export async function previewSkillAdd(
  body: {
    locator: string;
    selection?: string;
    all?: boolean;
    target_id?: string;
    collection?: string;
    idempotency_key?: string;
  },
  signal?: AbortSignal,
): Promise<SkillAddProposal> {
  return apiFetch<SkillAddProposal>('/skills/add/preview', {
    method: 'POST',
    body: JSON.stringify(body),
    signal,
  });
}

export async function confirmSkillAdd(pins: ConfirmationPins): Promise<SkillAddResult> {
  return apiFetch<SkillAddResult>('/skills/add/confirm', {
    method: 'POST',
    body: JSON.stringify(pins),
  });
}
