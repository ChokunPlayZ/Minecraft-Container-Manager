import { useId, useLayoutEffect, useRef, type HTMLAttributes } from 'react';
import { cn } from '../../lib/utils';

const layers: HTMLElement[] = [];
let previousOverflow = '';

interface DialogBackdropProps extends HTMLAttributes<HTMLDivElement> {
  onDismiss: () => void;
}

/** Shared focus, scroll, and dismissal behavior for all protected workflows. */
export function DialogBackdrop({ children, className, onDismiss, ...props }: DialogBackdropProps) {
  const root = useRef<HTMLDivElement>(null);
  const dismiss = useRef(onDismiss);
  const titleId = useId();
  useLayoutEffect(() => { dismiss.current = onDismiss; });

  useLayoutEffect(() => {
    const backdrop = root.current;
    const panel = backdrop?.firstElementChild as HTMLElement | null;
    if (!backdrop || !panel) return;
    const trigger = document.activeElement as HTMLElement | null;
    panel.setAttribute('role', 'dialog');
    panel.setAttribute('aria-modal', 'true');
    panel.tabIndex = -1;
    const heading = panel.querySelector('h2, h3');
    if (!panel.hasAttribute('aria-labelledby') && heading) {
      if (!heading.id) heading.id = titleId;
      panel.setAttribute('aria-labelledby', heading.id);
    }
    if (layers.length === 0) {
      previousOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
    }
    layers.push(backdrop);

    const focusable = () => Array.from(panel.querySelectorAll<HTMLElement>(
      'button:not(:disabled), a[href], input:not(:disabled):not([type="hidden"]), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
    )).filter((element) => !element.closest('[hidden], [inert], [aria-hidden="true"]') &&
      getComputedStyle(element).display !== 'none' && getComputedStyle(element).visibility !== 'hidden');

    const focusInside = () => (focusable()[0] ?? panel).focus({ preventScroll: true });
    if (!panel.contains(document.activeElement)) focusInside();

    const handleKey = (event: KeyboardEvent) => {
      if (layers.at(-1) !== backdrop) return;
      if (event.target instanceof Element && event.target.closest('[role="menu"]')) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopImmediatePropagation();
        dismiss.current();
      } else if (event.key === 'Tab') {
        const controls = focusable();
        const first = controls[0];
        const last = controls.at(-1);
        if (!first || !last) {
          event.preventDefault();
          panel.focus();
        } else if (event.shiftKey && (document.activeElement === first || document.activeElement === panel)) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && (document.activeElement === last || document.activeElement === panel)) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    const keepFocus = (event: FocusEvent) => {
      if (event.target instanceof Element && event.target.closest('[role="menu"]')) return;
      if (layers.at(-1) === backdrop && !backdrop.contains(event.target as Node)) focusInside();
    };
    document.addEventListener('keydown', handleKey, true);
    document.addEventListener('focusin', keepFocus);
    return () => {
      document.removeEventListener('keydown', handleKey, true);
      document.removeEventListener('focusin', keepFocus);
      const wasTop = layers.at(-1) === backdrop;
      const index = layers.indexOf(backdrop);
      if (index >= 0) layers.splice(index, 1);
      if (layers.length === 0) document.body.style.overflow = previousOverflow;
      if (wasTop && trigger?.isConnected) trigger.focus({ preventScroll: true });
    };
  }, [titleId]);

  return <div {...props} ref={root} role="presentation" className={cn(className, 'dialog-backdrop')}>{children}</div>;
}
