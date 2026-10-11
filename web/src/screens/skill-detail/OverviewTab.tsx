import type { SkillDetail, SkillReviewResult, SkillSourcesResult } from '../../api/types';

const TITLE_DESCRIPTION = 'Description';
const LABEL_NO_DESCRIPTION = 'No description yet.';

export interface OverviewTabProps {
  skill: SkillDetail;
  review?: SkillReviewResult;
  sources?: SkillSourcesResult;
  sourcesLoading: boolean;
  sourcesError: unknown;
  onOpenTab: (tab: 'sources' | 'editor') => void;
}

export function OverviewTab({ skill }: OverviewTabProps) {
  const description = skill.description.trim();
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 320px), 1fr))',
        gap: 'var(--space-4)',
        alignItems: 'start',
      }}
    >
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_DESCRIPTION}</span>
        </div>
        <p
          className="t-body-sm app-wrap"
          style={{ margin: 0, color: description ? undefined : 'var(--color-text-muted)' }}
        >
          {description || LABEL_NO_DESCRIPTION}
        </p>
      </section>
    </div>
  );
}
