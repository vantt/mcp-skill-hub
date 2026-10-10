import type { SkillDetail } from '../../api/types';

const LABEL_PATH = 'Path';
const LABEL_KIND = 'Kind';
const LABEL_SIZE = 'Size';
const LABEL_DIGEST = 'Digest';
const LABEL_STATE = 'State';
const LABEL_NO_RESOURCES =
  'No companion resources. This skill is only its SKILL.md. Scripts, reference documents or other files you put in the skill folder are listed here.';
const NOTICE_RESOURCES =
  'Changed resource: if only the catalog is out of date, run skillhub rebuild. If a file is missing, restore it or re-add the skill. Resource content is not viewable in v1.';
const LABEL_ACTIVE = 'Active';
const ELLIPSIS = '…';

interface ResourcesTabProps {
  skill: SkillDetail;
}

export function ResourcesTab({ skill }: ResourcesTabProps) {
  const resources = skill.resources || [];
  const companionResources = resources.filter((r) => !r.path.endsWith('SKILL.md'));

  if (resources.length === 0 || companionResources.length === 0) {
    return (
      <div className="fg-card">
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 'var(--space-8) var(--space-4)',
            textAlign: 'center',
          }}
        >
          <span className="t-body">
            <span>{LABEL_NO_RESOURCES}</span>
          </span>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <div className="fg-card" style={{ padding: 0, overflow: 'auto' }}>
        <table className="fg-table" style={{ width: '100%' }}>
          <thead>
            <tr>
              <th>
                <span>{LABEL_PATH}</span>
              </th>
              <th>
                <span>{LABEL_KIND}</span>
              </th>
              <th className="fg-table__num">
                <span>{LABEL_SIZE}</span>
              </th>
              <th>
                <span>{LABEL_DIGEST}</span>
              </th>
              <th>
                <span>{LABEL_STATE}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {resources.map((res) => {
              const sizeText = `${res.size_bytes} B`;
              const digestText = `${res.digest.slice(0, 16)}${ELLIPSIS}`;
              return (
                <tr key={res.path}>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                    <span>{res.path}</span>
                  </td>
                  <td className="t-body-sm">
                    <span>{res.kind}</span>
                  </td>
                  <td className="fg-table__num">
                    <span>{sizeText}</span>
                  </td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>{digestText}</span>
                  </td>
                  <td>
                    <span className="fg-chip fg-chip--neutral">
                      <span>{LABEL_ACTIVE}</span>
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="fg-caveat fg-caveat--info">
        <span>{NOTICE_RESOURCES}</span>
      </div>
    </div>
  );
}
