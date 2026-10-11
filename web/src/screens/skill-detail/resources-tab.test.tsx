import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SkillDetail, SkillResourceContent, SkillResourceList } from '../../api/types';
import { ResourcesTab } from './ResourcesTab';

const SKILL_ID = 'tools-skill';
const DIR = 'skills/core/tools-skill';
const ENTRY = `${DIR}/SKILL.md`;
const SCRIPT = `${DIR}/scripts/run.sh`;
const LOGO = `${DIR}/assets/logo.png`;
const DUMP = `${DIR}/references/dump.txt`;
const NEW_DOC = `${DIR}/references/new.md`;
const DIGEST = 'sha256:0123456789abcdef0123456789abcdef';

const skill: SkillDetail = {
  skill_id: SKILL_ID,
  name: 'tools-skill',
  description: 'Runs tools.',
  status: 'ok',
  path: ENTRY,
  catalog_snapshot: 'snap',
  content: '# tools',
  content_digest: DIGEST,
  state_basis: 'catalog',
  lifecycle_state: 'active',
  routing_eligible: true,
  diverged: false,
  routing: { operations: [], triggers: [], not_for: [], min_scope: 'project' },
  resources: [
    { path: ENTRY, kind: 'instructions', digest: DIGEST, size_bytes: 120 },
    { path: SCRIPT, kind: 'script', digest: DIGEST, size_bytes: 64 },
    { path: LOGO, kind: 'asset', digest: DIGEST, size_bytes: 2048 },
    { path: DUMP, kind: 'reference', digest: DIGEST, size_bytes: 3_000_000 },
    { path: NEW_DOC, kind: 'reference', digest: DIGEST, size_bytes: 10 },
  ],
};

const resourceList: SkillResourceList = {
  skill_id: SKILL_ID,
  items: [
    { path: SCRIPT, kind: 'script', size_bytes: 64, digest: DIGEST, editable: true, reason: '' },
    { path: LOGO, kind: 'asset', size_bytes: 2048, digest: DIGEST, editable: false, reason: 'binary' },
    { path: DUMP, kind: 'reference', size_bytes: 3_000_000, digest: DIGEST, editable: false, reason: 'too_large' },
  ],
};

const scriptContent: SkillResourceContent = {
  skill_id: SKILL_ID,
  path: SCRIPT,
  size_bytes: 64,
  digest: DIGEST,
  editable: true,
  reason: '',
  line_ending: 'lf',
  truncated: false,
  content: '#!/bin/sh\necho run\n',
};

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function currentSearch(): URLSearchParams {
  return new URLSearchParams(screen.getByTestId('location-search').textContent ?? '');
}

function renderTab(url: string, seed: (client: QueryClient) => void = () => {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  seed(queryClient);
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route
            path="/skills/:id"
            element={
              <>
                <ResourcesTab skill={skill} workspaceId="ws-1" thirdParty={false} />
                <LocationProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function seedList(client: QueryClient) {
  client.setQueryData(['skill-resources', SKILL_ID], resourceList);
  client.setQueryData(['skill-resource', SKILL_ID, SCRIPT], scriptContent);
}

function rowFor(path: string): HTMLElement {
  const row = screen.getByText(path).closest('tr');
  if (!row) throw new Error(`no table row for ${path}`);
  return row;
}

describe('ResourcesTab', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('offers Edit only for editable files', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    expect(screen.getByRole('button', { name: `Edit ${SCRIPT}` })).toBeEnabled();
    expect(screen.queryByRole('button', { name: `Edit ${LOGO}` })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: `Edit ${DUMP}` })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: `View ${SCRIPT}` })).not.toBeInTheDocument();
  });

  it('offers View with the reason for binary and too-large files', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    expect(screen.getByRole('button', { name: `View ${LOGO}` })).toBeEnabled();
    expect(within(rowFor(LOGO)).getByText('Binary')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: `View ${DUMP}` })).toBeEnabled();
    expect(within(rowFor(DUMP)).getByText('Too large to edit')).toBeInTheDocument();
  });

  it('offers only View for a catalog row the editability list does not report', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    expect(screen.getByRole('button', { name: `View ${NEW_DOC}` })).toBeEnabled();
    expect(screen.queryByRole('button', { name: `Edit ${NEW_DOC}` })).not.toBeInTheDocument();
  });

  it('labels each cell so a narrow screen can stack a row with the action beside the path', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    const row = rowFor(SCRIPT);
    const labels = Array.from(row.querySelectorAll('td[data-label]')).map((td) => td.getAttribute('data-label'));
    expect(labels).toEqual(['Kind', 'Size', 'Digest', 'State']);
    const actions = row.querySelector('td.app-resources__actions');
    expect(actions).not.toBeNull();
    expect(within(actions as HTMLElement).getByRole('button', { name: `Edit ${SCRIPT}` })).toBeInTheDocument();
  });

  it('leaves SKILL.md out of the table', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    expect(screen.queryByText(ENTRY)).not.toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(1 + 4);
  });

  it('tells the person that edits are previewed before they are saved', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    expect(screen.getByText(/nothing is saved without it/)).toBeInTheDocument();
    expect(screen.queryByText(/not viewable/)).not.toBeInTheDocument();
  });

  it('opens the file in edit mode from Edit and returns to the list from Back', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    fireEvent.click(screen.getByRole('button', { name: `Edit ${SCRIPT}` }));

    expect(currentSearch().get('tab')).toBe('resources');
    expect(currentSearch().get('file')).toBe(SCRIPT);
    expect(currentSearch().get('edit')).toBe('1');
    expect(screen.queryByRole('table')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Back to resources' }));

    expect(currentSearch().get('tab')).toBe('resources');
    expect(currentSearch().has('file')).toBe(false);
    expect(currentSearch().has('edit')).toBe(false);
    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  it('drops edit from the address when the person leaves edit mode', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources`, seedList);

    fireEvent.click(screen.getByRole('button', { name: `Edit ${SCRIPT}` }));
    expect(currentSearch().get('edit')).toBe('1');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel edit' }));
    expect(currentSearch().get('file')).toBe(SCRIPT);
    expect(currentSearch().has('edit')).toBe(false);
  });

  it('opens the file without edit mode from View', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources&edit=1`, seedList);

    fireEvent.click(screen.getByRole('button', { name: `View ${LOGO}` }));

    expect(currentSearch().get('file')).toBe(LOGO);
    expect(currentSearch().has('edit')).toBe(false);
    expect(screen.getByRole('button', { name: 'Back to resources' })).toBeInTheDocument();
  });

  it('opens the file view directly from the URL', () => {
    renderTab(`/skills/${SKILL_ID}?tab=resources&file=${encodeURIComponent(SCRIPT)}&edit=1`, seedList);

    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Back to resources' })).toBeInTheDocument();
  });

  it('disables the actions while editability is loading', () => {
    vi.spyOn(globalThis, 'fetch').mockReturnValue(new Promise<Response>(() => {}));
    renderTab(`/skills/${SKILL_ID}?tab=resources`);

    expect(screen.getByRole('button', { name: `View ${SCRIPT}` })).toBeDisabled();
    expect(screen.getByRole('button', { name: `View ${LOGO}` })).toBeDisabled();
  });

  it('shows one danger banner and keeps the table when the list fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 'internal_error',
          render: { ERROR: 'Could not list resources.', WHY: 'The skill folder is unreadable.', FIX: 'Retry.' },
        }),
        { status: 500, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    renderTab(`/skills/${SKILL_ID}?tab=resources`);

    const alerts = await screen.findAllByRole('alert');
    expect(alerts).toHaveLength(1);
    expect(alerts[0]).toHaveTextContent('The skill folder is unreadable.');
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: `View ${SCRIPT}` })).toBeEnabled();
  });
});
