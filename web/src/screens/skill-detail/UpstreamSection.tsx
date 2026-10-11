import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { checkSkillUpstream, confirmSourceProposal, previewTrackUpstream } from '../../api/queries';
import type { SkillUpstream, UpstreamTrackProposal } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { ProposalPreview } from '../../components/ProposalPreview';
import { StatusBadge } from '../../components/StatusBadge';
import { proposalPins, sourceErrorText } from './AddSourceForm';
import { isUpstreamTracked, upstreamStatusBadge } from './detail-format';

const TITLE_UPSTREAM = 'Upstream repository';
const BTN_CHECK_NOW = 'Check now';
const BTN_CHECKING = 'Checking…';
const BTN_REVIEW_UPDATE = 'Review update';
const BTN_TRACK = 'Track upstream';
const BTN_TRACK_PREVIEWING = 'Preparing preview…';
const LABEL_UNTRACKED_HINT =
  'Skill Hub knows where this skill came from, but no source record tracks that repository yet, so it cannot check for or review updates. Track the upstream to link it; you see the exact change first.';
const LABEL_FALLBACK = 'Or run this in a terminal:';
const BANNER_UPSTREAM_REMOVED =
  'Skill removed in upstream repository. Your local copy continues working as a custom skill and no upstream updates can be applied.';
const LABEL_REPOSITORY = 'Repository';
const LABEL_PATH = 'Path';
const LABEL_CURRENT_COMMIT = 'Current commit';
const LABEL_LATEST_COMMIT = 'Latest commit';
const LABEL_FILES_CHANGED = 'Files changed';
const LABEL_LAST_CHECKED = 'Last checked';
const LABEL_CHANGED_UPSTREAM_PREFIX = 'Changed upstream (';
const LABEL_CHANGED_UPSTREAM_SUFFIX = '):';

interface UpstreamSectionProps {
  skillId: string;
  upstream: SkillUpstream | null;
  onReviewUpdate: () => void;
  onTracked: (message: string) => void;
}

export function UpstreamSection({ skillId, upstream, onReviewUpdate, onTracked }: UpstreamSectionProps) {
  const queryClient = useQueryClient();
  const [checking, setChecking] = useState(false);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [trackPreviewing, setTrackPreviewing] = useState(false);
  const [trackError, setTrackError] = useState<string | null>(null);
  const [trackInfo, setTrackInfo] = useState<string | null>(null);
  const [trackProposal, setTrackProposal] = useState<UpstreamTrackProposal | null>(null);
  const [trackConfirming, setTrackConfirming] = useState(false);
  const [trackConfirmError, setTrackConfirmError] = useState<string | null>(null);
  const [trackAttempt, setTrackAttempt] = useState(0);

  if (!upstream) {
    return null;
  }

  const handleCheckNow = async () => {
    setChecking(true);
    setCheckError(null);
    try {
      await checkSkillUpstream(skillId);
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
    } catch (err) {
      setCheckError(err instanceof Error ? err.message : 'Failed to check upstream');
    } finally {
      setChecking(false);
    }
  };

  const handleTrackPreview = async () => {
    setTrackPreviewing(true);
    setTrackError(null);
    setTrackInfo(null);
    try {
      const res = await previewTrackUpstream(skillId);
      if (proposalPins(res)) {
        setTrackConfirmError(null);
        setTrackProposal(res);
      } else {
        setTrackInfo(res.summary);
      }
    } catch (err) {
      setTrackError(sourceErrorText(err, 'Failed to preview tracking the upstream'));
    } finally {
      setTrackPreviewing(false);
    }
  };

  const handleTrackConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    setTrackConfirming(true);
    setTrackConfirmError(null);
    try {
      const res = await confirmSourceProposal(pins.proposalId, {
        proposal_digest: pins.proposalDigest,
        base_version: pins.baseVersion,
      });
      for (const queryKey of [['skill-sources', skillId], ['skill-review', skillId], ['skill', skillId], ['skills'], ['sources']]) {
        void queryClient.invalidateQueries({ queryKey });
      }
      setTrackProposal(null);
      onTracked(res.summary || `Tracking the upstream of ${skillId}.`);
    } catch (err) {
      setTrackConfirmError(sourceErrorText(err, 'Failed to track the upstream'));
      setTrackAttempt((n) => n + 1);
    } finally {
      setTrackConfirming(false);
    }
  };

  const statusProps = upstreamStatusBadge(upstream.status);
  const tracked = isUpstreamTracked(upstream);
  const canReview =
    tracked && (upstream.status === 'update_available' || upstream.status === 'diverged');
  const trackPins = trackProposal ? proposalPins(trackProposal) : null;
  const shortBase = upstream.base_commit ? upstream.base_commit.slice(0, 12) : '-';
  const shortLatest = upstream.latest_commit ? upstream.latest_commit.slice(0, 12) : '-';
  const commitDate = upstream.latest_committed_at ? upstream.latest_committed_at.slice(0, 10) : '-';
  const repoRefText = `${upstream.repository}@${upstream.ref || 'main'}`;
  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <span className="fg-card__title">{TITLE_UPSTREAM}</span>
          <StatusBadge tone={statusProps.tone} label={statusProps.label} />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary fg-btn--small"
            onClick={handleCheckNow}
            disabled={checking || !tracked}
          >
            <span>{checking ? BTN_CHECKING : BTN_CHECK_NOW}</span>
          </button>
          <button
            type="button"
            className="fg-btn fg-btn--primary fg-btn--small"
            onClick={onReviewUpdate}
            disabled={!canReview}
          >
            <span>{BTN_REVIEW_UPDATE}</span>
          </button>
        </div>
      </div>

      {checkError && (
        <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
          <span>{checkError}</span>
        </div>
      )}

      {!tracked && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <p style={{ margin: 0, fontSize: '13px', color: 'var(--color-text-muted)', maxWidth: '72ch' }}>
            <span>{LABEL_UNTRACKED_HINT}</span>
          </p>
          <button
            type="button"
            className="fg-btn fg-btn--primary"
            style={{ width: 'fit-content' }}
            onClick={() => void handleTrackPreview()}
            disabled={trackPreviewing}
          >
            <span>{trackPreviewing ? BTN_TRACK_PREVIEWING : BTN_TRACK}</span>
          </button>
          {trackError && (
            <div className="fg-banner fg-banner--danger" role="alert" style={{ fontSize: '13px' }}>
              <span>{trackError}</span>
            </div>
          )}
          {trackInfo && (
            <div className="fg-banner fg-banner--info" role="status" style={{ fontSize: '13px' }}>
              <span>{trackInfo}</span>
            </div>
          )}
          <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>{LABEL_FALLBACK}</span>
          <CommandBlock command={`skillhub source backfill --skill ${skillId}`} />
        </div>
      )}

      {upstream.status === 'upstream_removed' && (
        <div className="fg-banner fg-banner--warning" style={{ fontSize: '13px' }}>
          <span>{BANNER_UPSTREAM_REMOVED}</span>
        </div>
      )}

      <div className="fg-facts">
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_REPOSITORY}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{repoRefText}</span>
          </div>
        </div>
        {upstream.path && (
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_PATH}</span>
            </div>
            <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{upstream.path}</span>
            </div>
          </div>
        )}
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_CURRENT_COMMIT}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortBase}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_LATEST_COMMIT}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortLatest} {commitDate !== '-' ? `(${commitDate})` : ''}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_FILES_CHANGED}</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.changed_files >= 0 ? `${upstream.changed_files} file(s)` : '-'}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_LAST_CHECKED}</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.checked_at || 'never'}</span>
          </div>
        </div>
      </div>

      {upstream.files && upstream.files.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div style={{ fontSize: '13px', fontWeight: 600 }}>
            <span>{LABEL_CHANGED_UPSTREAM_PREFIX}{upstream.files.length}{LABEL_CHANGED_UPSTREAM_SUFFIX}</span>
          </div>
          <ul style={{ margin: 0, paddingLeft: 'var(--space-4)', fontSize: '13px', fontFamily: 'var(--font-mono)' }}>
            {upstream.files.map((file) => {
              const statusBadgeText = `[${file.status}]`;
              return (
                <li key={file.path}>
                  <span style={{ color: 'var(--color-text-muted)', marginRight: 'var(--space-2)' }}>
                    <span>{statusBadgeText}</span>
                  </span>
                  <span>{file.path}</span>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {upstream.next_action && (
        <div style={{ fontSize: '13px', color: 'var(--color-text-muted)', marginTop: 'var(--space-1)' }}>
          <span>{upstream.next_action}</span>
        </div>
      )}

      {trackProposal && trackPins && (
        <ProposalPreview
          key={trackAttempt}
          open
          title={`Track upstream for ${skillId}?`}
          target={upstream.repository || skillId}
          impact={trackImpact(trackProposal)}
          paths={[...(trackProposal.diff?.added ?? []), ...(trackProposal.diff?.modified ?? [])]}
          warning={trackProposal.warnings?.[0]?.summary}
          diff={trackProposal.full_diff ?? ''}
          proposalId={trackPins.proposal_id}
          proposalDigest={trackPins.proposal_digest}
          baseVersion={trackPins.base_version}
          confirmLabel={BTN_TRACK}
          isLoading={trackConfirming}
          networkError={trackConfirmError}
          onConfirm={(p) => void handleTrackConfirm(p)}
          onCancel={() => setTrackProposal(null)}
        />
      )}
    </section>
  );
}

function trackImpact(p: UpstreamTrackProposal): string {
  const c = p.candidate;
  if (!c) return p.summary;
  return c.create_source
    ? `Creates a new source record ${c.source_id} and links this skill's upstream to it.`
    : `Links this skill's upstream to the existing source record ${c.source_id}; no new source record is created.`;
}
