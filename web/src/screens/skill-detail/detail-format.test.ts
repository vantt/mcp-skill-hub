import { describe, expect, it } from 'vitest';
import type { SkillUpstream } from '../../api/types';
import { isUpstreamTracked, resourceSummaryText, upstreamStatusBadge } from './detail-format';

function upstream(overrides: Partial<SkillUpstream>): SkillUpstream {
  return {
    skill_id: 'demo',
    source_id: 'src-demo',
    repository: 'https://github.com/acme/demo',
    ref: 'main',
    path: 'skills/demo',
    base_commit: 'abc',
    latest_commit: 'abc',
    changed_files: 0,
    local: 'clean',
    status: 'up_to_date',
    checked_at: '2026-10-11T00:00:00Z',
    ...overrides,
  };
}

describe('upstreamStatusBadge', () => {
  it.each([
    ['up_to_date', 'success', 'Up to date'],
    ['update_available', 'warning', 'Update available'],
    ['modified', 'warning', 'Modified locally'],
    ['unknown', 'warning', 'Not checked'],
    ['diverged', 'danger', 'Diverged'],
    ['upstream_removed', 'danger', 'Removed upstream'],
    ['unavailable', 'danger', 'Unreachable'],
    ['pinned', 'neutral', 'Pinned'],
    ['untracked', 'neutral', 'Not tracked'],
  ])('maps %s to a %s "%s" badge', (status, tone, label) => {
    expect(upstreamStatusBadge(status)).toEqual({ tone, label });
  });

  it('shows an unrecognised status as a neutral badge with the raw value', () => {
    expect(upstreamStatusBadge('something_new')).toEqual({ tone: 'neutral', label: 'something_new' });
  });
});

describe('isUpstreamTracked', () => {
  it('is false when there is no upstream', () => {
    expect(isUpstreamTracked(null)).toBe(false);
    expect(isUpstreamTracked(undefined)).toBe(false);
  });

  it('is false for an untracked upstream', () => {
    expect(isUpstreamTracked(upstream({ status: 'untracked', source_id: '' }))).toBe(false);
    expect(isUpstreamTracked(upstream({ status: 'untracked' }))).toBe(false);
  });

  it('is false when the upstream has no source record id', () => {
    expect(isUpstreamTracked(upstream({ status: 'unknown', source_id: '' }))).toBe(false);
  });

  it('is true for an upstream linked to a source record', () => {
    expect(isUpstreamTracked(upstream({}))).toBe(true);
    expect(isUpstreamTracked(upstream({ status: 'update_available' }))).toBe(true);
  });
});

describe('resourceSummaryText', () => {
  it('uses the singular for one file', () => {
    expect(resourceSummaryText(1, 120)).toBe('1 file · 120 bytes');
  });

  it('uses the plural for zero or many files', () => {
    expect(resourceSummaryText(0, 0)).toBe('0 files · 0 bytes');
    expect(resourceSummaryText(3, 4096)).toBe('3 files · 4096 bytes');
  });
});
