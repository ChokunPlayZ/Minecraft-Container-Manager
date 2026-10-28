import type { KeyboardEvent } from 'react';
import type { LucideIcon } from 'lucide-react';

interface SettingsTabsProps<T extends string> {
  id: string;
  label: string;
  tabs: readonly { id: T; label: string; icon: LucideIcon }[];
  activeTab: T;
  onTabChange: (tab: T) => void;
}

export function SettingsTabs<T extends string>({ id, label, tabs, activeTab, onTabChange }: SettingsTabsProps<T>) {
  function moveFocus(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next: number;
    switch (event.key) {
      case 'ArrowRight': next = (index + 1) % tabs.length; break;
      case 'ArrowLeft': next = (index - 1 + tabs.length) % tabs.length; break;
      case 'Home': next = 0; break;
      case 'End': next = tabs.length - 1; break;
      default: return;
    }
    event.preventDefault();
    document.getElementById(`${id}-tab-${tabs[next].id}`)?.focus();
    onTabChange(tabs[next].id);
  }

  return (
    <div className="workspace-tabs" role="tablist" aria-label={label}>
      {tabs.map(({ id: tab, label: title, icon: Icon }, index) => (
        <button
          key={tab}
          type="button"
          role="tab"
          id={`${id}-tab-${tab}`}
          aria-controls={`${id}-panel`}
          aria-selected={activeTab === tab}
          tabIndex={activeTab === tab ? 0 : -1}
          onClick={() => onTabChange(tab)}
          onKeyDown={(event) => moveFocus(event, index)}
          className={`inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${activeTab === tab ? 'border-primary text-primary' : 'border-transparent text-muted-foreground hover:text-foreground'}`}
        >
          <Icon className="h-4 w-4" aria-hidden="true" />
          {title}
        </button>
      ))}
    </div>
  );
}
