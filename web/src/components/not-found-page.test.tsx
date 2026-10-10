import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { NotFoundPage } from './NotFoundPage';

describe('NotFoundPage', () => {
  it('says the page is missing and links to the screens that exist', () => {
    render(
      <MemoryRouter initialEntries={['/inbox']}>
        <NotFoundPage />
      </MemoryRouter>,
    );
    expect(screen.getByText('Page not found.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Skills' })).toHaveAttribute('href', '/skills');
    expect(screen.getByRole('link', { name: 'Sources' })).toHaveAttribute('href', '/sources');
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/');
  });
});
