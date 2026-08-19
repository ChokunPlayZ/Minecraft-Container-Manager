import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ThemeProvider, useTheme } from '../src/components/theme-provider';
import { Button } from '../src/components/ui/button';

function Probe() {
  const { theme, toggle } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <Button onClick={toggle}>Toggle</Button>
    </div>
  );
}

describe('ThemeProvider', () => {
  it('provides a default theme context', async () => {
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    expect(screen.getByTestId('theme')).toBeInTheDocument();
  });

  it('toggles between light and dark', async () => {
    window.localStorage.setItem('mcm-theme', 'light');
    const user = userEvent.setup();
    render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    );
    const theme = screen.getByTestId('theme');
    expect(theme.textContent).toBe('light');

    await user.click(screen.getByRole('button', { name: 'Toggle' }));

    await waitFor(() => expect(theme.textContent).toBe('dark'));
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});
