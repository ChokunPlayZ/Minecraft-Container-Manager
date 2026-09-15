import React from 'react';

export interface ProgressBarProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number | null;
  max?: number;
  label?: React.ReactNode;
  subtext?: React.ReactNode;
  showPercent?: boolean;
  variant?: 'default' | 'success' | 'warning' | 'destructive' | 'emerald' | 'sky';
  size?: 'xs' | 'sm' | 'md' | 'lg';
  animated?: boolean;
}

export function ProgressBar({
  value,
  max = 100,
  label,
  subtext,
  showPercent = true,
  variant = 'default',
  size = 'md',
  animated = true,
  className = '',
  ...props
}: ProgressBarProps) {
  const isIndeterminate = value === undefined || value === null;
  const clampedValue = !isIndeterminate ? Math.max(0, Math.min(max, value)) : 0;
  const percentage = !isIndeterminate ? Math.round((clampedValue / max) * 100) : 0;

  const sizeClasses = {
    xs: 'h-1.5',
    sm: 'h-2',
    md: 'h-2.5',
    lg: 'h-3.5',
  }[size];

  const variantBarClasses = {
    default: 'bg-primary text-primary',
    emerald: 'bg-emerald-500 text-emerald-500 dark:bg-emerald-400 dark:text-emerald-400',
    success: 'bg-emerald-500 text-emerald-500 dark:bg-emerald-400 dark:text-emerald-400',
    warning: 'bg-amber-500 text-amber-500 dark:bg-amber-400 dark:text-amber-400',
    destructive: 'bg-destructive text-destructive',
    sky: 'bg-sky-500 text-sky-500 dark:bg-sky-400 dark:text-sky-400',
  }[variant];

  return (
    <div className={`w-full space-y-1.5 ${className}`} {...props}>
      {(label || subtext || (!isIndeterminate && showPercent)) && (
        <div className="flex items-center justify-between gap-2 text-xs">
          <div className="flex items-center gap-1.5 min-w-0 font-medium text-foreground truncate">
            {label}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {subtext && <span className="text-muted-foreground text-[11px]">{subtext}</span>}
            {!isIndeterminate && showPercent && (
              <span className="font-semibold tabular-nums text-foreground">{percentage}%</span>
            )}
          </div>
        </div>
      )}

      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={!isIndeterminate ? clampedValue : undefined}
        className={`w-full overflow-hidden rounded-full bg-secondary/80 border border-border/40 ${sizeClasses}`}
      >
        {isIndeterminate ? (
          <div
            className={`h-full w-full origin-left rounded-full ${variantBarClasses} animate-pulse`}
            style={{
              backgroundImage:
                'linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.2) 50%, transparent 100%)',
              backgroundSize: '200% 100%',
            }}
          />
        ) : (
          <div
            className={`h-full rounded-full transition-all duration-300 ease-out ${variantBarClasses} ${
              animated ? 'shadow-xs' : ''
            }`}
            style={{ width: `${percentage}%` }}
          />
        )}
      </div>
    </div>
  );
}
