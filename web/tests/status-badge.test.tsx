import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from '../src/components/status-badge';

describe('StatusBadge', () => {
  it('renders the server state as text', () => {
    render(<StatusBadge state="running" />);
    expect(screen.getByText('running')).toBeInTheDocument();
  });

  it('applies a running-specific style class', () => {
    const { container } = render(<StatusBadge state="running" />);
    expect(container.firstChild).toHaveClass('border-emerald-300');
  });

  it('renders the error state', () => {
    render(<StatusBadge state="error" />);
    expect(screen.getByText('error')).toBeInTheDocument();
    expect(screen.getByText('error')).toHaveClass('border-red-300');
  });
});
