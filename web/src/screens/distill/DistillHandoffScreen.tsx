import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { useSession, useSources } from '../../api/queries';
import type { SourceSummary } from '../../api/types';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { buildHandoffBrief } from '../../domain/handoff-brief';
import { distillLabel } from '../../domain/source-distill';
import { loadDraft, saveDraft } from '../../state/drafts';

const TITLE_DISTILL = 'Distill with Curator Agent';
const LABEL_SELECTED_SOURCES = 'Selected sources';
const LABEL_EMPTY_SELECTION = 'Select learning sources on the Sources screen first.';
const BTN_BACK_SOURCES = 'Back to Sources';
const TITLE_BRIEF_BAR = 'Handoff for your Curator Agent';
const BTN_COPY_HANDOFF = 'Copy handoff';
const LABEL_COPIED = 'Copied';
const LABEL_ARROW = ' ← ';
const LABEL_LPAREN = ' (';
const LABEL_RPAREN = ')';

interface ClassifiedSource {
  id: string;
  valid: boolean;
  error?: string;
  source?: SourceSummary;
}

export function DistillHandoffScreen() {
  const [searchParams] = useSearchParams();
  const requestedSourceIds = searchParams.getAll('source');

  const { data: session } = useSession();
  const workspaceId = session?.workspace_id || 'default';

  const { data: sourcesData, isLoading, error: sourcesError } = useSources();

  const [copied, setCopied] = useState(false);
  const [randomId] = useState(() =>
    typeof crypto !== 'undefined' && crypto.randomUUID
      ? crypto.randomUUID()
      : 'hnd-' + Math.random().toString(36).substring(2, 12),
  );

  if (requestedSourceIds.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_DISTILL}</span>
        </h1>
        <div
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-6) var(--space-4)',
          }}
        >
          <span style={{ color: 'var(--color-text-muted)', fontSize: '14px', textAlign: 'center' }}>
            {LABEL_EMPTY_SELECTION}
          </span>
          <Link to="/sources" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_SOURCES}</span>
          </Link>
        </div>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="32px" />
        <Skeleton height="100px" />
        <Skeleton height="220px" />
      </div>
    );
  }

  if (sourcesError) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-banner fg-banner--danger">
          <span>{sourcesError instanceof Error ? sourcesError.message : 'Failed to load sources'}</span>
        </div>
        <Link to="/sources" className="fg-btn fg-btn--secondary" style={{ width: 'fit-content', textDecoration: 'none' }}>
          <span>{BTN_BACK_SOURCES}</span>
        </Link>
      </div>
    );
  }

  // Find all sources from groups or allSources
  const allSources: SourceSummary[] = [];
  if (sourcesData?.sources && sourcesData.sources.length > 0) {
    for (const s of sourcesData.sources) {
      const rec = s.record;
      const isUpstreamOnly = rec.purpose === 'upstream' || s.role === 'upstream';
      const isNeverDistilled = !rec.distilled_revision || !rec.distilled_revision.value;
      const isChangedOrPending = rec.status === 'changed' || rec.status === 'distill_pending';
      const isReadyToDistill = !isUpstreamOnly && (isChangedOrPending || isNeverDistilled);

      allSources.push({
        id: rec.id,
        status: rec.status,
        role: s.role,
        referencing_skills: s.skills || [],
        skills_vendored_count: 0,
        importable_count: s.importable_count,
        current_revision: rec.current_revision,
        distilled_revision: rec.distilled_revision,
        ready_to_distill: isReadyToDistill,
        upstream_only: isUpstreamOnly,
      });
    }
  } else if (sourcesData?.groups) {
    for (const g of sourcesData.groups) {
      allSources.push(...g.sources);
    }
  }

  const classified: ClassifiedSource[] = requestedSourceIds.map((id) => {
    const src = allSources.find((s) => s.id === id);
    if (!src) {
      return { id, valid: false, error: 'Source not found' };
    }
    const d = distillLabel(src);
    if (!d.selectable) {
      return { id, valid: false, error: `Not ready to distill (${d.label})`, source: src };
    }
    return { id, valid: true, source: src };
  });

  const validSources = classified.filter((c) => c.valid);
  const validIds = validSources.map((c) => c.id);

  // Generate or retrieve idempotency key
  const draftId = [...validIds].sort().join(',');
  let handoffRequestId = '';
  if (validIds.length > 0) {
    const existingDraft = loadDraft<{ handoff_request_id: string }>(
      workspaceId,
      'distill-handoff',
      draftId,
      '',
    );
    if (existingDraft?.value?.handoff_request_id) {
      handoffRequestId = existingDraft.value.handoff_request_id;
    } else {
      handoffRequestId = randomId;
      saveDraft(workspaceId, 'distill-handoff', draftId, '', {
        handoff_request_id: handoffRequestId,
      });
    }
  }

  const brief = validIds.length > 0
    ? buildHandoffBrief({ sourceIds: validIds, idempotencyKey: handoffRequestId })
    : '';

  const handleCopy = async () => {
    if (!brief) return;
    try {
      await navigator.clipboard.writeText(brief);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback or ignore
    }
  };


  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Header */}
      <div>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_DISTILL}</span>
        </h1>
      </div>

      {/* Selected sources card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span style={{ fontSize: '15px', fontWeight: 600 }}>
            {LABEL_SELECTED_SOURCES}{LABEL_LPAREN}{validIds.length}{LABEL_RPAREN}
          </span>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          {classified.map((c) => {
            if (!c.valid) {
              return (
                <div
                  key={c.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: 'var(--space-2) var(--space-3)',
                    backgroundColor: 'var(--color-surface-sunken)',
                    borderRadius: 'var(--radius-sm)',
                    border: '1px solid var(--color-danger-subtle, #fca5a5)',
                    fontSize: '13px',
                  }}
                >
                  <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600 }}>{c.id}</span>
                  <span style={{ color: 'var(--color-danger, #dc2626)' }}>{c.error}</span>
                </div>
              );
            }

            const src = c.source!;
            const curRev = src.current_revision?.value || 'unknown';
            const distRev = src.distilled_revision?.value || 'never';
            const distill = distillLabel(src);

            return (
              <div
                key={c.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: 'var(--space-2) var(--space-3)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--radius-sm)',
                  flexWrap: 'wrap',
                  gap: 'var(--space-2)',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600, fontSize: '14px' }}>
                    {c.id}
                  </span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', color: 'var(--color-text-muted)' }}>
                    {curRev}{LABEL_ARROW}{distRev}
                  </span>
                  <StatusBadge variant="chip" tone="info" label={distill.label} />
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {/* Brief Codeblock */}
      {brief && (
        <section className="fg-codeblock" style={{ border: '1px solid var(--color-border)', borderRadius: 'var(--radius-sm)', overflow: 'hidden' }}>
          <div
            className="fg-codeblock__bar"
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: 'var(--space-2) var(--space-3)',
              backgroundColor: 'var(--color-surface)',
              borderBottom: '1px solid var(--color-border)',
            }}
          >
            <span className="t-label" style={{ fontWeight: 600, fontSize: '13px' }}>
              {TITLE_BRIEF_BAR}
            </span>
            <button
              type="button"
              className="fg-btn fg-btn--secondary fg-btn--small"
              onClick={() => void handleCopy()}
            >
              <span>{copied ? LABEL_COPIED : BTN_COPY_HANDOFF}</span>
            </button>
          </div>
          <pre
            tabIndex={0}
            aria-label={TITLE_BRIEF_BAR}
            style={{
              margin: 0,
              padding: 'var(--space-4)',
              maxHeight: '300px',
              overflow: 'auto',
              fontFamily: 'var(--font-mono)',
              fontSize: '12.5px',
              lineHeight: 1.6,
              backgroundColor: 'var(--color-surface-sunken)',
              whiteSpace: 'pre-wrap',
            }}
          >
            {brief}
          </pre>
        </section>
      )}

    </div>
  );
}
