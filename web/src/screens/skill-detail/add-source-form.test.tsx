import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../api/client';
import * as queries from '../../api/queries';
import type { SourceListItem, SourceListResult, SourceProposal } from '../../api/types';
import { AddSourceForm } from './AddSourceForm';

function watched(id: string, repository: string, skills: string[] = []): SourceListItem {
  return {
    record: {
      id,
      adapter: 'github',
      locator: { repository },
      status: 'active',
      identity: { name: id },
      monitoring: { enabled: true, cadence: 'weekly' },
    },
    skills,
    role: 'learning-source',
    importable_count: 0,
  };
}

const pins = {
  proposal_id: 'PROP-ADD',
  proposal_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
  base_version: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
};

function proposal(sourceId: string): SourceProposal {
  return {
    schema_version: '1',
    status: 'action_required',
    summary: `Link ${sourceId} to my-skill as a learning reference.`,
    source: {
      id: sourceId,
      adapter: 'github',
      locator: { repository: `https://github.com/example/${sourceId}` },
      status: 'active',
      identity: { name: sourceId },
      monitoring: { enabled: true, cadence: 'weekly' },
    },
    diff: {
      added: [`sources/skills/LINK-my-skill-${sourceId}.yaml`],
      modified: [`sources/catalog/${sourceId}.yaml`],
      deleted: [],
    },
    warnings: [{ code: 'large_repository', summary: 'This repository is large; checks may be slow.' }],
    confirmation: { confirmation: { required: true, pins } },
  };
}

describe('AddSourceForm', () => {
  let queryClient: QueryClient;
  let onAdded: ReturnType<typeof vi.fn<(message: string) => void>>;
  let onCancel: ReturnType<typeof vi.fn<() => void>>;

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    onAdded = vi.fn<(message: string) => void>();
    onCancel = vi.fn<() => void>();
  });

  function renderForm(sources: SourceListItem[], opts: { linked?: string[]; upstream?: string } = {}) {
    const list: SourceListResult = { schema_version: '1', status: 'ok', summary: '', candidates: [], sources };
    queryClient.setQueryData(['sources'], list);
    return render(
      <QueryClientProvider client={queryClient}>
        <AddSourceForm
          skillId="my-skill"
          linkedSourceIds={opts.linked ?? []}
          upstreamSourceId={opts.upstream}
          onAdded={onAdded}
          onCancel={onCancel}
        />
      </QueryClientProvider>,
    );
  }

  it('offers only watched sources that are neither linked nor the upstream', () => {
    renderForm(
      [
        watched('docs-guide', 'https://github.com/example/docs-guide'),
        watched('already-linked', 'https://github.com/example/already-linked'),
        watched('linked-elsewhere', 'https://github.com/example/linked-elsewhere', ['my-skill']),
        watched('vendor-upstream', 'https://github.com/vendor/skills'),
      ],
      { linked: ['already-linked'], upstream: 'vendor-upstream' },
    );

    const select = screen.getByLabelText('Watched source');
    const values = within(select).getAllByRole('option').map((o) => (o as HTMLOptionElement).value);
    expect(values).toEqual(['docs-guide']);
  });

  it('explains why linking is unavailable when every watched source is taken', () => {
    renderForm([watched('already-linked', 'https://github.com/example/already-linked')], {
      linked: ['already-linked'],
    });

    // Defaults to watching a URL when nothing can be linked.
    expect(screen.getByLabelText(/GitHub repository URL/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('radio', { name: 'Link a watched source' }));
    expect(screen.getByLabelText('Watched source')).toBeDisabled();
    expect(
      screen.getByText('Every watched source is already linked to this skill. Watch a GitHub repository instead.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Preview' })).toBeDisabled();
  });

  it('links a watched source after showing the preview, then invalidates the skill sources and the source list', async () => {
    const previewSpy = vi.spyOn(queries, 'previewAttachSource').mockResolvedValue(proposal('docs-guide'));
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Linked docs-guide to my-skill.',
    });
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue(undefined);
    renderForm([watched('docs-guide', 'https://github.com/example/docs-guide')]);

    fireEvent.click(screen.getByRole('button', { name: 'Preview' }));

    const dialog = await screen.findByRole('dialog');
    expect(previewSpy).toHaveBeenCalledWith('my-skill', { source_id: 'docs-guide' });
    expect(confirmSpy).not.toHaveBeenCalled();
    expect(within(dialog).getByText('Add source docs-guide?')).toBeInTheDocument();
    expect(within(dialog).getByText('sources/skills/LINK-my-skill-docs-guide.yaml')).toBeInTheDocument();
    expect(within(dialog).getByText('sources/catalog/docs-guide.yaml')).toBeInTheDocument();
    expect(within(dialog).getByText('This repository is large; checks may be slow.')).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole('button', { name: 'Add source' }));

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledWith('PROP-ADD', {
        proposal_digest: pins.proposal_digest,
        base_version: pins.base_version,
      });
    });
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['skill-sources', 'my-skill'] });
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sources'] });
      expect(onAdded).toHaveBeenCalledWith('Linked docs-guide to my-skill.');
    });
  });

  it('watches a GitHub repository with its ref and folder path', async () => {
    let resolvePreview: (p: SourceProposal) => void = () => {};
    const previewSpy = vi.spyOn(queries, 'previewAttachSource').mockImplementation(
      () => new Promise<SourceProposal>((resolve) => {
        resolvePreview = resolve;
      }),
    );
    renderForm([]);

    fireEvent.change(screen.getByLabelText(/GitHub repository URL/), {
      target: { value: '  https://github.com/example/guides  ' },
    });
    fireEvent.change(screen.getByLabelText('Branch or tag'), { target: { value: 'v2' } });
    fireEvent.change(screen.getByLabelText('Folder path'), { target: { value: 'docs/skills' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }));

    expect(await screen.findByRole('button', { name: 'Checking repository…' })).toBeDisabled();
    expect(previewSpy).toHaveBeenCalledWith('my-skill', {
      locator: 'https://github.com/example/guides',
      ref: 'v2',
      path: 'docs/skills',
    });

    resolvePreview(proposal('example-guides'));
    expect(await screen.findByText('Add source example-guides?')).toBeInTheDocument();
  });

  it('shows the summary and no preview when the source is already linked', async () => {
    vi.spyOn(queries, 'previewAttachSource').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'docs-guide is already linked to my-skill.',
      confirmation: { confirmation: { required: false, pins: { proposal_id: '', proposal_digest: '', base_version: '' } } },
    });
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal');
    renderForm([watched('docs-guide', 'https://github.com/example/docs-guide')]);

    fireEvent.click(screen.getByRole('button', { name: 'Preview' }));

    expect(await screen.findByText('docs-guide is already linked to my-skill.')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(confirmSpy).not.toHaveBeenCalled();
    expect(onAdded).not.toHaveBeenCalled();
  });

  it('shows the reason from the API when the preview fails', async () => {
    vi.spyOn(queries, 'previewAttachSource').mockRejectedValue(
      new ApiError(400, {
        code: 'invalid_locator',
        render: { ERROR: 'Cannot watch this address.', WHY: 'Only GitHub repositories can be watched.', FIX: '' },
      }),
    );
    renderForm([]);

    fireEvent.change(screen.getByLabelText(/GitHub repository URL/), { target: { value: 'https://gitlab.com/x/y' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }));

    expect(await screen.findByText('Only GitHub repositories can be watched.')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('cancel sends nothing', () => {
    const previewSpy = vi.spyOn(queries, 'previewAttachSource');
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal');
    renderForm([watched('docs-guide', 'https://github.com/example/docs-guide')]);

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(previewSpy).not.toHaveBeenCalled();
    expect(confirmSpy).not.toHaveBeenCalled();
  });
});
