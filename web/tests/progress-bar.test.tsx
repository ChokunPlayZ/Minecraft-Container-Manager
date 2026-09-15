import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProgressBar } from '../src/components/ui/progress';

describe('ProgressBar', () => {
  it('renders determinate progress with percentage and label', () => {
    render(<ProgressBar value={45} max={100} label="Compressing files..." />);
    expect(screen.getByText('Compressing files...')).toBeInTheDocument();
    expect(screen.getByText('45%')).toBeInTheDocument();
    const bar = screen.getByRole('progressbar');
    expect(bar).toBeInTheDocument();
    expect(bar).toHaveAttribute('aria-valuenow', '45');
    expect(bar).toHaveAttribute('aria-valuemin', '0');
    expect(bar).toHaveAttribute('aria-valuemax', '100');
  });

  it('renders indeterminate state when value is undefined or null', () => {
    render(<ProgressBar value={undefined} label="Downloading jar..." />);
    expect(screen.getByText('Downloading jar...')).toBeInTheDocument();
    const bar = screen.getByRole('progressbar');
    expect(bar).not.toHaveAttribute('aria-valuenow');
    // Percentage should not be rendered for indeterminate
    expect(screen.queryByText('%')).not.toBeInTheDocument();
  });

  it('clamps value between 0 and max', () => {
    const { rerender } = render(<ProgressBar value={150} max={100} />);
    let bar = screen.getByRole('progressbar');
    expect(bar).toHaveAttribute('aria-valuenow', '100');
    expect(screen.getByText('100%')).toBeInTheDocument();

    rerender(<ProgressBar value={-20} max={100} />);
    bar = screen.getByRole('progressbar');
    expect(bar).toHaveAttribute('aria-valuenow', '0');
    expect(screen.getByText('0%')).toBeInTheDocument();
  });

  it('renders subtext when provided', () => {
    render(
      <ProgressBar
        value={60}
        label="Uploading modpack"
        subtext="12.5 MB / 20.0 MB"
      />
    );
    expect(screen.getByText('Uploading modpack')).toBeInTheDocument();
    expect(screen.getByText('12.5 MB / 20.0 MB')).toBeInTheDocument();
    expect(screen.getByText('60%')).toBeInTheDocument();
  });

  it('hides percentage text when showPercent is false', () => {
    render(<ProgressBar value={75} showPercent={false} label="Processing" />);
    expect(screen.getByText('Processing')).toBeInTheDocument();
    expect(screen.queryByText('75%')).not.toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '75');
  });

  it('applies different variants and sizes', () => {
    const { container, rerender } = render(
      <ProgressBar value={50} variant="emerald" size="lg" />
    );
    let bar = container.querySelector('[role="progressbar"]');
    expect(bar).toHaveClass('h-3.5');

    rerender(<ProgressBar value={50} variant="sky" size="xs" />);
    bar = container.querySelector('[role="progressbar"]');
    expect(bar).toHaveClass('h-1.5');
  });
});
