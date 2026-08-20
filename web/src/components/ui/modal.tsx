import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button } from './button';
import { Input } from './input';

type ModalRequest =
  | {
      kind: 'confirm';
      message: string;
      title: string;
      confirmLabel: string;
      cancelLabel: string;
      destructive: boolean;
    }
  | {
      kind: 'prompt';
      message: string;
      title: string;
      confirmLabel: string;
      cancelLabel: string;
      defaultValue: string;
    };

export function useModal() {
  const [request, setRequest] = useState<ModalRequest | null>(null);
  const [promptValue, setPromptValue] = useState('');
  const resolver = useRef<((value: boolean | string | null) => void) | null>(null);

  useEffect(() => {
    return () => {
      resolver.current?.(null);
      resolver.current = null;
    };
  }, []);

  useEffect(() => {
    if (!request) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') finish(null);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [request]);

  function finish(value: boolean | string | null) {
    resolver.current?.(value);
    resolver.current = null;
    setRequest(null);
  }

  function confirm(
    message: string,
    options: Partial<Pick<Extract<ModalRequest, { kind: 'confirm' }>, 'title' | 'confirmLabel' | 'cancelLabel' | 'destructive'>> = {},
  ): Promise<boolean> {
    if (resolver.current) return Promise.resolve(false);
    return new Promise((resolve) => {
      resolver.current = (value) => resolve(value === true);
      setRequest({
        kind: 'confirm',
        message,
        title: options.title ?? 'Are you sure?',
        confirmLabel: options.confirmLabel ?? 'Confirm',
        cancelLabel: options.cancelLabel ?? 'Cancel',
        destructive: options.destructive ?? false,
      });
    });
  }

  function prompt(
    message: string,
    defaultValue = '',
    options: Partial<Pick<Extract<ModalRequest, { kind: 'prompt' }>, 'title' | 'confirmLabel' | 'cancelLabel'>> = {},
  ): Promise<string | null> {
    if (resolver.current) return Promise.resolve(null);
    return new Promise((resolve) => {
      resolver.current = (value) => resolve(typeof value === 'string' ? value : null);
      setPromptValue(defaultValue);
      setRequest({
        kind: 'prompt',
        message,
        title: options.title ?? 'Enter a value',
        confirmLabel: options.confirmLabel ?? 'Continue',
        cancelLabel: options.cancelLabel ?? 'Cancel',
        defaultValue,
      });
    });
  }

  const dialog: ReactNode = request ? (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) finish(null);
      }}
    >
      <div
        className="w-full max-w-md rounded-lg border bg-card p-6 text-card-foreground shadow-lg"
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        onKeyDown={(event) => {
          if (event.key === 'Escape') finish(null);
        }}
      >
        <h2 id="modal-title" className="text-lg font-semibold">
          {request.title}
        </h2>
        <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{request.message}</p>
        {request.kind === 'prompt' && (
          <Input
            autoFocus
            className="mt-4"
            value={promptValue}
            onChange={(event) => setPromptValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') finish(promptValue);
            }}
          />
        )}
        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {request.cancelLabel}
          </Button>
          <Button
            type="button"
            variant={request.kind === 'confirm' && request.destructive ? 'destructive' : 'default'}
            onClick={() => finish(request.kind === 'prompt' ? promptValue : true)}
          >
            {request.confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  ) : null;

  return { confirm, prompt, dialog };
}
