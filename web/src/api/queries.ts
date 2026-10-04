import { useQuery } from '@tanstack/react-query';
import { apiFetch } from './client';
import type {
  CurationHome,
  SessionResponse,
  SkillDetail,
  SkillListResult,
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
