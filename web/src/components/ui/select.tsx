import * as React from 'react';
import * as SelectPrimitive from '@radix-ui/react-select';
import { Check, ChevronDown, ChevronUp } from 'lucide-react';
import { cn } from '../../lib/utils';

type SelectEntry =
  | { kind: 'option'; value: string; label: React.ReactNode; disabled: boolean }
  | { kind: 'group'; label: React.ReactNode; entries: SelectEntry[] };

// Retain declarative options, including fragments and option groups.
function readOptions(children: React.ReactNode, disabled = false): SelectEntry[] {
  const entries: SelectEntry[] = [];
  React.Children.forEach(children, (child) => {
    if (!React.isValidElement<{ value?: string | number; label?: string; disabled?: boolean; children?: React.ReactNode }>(child)) return;
    if (child.type === React.Fragment) entries.push(...readOptions(child.props.children, disabled));
    else if (child.type === 'optgroup') entries.push({ kind: 'group', label: child.props.label, entries: readOptions(child.props.children, disabled || !!child.props.disabled) });
    else if (child.type === 'option') entries.push({ kind: 'option', value: String(child.props.value ?? child.props.children ?? ''), label: child.props.label ?? child.props.children, disabled: disabled || !!child.props.disabled });
  });
  return entries;
}

interface SelectProps extends Omit<React.ComponentPropsWithoutRef<typeof SelectPrimitive.Trigger>, 'value' | 'defaultValue' | 'onChange' | 'dir'> {
  value?: string | number;
  defaultValue?: string | number;
  onValueChange?: (value: string) => void;
  name?: string;
  required?: boolean;
  dir?: 'ltr' | 'rtl';
  placeholder?: string;
}

const Select = React.forwardRef<HTMLButtonElement, SelectProps>(
  ({ className, children, value, defaultValue, onValueChange, name, required, disabled, dir, placeholder, ...props }, ref) => {
    const entries = readOptions(children);
    const flatten = (items: SelectEntry[]): Extract<SelectEntry, { kind: 'option' }>[] => items.flatMap((item) => item.kind === 'group' ? flatten(item.entries) : [item]);
    const options = flatten(entries);
    const initialValue = defaultValue ?? options.find((option) => !option.disabled)?.value;
    const emptyLabel = options.find((option) => option.value === '')?.label;

    function renderEntries(items: SelectEntry[]): React.ReactNode {
      return items.map((entry, index) => entry.kind === 'group' ? (
        <SelectPrimitive.Group key={`group-${index}`}>
          <SelectPrimitive.Label className="px-3 pb-1.5 pt-3 text-xs font-medium text-muted-foreground">{entry.label}</SelectPrimitive.Label>
          {renderEntries(entry.entries)}
        </SelectPrimitive.Group>
      ) : entry.value === '' ? null : (
        <SelectPrimitive.Item key={entry.value} value={entry.value} disabled={entry.disabled}
          className="relative flex min-h-9 cursor-pointer select-none items-center rounded-md py-2 pl-3 pr-9 text-sm outline-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-45">
          <SelectPrimitive.ItemText className="min-w-0"><span className="break-words">{entry.label}</span></SelectPrimitive.ItemText>
          <SelectPrimitive.ItemIndicator className="absolute right-3 flex items-center"><Check className="h-4 w-4" /></SelectPrimitive.ItemIndicator>
        </SelectPrimitive.Item>
      ));
    }

    return (
      <SelectPrimitive.Root value={value === undefined ? undefined : String(value)} defaultValue={initialValue === undefined ? undefined : String(initialValue)}
        onValueChange={onValueChange} name={name} required={required} disabled={disabled} dir={dir}>
        <SelectPrimitive.Trigger ref={ref} {...props}
          className={cn('flex h-10 w-full min-w-0 items-center justify-between gap-2 rounded-md border border-input bg-card px-3 py-1 text-left text-sm transition-[border-color,box-shadow] duration-150 hover:border-muted-foreground/50 focus-visible:outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-ring/20 disabled:cursor-not-allowed disabled:bg-muted disabled:opacity-45 data-[state=open]:border-primary data-[state=open]:ring-2 data-[state=open]:ring-ring/20 [&>span:first-child]:min-w-0 [&>span:first-child]:truncate', className)}>
          <SelectPrimitive.Value placeholder={placeholder ?? emptyLabel ?? 'Select an option'} />
          <SelectPrimitive.Icon asChild><ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" /></SelectPrimitive.Icon>
        </SelectPrimitive.Trigger>
        <SelectPrimitive.Portal>
          <SelectPrimitive.Content position="popper" sideOffset={4} collisionPadding={12} onEscapeKeyDown={(event) => event.stopPropagation()}
            className="z-[150] max-h-[min(320px,var(--radix-select-content-available-height))] min-w-[var(--radix-select-trigger-width)] max-w-[calc(100vw-24px)] overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-md">
            <SelectPrimitive.ScrollUpButton className="flex h-7 items-center justify-center bg-popover text-muted-foreground"><ChevronUp className="h-4 w-4" /></SelectPrimitive.ScrollUpButton>
            <SelectPrimitive.Viewport className="p-1.5">{renderEntries(entries)}</SelectPrimitive.Viewport>
            <SelectPrimitive.ScrollDownButton className="flex h-7 items-center justify-center bg-popover text-muted-foreground"><ChevronDown className="h-4 w-4" /></SelectPrimitive.ScrollDownButton>
          </SelectPrimitive.Content>
        </SelectPrimitive.Portal>
      </SelectPrimitive.Root>
    );
  },
);
Select.displayName = 'Select';

export { Select };
