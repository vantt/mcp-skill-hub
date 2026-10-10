import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { AddSkillScreen } from '../skill-add/AddSkillScreen';
import { CreateSkillScreen } from './CreateSkillScreen';

function wrap(node: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('CreateSkillScreen', () => {
  it('says a draft is not routed yet and explains the routing fields with an example', () => {
    wrap(<CreateSkillScreen />);
    expect(screen.getByText(/A draft is not routed to agents yet/)).toBeInTheDocument();
    expect(screen.getByText(/What this skill should not be used for/)).toBeInTheDocument();
    expect(screen.getByText(/How big a job has to be/)).toBeInTheDocument();
    expect(document.getElementById('create-notfor')).toHaveAttribute('placeholder', 'one-off typo fixes, writing new features');
    expect(document.getElementById('create-scope')).toBeInTheDocument();
  });
});

describe('AddSkillScreen', () => {
  it('starts empty with a placeholder address, not a value that looks real', () => {
    wrap(<AddSkillScreen />);
    const input = document.getElementById('gh-url') as HTMLInputElement;
    expect(input.value).toBe('');
    expect(input.placeholder).toBe('https://github.com/OWNER/REPO/tree/main/skills');
    expect(screen.getByRole('button', { name: 'Discover skills' })).toBeDisabled();
  });

  it('says what Discover does and that nothing is written yet', () => {
    wrap(<AddSkillScreen />);
    expect(screen.getByText(/Discover only looks/)).toBeInTheDocument();
    expect(screen.getByText(/Nothing is written to your hub until you confirm/)).toBeInTheDocument();
    expect(screen.getByText(/Choose where new skills are filed/)).toBeInTheDocument();
  });
});
