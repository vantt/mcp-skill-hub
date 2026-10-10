import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { useSources } from '../../api/queries';
import type { SourceSummary } from '../../api/types';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { buildHandoffBrief } from '../../domain/handoff-brief';
import { distillLabel } from '../../domain/source-distill';

const TITLE_DISTILL = 'Distill with Curator Agent';
const LABEL_SELECTED_SOURCES = 'Selected sources';
const BTN_BACK_SOURCES = 'Back to Sources';
const INTRO_PICK =
  'A curator agent reads a learning source and records lessons for the skills linked to it. Pick the sources to hand over; nothing is changed until you decide on a lesson.';
const LABEL_PICK_SOURCES = 'Sources ready to distill';
const BTN_CONTINUE = 'Continue with selected sources';
const MSG_NO_SOURCES =
  'There are no sources yet. A source is a repository your skills learn from. Open a skill, go to its Sources tab and add a learning reference.';
const MSG_NONE_READY =
  'No source is ready to distill. A source becomes ready when a check finds new commits in it, or when it was never distilled.';
const BTN_OPEN_SOURCES = 'Open Sources';
const BTN_CHANGE_SELECTION = 'Change selection';
const INTRO_BRIEF =
  'Copy this handoff and paste it to your curator agent, an agent that has the distill-lab skill. It records lessons in each skill; read them in the skill\'s Distill tab, where you decide what to keep.';
const LABEL_SKILLS = 'Skills: ';
const LABEL_NO_SKILL = 'no linked skill';
const TITLE_BRIEF_BAR = 'Handoff for your Curator Agent';
const BTN_COPY_HANDOFF = 'Copy handoff';
const LABEL_COPIED = 'Copied';
const LABEL_ARROW = ' ← ';
const LABEL_LPAREN = ' (';
const LABEL_RPAREN = ')';

function shortRevision(value?: string): string {
  if (!value) return '';
  const colon = value.indexOf(':');
  return value.slice(0, colon + 1) + value.slice(colon + 1, colon + 11);
}

interface ClassifiedSource {
  id: string;
  valid: boolean;
  error?: string;
  source?: SourceSummary;
}

export function DistillHandoffScreen() {
  const [searchParams] = useSearchParams();
  const requestedSourceIds = searchParams.getAll('source');

  const { data: sourcesData, isLoading, error: sourcesError } = useSources();

  const [copied, setCopied] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="32px" />
        <Skeleton height="100px" />
        <Skeleton height="220px" />
      </div>
    );
  }

  if (sourcesError && !sourcesData) {
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

  if (requestedSourceIds.length === 0) {
    const ready = allSources.filter((src) => distillLabel(src).selectable);
    const pickedQuery = picked.map((id) => `source=${encodeURIComponent(id)}`).join('&');
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_DISTILL}</span>
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: 'var(--color-text-muted)', maxWidth: '64ch' }}>
          <span>{INTRO_PICK}</span>
        </p>
        {ready.length === 0 ? (
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
            <span style={{ color: 'var(--color-text-muted)', fontSize: '14px', textAlign: 'center', maxWidth: '56ch' }}>
              {allSources.length === 0 ? MSG_NO_SOURCES : MSG_NONE_READY}
            </span>
            <Link to="/sources" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
              <span>{BTN_OPEN_SOURCES}</span>
            </Link>
          </div>
        ) : (
          <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            <div className="fg-card__title">
              <span style={{ fontSize: '15px', fontWeight: 600 }}>{LABEL_PICK_SOURCES}</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              {ready.map((src) => (
                <label
                  key={src.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 'var(--space-3)',
                    flexWrap: 'wrap',
                    padding: 'var(--space-2) var(--space-3)',
                    border: '1px solid var(--color-border)',
                    borderRadius: 'var(--radius-sm)',
                    cursor: 'pointer',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={picked.includes(src.id)}
                    onChange={(e) =>
                      setPicked((prev) =>
                        e.target.checked ? [...prev, src.id] : prev.filter((id) => id !== src.id),
                      )
                    }
                  />
                  <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600, fontSize: '14px' }}>{src.id}</span>
                  <StatusBadge variant="chip" tone="warning" label={distillLabel(src).label} />
                  <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
                    {LABEL_SKILLS}
                    {src.referencing_skills.length > 0 ? src.referencing_skills.join(', ') : LABEL_NO_SKILL}
                  </span>
                </label>
              ))}
            </div>
            <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
              {picked.length > 0 ? (
                <Link
                  to={`/sources/distill?${pickedQuery}`}
                  className="fg-btn fg-btn--primary"
                  style={{ textDecoration: 'none' }}
                >
                  <span>{BTN_CONTINUE}</span>
                </Link>
              ) : (
                <button type="button" className="fg-btn fg-btn--primary" disabled>
                  <span>{BTN_CONTINUE}</span>
                </button>
              )}
              <Link to="/sources" className="fg-btn fg-btn--secondary" style={{ textDecoration: 'none' }}>
                <span>{BTN_BACK_SOURCES}</span>
              </Link>
            </div>
          </section>
        )}
      </div>
    );
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

  const brief =
    validSources.length > 0
      ? buildHandoffBrief({
          sources: validSources.map((c) => ({ id: c.id, skills: c.source?.referencing_skills ?? [] })),
        })
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_DISTILL}</span>
        </h1>
        <Link to="/sources/distill" className="fg-btn fg-btn--secondary fg-btn--small" style={{ textDecoration: 'none' }}>
          <span>{BTN_CHANGE_SELECTION}</span>
        </Link>
      </div>
      <p style={{ margin: 0, fontSize: '14px', color: 'var(--color-text-muted)', maxWidth: '64ch' }}>
        <span>{INTRO_BRIEF}</span>
      </p>

      {/* Selected sources card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span style={{ fontSize: '15px', fontWeight: 600 }}>
            {LABEL_SELECTED_SOURCES}{LABEL_LPAREN}{validSources.length}{LABEL_RPAREN}
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
                    border: '1px solid var(--color-danger)',
                    fontSize: '13px',
                  }}
                >
                  <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600 }}>{c.id}</span>
                  <span style={{ color: 'var(--color-danger)' }}>{c.error}</span>
                </div>
              );
            }

            const src = c.source!;
            const curRev = shortRevision(src.current_revision?.value) || 'unknown';
            const distRev = shortRevision(src.distilled_revision?.value) || 'never';
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
                  <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
                    {LABEL_SKILLS}
                    {src.referencing_skills.length > 0 ? src.referencing_skills.join(', ') : LABEL_NO_SKILL}
                  </span>
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
