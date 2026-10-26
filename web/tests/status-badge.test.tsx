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
    expect(container.firstChild).toHaveClass('text-emerald-800');
  });

  it('renders the error state', () => {
    render(<StatusBadge state="error" />);
    expect(screen.getByText('error')).toBeInTheDocument();
    expect(screen.getByText('error')).toHaveClass('text-destructive');
  });

  it('renders the installing state', () => {
    render(<StatusBadge state="installing" />);
    expect(screen.getByText('installing')).toBeInTheDocument();
    expect(screen.getByText('installing')).toHaveClass('text-sky-800');
  });

  it('renders the building state', () => {
    render(<StatusBadge state="building" />);
    expect(screen.getByText('building')).toBeInTheDocument();
    expect(screen.getByText('building')).toHaveClass('text-sky-800');
  });
});
