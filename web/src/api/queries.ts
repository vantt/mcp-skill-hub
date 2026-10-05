import { useQuery } from '@tanstack/react-query';
import { apiFetch } from './client';
import type {
  ConfirmationPins,
  CurationHome,
  FunnelReport,
  FunnelSince,
  SessionResponse,
  SkillAddProposal,
  SkillAddResult,
  SkillDetail,
  SkillListResult,
  SkillMutationResult,
  SkillProposal,
  SkillReviewResult,
  SkillRuntimeStatus,
  SkillSourcesResult,
  SkillUpstream,
  SourceCheckResult,
  SourceImportProposal,
  SourceListResult,
  SourceMutationResult,
  SourceProposal,
  UpstreamUpdatePreview,
  UpstreamUpdateResult,
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

export function useSkillUsage(id: string, since: FunnelSince = '30d') {
  return useQuery({
    queryKey: ['skill-usage', id, since],
    queryFn: () =>
      apiFetch<FunnelReport>(`/skills/${encodeURIComponent(id)}/usage?since=${encodeURIComponent(since)}`),
    enabled: Boolean(id),
  });
}

export function useSkillRuntime(id: string) {
  return useQuery({
    queryKey: ['skill-runtime', id],
    queryFn: () => apiFetch<SkillRuntimeStatus>(`/skills/${encodeURIComponent(id)}/runtime`),
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

export function useSkillSources(id: string) {
  return useQuery({
    queryKey: ['skill-sources', id],
    queryFn: () => apiFetch<SkillSourcesResult>(`/skills/${encodeURIComponent(id)}/sources`),
    enabled: Boolean(id),
  });
}

export function useSources() {
  return useQuery({
    queryKey: ['sources'],
    queryFn: () => apiFetch<SourceListResult>('/sources'),
  });
}

export async function checkSkillUpstream(id: string): Promise<SkillUpstream> {
  return apiFetch<SkillUpstream>(`/skills/${encodeURIComponent(id)}/upstream/check`, {
    method: 'POST',
  });
}

export async function reviewSkillUpdate(
  id: string,
  body?: {
    target_commit?: string;
    resolutions?: Array<{ path: string; action: string; content?: string }>;
    idempotency_key?: string;
  },
): Promise<UpstreamUpdatePreview> {
  return apiFetch<UpstreamUpdatePreview>(`/skills/${encodeURIComponent(id)}/upstream/review`, {
    method: 'POST',
    body: JSON.stringify(body || {}),
  });
}

export async function confirmUpstreamUpdate(
  proposalId: string,
  pins: { proposal_digest: string; base_version: string },
): Promise<UpstreamUpdateResult> {
  return apiFetch<UpstreamUpdateResult>(
    `/upstream/proposals/${encodeURIComponent(proposalId)}/confirm`,
    {
      method: 'POST',
      body: JSON.stringify(pins),
    },
  );
}

export async function previewAttachSource(
  id: string,
  body: {
    source_id?: string;
    locator?: string;
    ref?: string;
    path?: string;
    cadence?: string;
  },
): Promise<SourceProposal> {
  return apiFetch<SourceProposal>(`/skills/${encodeURIComponent(id)}/sources/attach/preview`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function previewDetachSource(id: string, sourceId: string): Promise<SourceProposal> {
  return apiFetch<SourceProposal>(
    `/skills/${encodeURIComponent(id)}/sources/${encodeURIComponent(sourceId)}/detach/preview`,
    {
      method: 'POST',
    },
  );
}

export async function previewUnwatchSource(sourceId: string): Promise<SourceProposal> {
  return apiFetch<SourceProposal>(`/sources/${encodeURIComponent(sourceId)}/unwatch/preview`, {
    method: 'POST',
  });
}

export async function confirmSourceProposal(
  proposalId: string,
  pins: { proposal_digest: string; base_version: string },
): Promise<SourceMutationResult> {
  return apiFetch<SourceMutationResult>(
    `/sources/proposals/${encodeURIComponent(proposalId)}/confirm`,
    {
      method: 'POST',
      body: JSON.stringify(pins),
    },
  );
}

export async function checkSources(body: {
  source_ids?: string[];
  all?: boolean;
  due?: boolean;
}): Promise<SourceCheckResult> {
  return apiFetch<SourceCheckResult>('/sources/check', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function previewSourceImport(
  sourceId: string,
  body?: { skills?: string[]; path?: string },
): Promise<SourceImportProposal> {
  return apiFetch<SourceImportProposal>(`/sources/${encodeURIComponent(sourceId)}/import/preview`, {
    method: 'POST',
    body: JSON.stringify(body || {}),
  });
}

export async function confirmSourceImport(pins: ConfirmationPins): Promise<SourceMutationResult> {
  return apiFetch<SourceMutationResult>('/sources/import/confirm', {
    method: 'POST',
    body: JSON.stringify(pins),
  });
}
