import { useSkillDistill } from '../../api/queries';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_KNOWLEDGE = 'Distillation Knowledge';
const MSG_NO_DOCUMENT = 'No distillation document (.meta/distill.yaml) recorded for this skill yet.';
const TITLE_GOAL = 'Distillation Goal';
const MSG_NO_GOAL = 'No explicit distillation goal stated.';
const MSG_NO_LESSONS = 'No lessons recorded yet.';
const LABEL_WHAT = 'What: ';
const LABEL_NOTABLE = 'Notable: ';
const LABEL_CONTRAST = 'Contrast: ';
const LABEL_LAYER = 'Layer: ';
const LABEL_DECISION = 'Decision: ';
const LABEL_REASON = 'Reason: ';
const LABEL_RELEVANCE = 'Relevance: ';
const LABEL_IMPACT = 'Impact: ';
const LABEL_EVIDENCE = 'Evidence: ';
const LABEL_EFFORT = 'Effort: ';
const LABEL_TRACKED_CURSORS = 'Tracked Cursors (';
const LABEL_COVERAGE_ANALYSIS = 'Coverage Analysis (';
const LABEL_DISTILLED_LESSONS = 'Distilled Lessons (';
const LABEL_CITATIONS = 'Evidence citations (';
const LABEL_RPAREN = ')';
const LABEL_AT = 'at ';
const LABEL_READ = 'read: ';
const LABEL_NOT_READ = 'not read: ';

const LABEL_FILES = ' files';
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

  const decisionTone = (state: string): 'success' | 'warning' | 'danger' | 'info' | 'neutral' => {
    switch (state) {
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

  const cursorEntries = Object.entries(doc.cursors || {});
  const coverageEntries = Object.entries(doc.coverage || {});

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Goal */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div className="fg-card__title">
            <span>{TITLE_GOAL}</span>
          </div>
          {doc.goal?.status && (
            <StatusBadge
              label={doc.goal.status}
              tone={doc.goal.status === 'confirmed' ? 'success' : 'neutral'}
              variant="chip"
            />
          )}
        </div>
        <p style={{ margin: 0, fontSize: '14px', lineHeight: 1.5 }}>
          <span>{doc.goal?.purpose || MSG_NO_GOAL}</span>
        </p>
      </section>

      {/* Cursors */}
      {cursorEntries.length > 0 && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span>{LABEL_TRACKED_CURSORS}</span>
            <span>{cursorEntries.length}</span>
            <span>{LABEL_RPAREN}</span>
          </div>
          <div className="fg-facts">
            {cursorEntries.map(([sourceId, commit]) => (
              <div key={sourceId} className="fg-fact">
                <div className="fg-fact__label">
                  <span>{sourceId}</span>
                </div>
                <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                  <span>{commit}</span>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Coverage */}
      {coverageEntries.length > 0 && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span>{LABEL_COVERAGE_ANALYSIS}</span>
            <span>{coverageEntries.length}</span>
            <span>{LABEL_RPAREN}</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            {coverageEntries.map(([srcId, cov]) => (
              <div key={srcId} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
                <div style={{ fontWeight: 600, fontSize: '13px' }}>
                  <span>{srcId}</span>
                </div>
                {cov.read && cov.read.length > 0 && (
                  <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>{LABEL_READ}</span>
                    <span>{cov.read.length}</span>
                    <span>{LABEL_FILES}</span>
                  </div>
                )}
                {cov.not_read && cov.not_read.length > 0 && (
                  <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>{LABEL_NOT_READ}</span>
                    <span>{cov.not_read.length}</span>
                    <span>{LABEL_FILES}</span>
                  </div>
                )}
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
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                    <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 600, fontSize: '14px' }}>
                      <span>{lesson.key}</span>
                    </span>
                    <span className="fg-chip fg-chip--neutral" style={{ fontSize: '11px', padding: '1px 6px' }}>
                      <span>{LABEL_LAYER}</span>
                      <span>{lesson.layer}</span>
                    </span>
                  </div>
                  <StatusBadge
                    label={lesson.decision?.state || 'candidate'}
                    tone={decisionTone(lesson.decision?.state || 'candidate')}
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

                {lesson.score && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>
                      <span>{LABEL_RELEVANCE}</span>
                      <span>{lesson.score.relevance}</span>
                    </span>
                    <span>
                      <span>{LABEL_IMPACT}</span>
                      <span>{lesson.score.impact}</span>
                    </span>
                    <span>
                      <span>{LABEL_EVIDENCE}</span>
                      <span>{lesson.score.evidence}</span>
                    </span>
                    <span>
                      <span>{LABEL_EFFORT}</span>
                      <span>{lesson.score.effort}</span>
                    </span>
                  </div>
                )}

                {lesson.where && lesson.where.length > 0 && (
                  <div style={{ marginTop: 'var(--space-1)' }}>
                    <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-text-muted)', marginBottom: 'var(--space-1)' }}>
                      <span>{LABEL_CITATIONS}</span>
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
                      <span>{lesson.decision.state}</span>
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
