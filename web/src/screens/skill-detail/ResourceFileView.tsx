import type { SkillDetail } from '../../api/types';

const LABEL_BACK = 'Back to resources';

export interface ResourceFileViewProps {
  skill: SkillDetail;
  workspaceId: string;
  path: string; // workspace-relative path from the list
  startInEdit: boolean; // true when opened with Edit
  thirdParty: boolean;
  onClose: () => void;
}

// Shows one companion file of a skill. For now it names the file and offers the way back to the list.
export function ResourceFileView({ path, onClose }: ResourceFileViewProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-3)' }}>
        <span className="app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', flex: '1 1 260px' }}>
          {path}
        </span>
        <button type="button" className="fg-btn fg-btn--secondary" onClick={onClose}>
          <span>{LABEL_BACK}</span>
        </button>
      </div>
    </div>
  );
}
