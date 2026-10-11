import { useSearchParams } from 'react-router';
import { ApiError } from '../../api/client';
import { useSkillResources } from '../../api/queries';
import type { ResourceUnavailableReason, SkillDetail, SkillResourceItem } from '../../api/types';
import { ResourceFileView } from './ResourceFileView';

const LABEL_PATH = 'Path';
const LABEL_KIND = 'Kind';
const LABEL_SIZE = 'Size';
const LABEL_DIGEST = 'Digest';
const LABEL_STATE = 'State';
const LABEL_ACTIONS = 'Actions';
const LABEL_EDIT = 'Edit';
const LABEL_VIEW = 'View';
const LABEL_NO_RESOURCES =
  'No companion resources. This skill is only its SKILL.md. Scripts, reference documents or other files you put in the skill folder are listed here.';
const NOTICE_RESOURCES =
  'Changed resource: if only the catalog is out of date, run skillhub rebuild. If a file is missing, restore it or re-add the skill. Edits go through a preview you confirm; nothing is saved without it.';
const LABEL_ACTIVE = 'Active';
const ELLIPSIS = '…';

const PARAM_FILE = 'file';
const PARAM_EDIT = 'edit';

const REASON_LABELS: Record<Exclude<ResourceUnavailableReason, ''>, string> = {
  binary: 'Binary',
  too_large: 'Too large to edit',
  missing: 'Missing on disk',
  not_regular: 'Not a regular file',
};

interface ResourcesTabProps {
  skill: SkillDetail;
  workspaceId: string;
  thirdParty: boolean;
}

function errorText(err: unknown): string {
  if (err instanceof ApiError) return err.render.WHY || err.render.ERROR || err.message;
  return err instanceof Error ? err.message : String(err);
}

export function ResourcesTab({ skill, workspaceId, thirdParty }: ResourcesTabProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const resourcesQuery = useSkillResources(skill.skill_id);
  const openFile = searchParams.get(PARAM_FILE);

  const openResource = (path: string, edit: boolean) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set(PARAM_FILE, path);
      if (edit) next.set(PARAM_EDIT, '1');
      else next.delete(PARAM_EDIT);
      return next;
    });
  };

  const closeResource = () => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete(PARAM_FILE);
      next.delete(PARAM_EDIT);
      return next;
    });
  };

  if (openFile) {
    return (
      <ResourceFileView
        skill={skill}
        workspaceId={workspaceId}
        path={openFile}
        startInEdit={searchParams.get(PARAM_EDIT) === '1'}
        thirdParty={thirdParty}
        onClose={closeResource}
      />
    );
  }

  // The Editor tab owns SKILL.md; this tab lists only the companion files.
  const companionResources = (skill.resources || []).filter((r) => r.path !== skill.path);

  if (companionResources.length === 0) {
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

  const itemsByPath = new Map<string, SkillResourceItem>(
    (resourcesQuery.data?.items ?? []).map((item) => [item.path, item]),
  );
  const loading = resourcesQuery.isPending;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {resourcesQuery.isError && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{errorText(resourcesQuery.error)}</span>
        </div>
      )}

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
              <th style={{ textAlign: 'right' }}>
                <span>{LABEL_ACTIONS}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {companionResources.map((res) => {
              const sizeText = `${res.size_bytes} B`;
              const digestText = `${res.digest.slice(0, 16)}${ELLIPSIS}`;
              // A catalog row the working tree does not report (catalog older than the files) is view-only.
              const item = itemsByPath.get(res.path);
              const editable = item?.editable === true;
              const reasonText = !editable && item?.reason ? REASON_LABELS[item.reason] : '';
              return (
                <tr key={res.path}>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                    <span className="app-wrap">{res.path}</span>
                  </td>
                  <td className="t-body-sm">
                    <span>{res.kind}</span>
                  </td>
                  <td className="fg-table__num">
                    <span>{sizeText}</span>
                    {reasonText && (
                      <div className="t-caption" style={{ color: 'var(--color-text-subtle)', whiteSpace: 'nowrap' }}>
                        {reasonText}
                      </div>
                    )}
                  </td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>{digestText}</span>
                  </td>
                  <td>
                    <span className="fg-chip fg-chip--neutral">
                      <span>{LABEL_ACTIVE}</span>
                    </span>
                  </td>
                  <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                    {editable ? (
                      <button
                        type="button"
                        className="fg-btn fg-btn--primary fg-btn--small"
                        aria-label={`${LABEL_EDIT} ${res.path}`}
                        disabled={loading}
                        onClick={() => openResource(res.path, true)}
                      >
                        <span>{LABEL_EDIT}</span>
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        aria-label={`${LABEL_VIEW} ${res.path}`}
                        disabled={loading}
                        onClick={() => openResource(res.path, false)}
                      >
                        <span>{LABEL_VIEW}</span>
                      </button>
                    )}
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
