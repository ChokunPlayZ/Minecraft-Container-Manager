import { useState } from 'react';
import { User } from 'lucide-react';

export interface PlayerAvatarProps {
  name: string;
  size?: number;
}

export function PlayerAvatar({ name, size = 32 }: PlayerAvatarProps) {
  const [loadError, setLoadError] = useState(false);
  const avatarUrl = `https://mc-heads.net/avatar/${encodeURIComponent(name)}/${size}`;

  if (loadError) {
    return (
      <div
        style={{ width: size, height: size }}
        className="flex shrink-0 items-center justify-center rounded-md bg-secondary text-muted-foreground font-semibold text-xs select-none"
      >
        <User className="h-4 w-4" />
      </div>
    );
  }

  return (
    <img
      src={avatarUrl}
      alt={name}
      width={size}
      height={size}
      loading="lazy"
      onError={() => setLoadError(true)}
      className="shrink-0 rounded-md bg-muted object-contain shadow-2xs"
    />
  );
}
