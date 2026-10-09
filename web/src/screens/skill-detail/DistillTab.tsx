import { useSkillDistill } from '../../api/queries';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_KNOWLEDGE = 'Distillation Knowledge';
const MSG_NO_DOCUMENT = 'No distillation document (.meta/distill.yaml) recorded for this skill yet.';
const TITLE_GOAL = 'Distillation Goal';
const MSG_NO_GOAL = 'No explicit distillation goal stated.';
const MSG_NO_LESSONS = 'No lessons recorded yet.';
const LABEL_BLOCKING = 'Blocking';
const LABEL_WHAT = 'What: ';
const LABEL_NOTABLE = 'Notable: ';
const LABEL_CONTRAST = 'Contrast: ';
const LABEL_DECISION = 'Decision: ';
const LABEL_REASON = 'Reason: ';
const LABEL_RELEVANCE = 'Relevance: ';
const LABEL_QUALITY = 'Quality: ';
const LABEL_FIT = 'Fit: ';
const LABEL_TRACKED_CURSORS = 'Tracked Cursors (';
const LABEL_COVERAGE_ANALYSIS = 'Coverage Analysis (';
const LABEL_DISTILLED_LESSONS = 'Distilled Lessons (';
const LABEL_EVIDENCE = 'Evidence (';
const LABEL_LPAREN = '(';
const LABEL_RPAREN = ')';
const LABEL_AT = 'at ';

interface DistillTabProps {
  skillId: string;
}

export function DistillTab({ skillId }: DistillTabProps) {
  const { data: doc, isLoading, error } = useSkillDistill(skillId);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="24px" />
        <Skeleton height="120px" />
        <Skeleton height="200px" />
      </div>
    );
  }

  if (error || !doc) {
    return (
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_KNOWLEDGE}</span>
        </div>
        <p style={{ color: 'var(--color-text-muted)', margin: 0, fontSize: '13px' }}>
          <span>{MSG_NO_DOCUMENT}</span>
        </p>
      </section>
    );
  }

  const decisionTone = (status: string): 'success' | 'warning' | 'danger' | 'info' | 'neutral' => {
    switch (status) {
      case 'ported':
        return 'success';
      case 'planned':
        return 'warning';
      case 'rejected':
        return 'danger';
      case 'candidate':
        return 'info';
      default:
        return 'neutral';
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Goal */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_GOAL}</span>
        </div>
        <p style={{ margin: 0, fontSize: '14px', lineHeight: 1.5 }}>
          <span>{doc.goal || MSG_NO_GOAL}</span>
        </p>
      </section>

      {/* Cursors */}
      {doc.cursors && doc.cursors.length > 0 && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span>{LABEL_TRACKED_CURSORS}</span>
            <span>{doc.cursors.length}</span>
            <span>{LABEL_RPAREN}</span>
          </div>
          <div className="fg-facts">
            {doc.cursors.map((c) => (
              <div key={c.source_id} className="fg-fact">
                <div className="fg-fact__label">
                  <span>{c.source_id}</span>
                </div>
                <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                  <span>{c.commit}</span>
                  {c.synced_at && (
                    <span style={{ color: 'var(--color-text-muted)', marginLeft: 'var(--space-2)' }}>
                      <span>{LABEL_LPAREN}</span>
                      <span>{new Date(c.synced_at).toLocaleString()}</span>
                      <span>{LABEL_RPAREN}</span>
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Coverage */}
      {doc.coverage && doc.coverage.length > 0 && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span>{LABEL_COVERAGE_ANALYSIS}</span>
            <span>{doc.coverage.length}</span>
            <span>{LABEL_RPAREN}</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            {doc.coverage.map((entry, idx) => (
              <div
                key={idx}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  padding: 'var(--space-2) 0',
                  borderBottom: idx < doc.coverage!.length - 1 ? '1px solid var(--color-border)' : 'none',
                }}
              >
                <div>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 500 }}>
                    <span>{entry.resource}</span>
                  </div>
                  {entry.reason && (
                    <div style={{ color: 'var(--color-text-muted)', fontSize: '12px' }}>
                      <span>{entry.reason}</span>
                    </div>
                  )}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                  {entry.blocking && (
                    <StatusBadge label={LABEL_BLOCKING} tone="warning" variant="chip" />
                  )}
                  <StatusBadge
                    label={entry.status}
                    tone={entry.status === 'analyzed' ? 'success' : 'neutral'}
                    variant="chip"
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Lessons */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div className="fg-card__title">
            <span>{LABEL_DISTILLED_LESSONS}</span>
            <span>{doc.lessons?.length ?? 0}</span>
            <span>{LABEL_RPAREN}</span>
          </div>
        </div>

        {(!doc.lessons || doc.lessons.length === 0) ? (
          <p style={{ color: 'var(--color-text-muted)', margin: 0, fontSize: '13px' }}>
            <span>{MSG_NO_LESSONS}</span>
          </p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            {doc.lessons.map((lesson) => (
              <div
                key={lesson.key}
                className="fg-card"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 'var(--space-2)',
                  backgroundColor: 'var(--color-surface-subtle)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 'var(--space-2)' }}>
                  <div>
                    <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600, fontSize: '14px' }}>
                      <span>{lesson.key}</span>
                    </span>
                  </div>
                  <StatusBadge
                    label={lesson.decision?.status || 'candidate'}
                    tone={decisionTone(lesson.decision?.status || 'candidate')}
                    variant="chip"
                  />
                </div>

                <div style={{ fontSize: '14px', lineHeight: 1.5 }}>
                  <strong><span>{LABEL_WHAT}</span></strong>
                  <span>{lesson.what}</span>
                </div>

                {lesson.notable && (
                  <div style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
                    <strong><span>{LABEL_NOTABLE}</span></strong>
                    <span>{lesson.notable}</span>
                  </div>
                )}

                {lesson.contrast && (
                  <div style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
                    <strong><span>{LABEL_CONTRAST}</span></strong>
                    <span>{lesson.contrast}</span>
                  </div>
                )}

                {lesson.scores && (
                  <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    {lesson.scores.relevance !== undefined && (
                      <span>
                        <span>{LABEL_RELEVANCE}</span>
                        <span>{lesson.scores.relevance}</span>
                      </span>
                    )}
                    {lesson.scores.evidence_quality !== undefined && (
                      <span>
                        <span>{LABEL_QUALITY}</span>
                        <span>{lesson.scores.evidence_quality}</span>
                      </span>
                    )}
                    {lesson.scores.fit !== undefined && (
                      <span>
                        <span>{LABEL_FIT}</span>
                        <span>{lesson.scores.fit}</span>
                      </span>
                    )}
                  </div>
                )}

                {lesson.where && lesson.where.length > 0 && (
                  <div style={{ marginTop: 'var(--space-1)' }}>
                    <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-text-muted)', marginBottom: 'var(--space-1)' }}>
                      <span>{LABEL_EVIDENCE}</span>
                      <span>{lesson.where.length}</span>
                      <span>{LABEL_RPAREN}</span>
                    </div>
                    <ul style={{ margin: 0, paddingLeft: 'var(--space-4)', fontSize: '12px', fontFamily: 'var(--font-mono)' }}>
                      {lesson.where.map((loc, idx) => (
                        <li key={idx} style={{ wordBreak: 'break-all' }}>
                          <span>{loc}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {lesson.decision && (
                  <div style={{ marginTop: 'var(--space-1)', padding: 'var(--space-2)', backgroundColor: 'var(--color-surface)', borderRadius: 'var(--radius-sm)', fontSize: '12px' }}>
                    <div>
                      <strong><span>{LABEL_DECISION}</span></strong>
                      <span>{lesson.decision.status}</span>
                      {lesson.decision.at && (
                        <span style={{ color: 'var(--color-text-muted)', marginLeft: 'var(--space-2)' }}>
                          <span>{LABEL_AT}</span>
                          <span>{new Date(lesson.decision.at).toLocaleString()}</span>
                        </span>
                      )}
                    </div>
                    {lesson.decision.reason && (
                      <div style={{ marginTop: '2px', color: 'var(--color-text-muted)' }}>
                        <span>{LABEL_REASON}</span>
                        <span>{lesson.decision.reason}</span>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
