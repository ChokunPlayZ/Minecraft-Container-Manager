import { modChannelLabel, normalizeModChannel } from '../api/mod-channels';
import { Badge } from './ui/badge';
import { Select } from './ui/select';

export function ModChannelBadge({ channel }: { channel?: string }) {
  const value = normalizeModChannel(channel);
  return (
    <Badge
      variant={value === 'release' ? 'default' : 'outline'}
      className={value === 'beta' ? 'border-amber-500/50 text-amber-700 dark:text-amber-300'
        : value === 'alpha' ? 'border-violet-500/50 text-violet-700 dark:text-violet-300' : undefined}
    >
      {modChannelLabel(value)}
    </Badge>
  );
}

export function ModChannelFilter({
  value,
  channels,
  onChange,
}: {
  value: string;
  channels: string[];
  onChange: (channel: string) => void;
}) {
  const options = [...new Set(['release', 'beta', 'alpha', ...channels, value])].filter((channel) => channel !== 'all');
  return (
    <Select aria-label="Release channel" className="h-8 w-40 text-xs" value={value} onValueChange={onChange}>
      <option value="all">All channels</option>
      {options.map((channel) => <option key={channel} value={channel}>{modChannelLabel(channel)}</option>)}
    </Select>
  );
}
