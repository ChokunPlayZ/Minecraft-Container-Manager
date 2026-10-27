export type NbtFormat = 'modern' | 'legacy';

export interface NbtPreset {
  id: string;
  label: string;
  description: string;
  modern: string;
  modernBefore1215?: string;
  legacy: string;
  applicableTo?: (itemId: string) => boolean;
}

function isVersionAtLeast(version: string | undefined, major: number, minor: number, patch = 0): boolean {
  if (!version) return true;
  // Unknown versions (including "latest") default to the current component format.
  const match = version.match(/^(\d+)\.(\d+)(?:\.(\d+))?/);
  if (!match) return true;
  const actual = [Number(match[1]), Number(match[2]), Number(match[3] || 0)];
  const required = [major, minor, patch];
  for (let i = 0; i < required.length; i++) {
    if (actual[i] !== required[i]) return actual[i] > required[i];
  }
  return true;
}

/** Whether a Minecraft version uses item components instead of legacy SNBT. */
export function isModernVersion(version?: string): boolean {
  return isVersionAtLeast(version, 1, 20, 5);
}

/** Minecraft 1.21.5 removed the enchantment component's nested levels field. */
function enchantmentNbt(
  enchantments: Record<string, number>,
  unbreakable = false,
): Pick<NbtPreset, 'modern' | 'modernBefore1215' | 'legacy'> {
  const entries = Object.entries(enchantments);
  const levels = entries.map(([id, level]) => `'minecraft:${id}':${level}`).join(',');
  const durability = unbreakable ? ',unbreakable={}' : '';
  const legacyLevels = entries.map(([id, level]) => `{id:"minecraft:${id}",lvl:${level}s}`).join(',');
  return {
    modern: `[enchantments={${levels}}${durability}]`,
    modernBefore1215: `[enchantments={levels:{${levels}}}${durability}]`,
    legacy: `{Enchantments:[${legacyLevels}]${unbreakable ? ',Unbreakable:1b' : ''}}`,
  };
}

export function getPresetNbt(
  preset: Pick<NbtPreset, 'modern' | 'modernBefore1215' | 'legacy'>,
  format: NbtFormat,
  version?: string,
): string {
  if (format === 'legacy') return preset.legacy;
  return isVersionAtLeast(version, 1, 21, 5)
    ? preset.modern
    : (preset.modernBefore1215 ?? preset.modern);
}

export const COMMON_NBT_PRESETS: NbtPreset[] = [
  // Swords
  {
    id: 'god-sword',
    label: '⚡ God Sword (Max Enchants)',
    description: 'Sharpness V, Fire Aspect II, Looting III, Sweeping Edge III, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_sword') || id === 'minecraft:mace',
    ...enchantmentNbt({ sharpness: 5, fire_aspect: 2, looting: 3, sweeping_edge: 3, unbreaking: 3, mending: 1 }, true),
  },
  {
    id: 'smite-sword',
    label: '💀 Smite Slayer (Undead)',
    description: 'Smite V, Looting III, Unbreaking III, Mending',
    applicableTo: (id) => id.endsWith('_sword') || id.endsWith('_axe'),
    ...enchantmentNbt({ smite: 5, looting: 3, unbreaking: 3, mending: 1 }),
  },

  // Tools - Pickaxes & Axes
  {
    id: 'god-pickaxe-fortune',
    label: '💎 Fortune Miner (Max)',
    description: 'Efficiency V, Fortune III, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_pickaxe') || id.endsWith('_axe') || id.endsWith('_shovel'),
    ...enchantmentNbt({ efficiency: 5, fortune: 3, unbreaking: 3, mending: 1 }, true),
  },
  {
    id: 'god-pickaxe-silk',
    label: '✨ Silk Touch Master',
    description: 'Efficiency V, Silk Touch, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_pickaxe') || id.endsWith('_axe') || id.endsWith('_shovel'),
    ...enchantmentNbt({ efficiency: 5, silk_touch: 1, unbreaking: 3, mending: 1 }, true),
  },

  // Ranged Weapons
  {
    id: 'god-bow',
    label: '🏹 God Bow (Power V)',
    description: 'Power V, Flame, Infinity, Punch II, Unbreaking III, Unbreakable',
    applicableTo: (id) => id === 'minecraft:bow',
    ...enchantmentNbt({ power: 5, flame: 1, infinity: 1, punch: 2, unbreaking: 3 }, true),
  },
  {
    id: 'god-crossbow',
    label: '🎯 Rapid Multi-Crossbow',
    description: 'Quick Charge III, Multishot, Piercing IV, Unbreaking III, Mending',
    applicableTo: (id) => id === 'minecraft:crossbow',
    ...enchantmentNbt({ quick_charge: 3, multishot: 1, piercing: 4, unbreaking: 3, mending: 1 }),
  },

  // Mace (1.21+)
  {
    id: 'god-mace',
    label: '🔨 Smash Mace (1.21+)',
    description: 'Density V, Breach IV, Wind Burst III, Unbreaking III, Mending',
    applicableTo: (id) => id === 'minecraft:mace',
    ...enchantmentNbt({ density: 5, breach: 4, wind_burst: 3, unbreaking: 3, mending: 1 }, true),
  },

  // Armor
  {
    id: 'god-armor',
    label: '🛡️ God Armor (Protection IV)',
    description: 'Protection IV, Unbreaking III, Mending, Thorns III, Unbreakable',
    applicableTo: (id) =>
      id.endsWith('_helmet') ||
      id.endsWith('_chestplate') ||
      id.endsWith('_leggings') ||
      id.endsWith('_boots'),
    ...enchantmentNbt({ protection: 4, unbreaking: 3, mending: 1, thorns: 3 }, true),
  },

  // Universal Presets
  {
    id: 'unbreakable',
    label: '♾️ Unbreakable',
    description: 'Never takes durability damage',
    modern: '[unbreakable={}]',
    legacy: '{Unbreakable:1b}',
  },
  {
    id: 'custom-name',
    label: '🏷️ Named: Excalibur',
    description: 'Sets custom golden bold display name',
    modern: "[custom_name='{\"text\":\"Excalibur\",\"color\":\"gold\",\"bold\":true}']",
    legacy: '{display:{Name:\'{"text":"Excalibur","color":"gold","bold":true}\'}}',
  },
  {
    id: 'glint',
    label: '✨ Enchantment Glint',
    description: 'Makes any item shine with enchantment glow',
    modern: '[enchantment_glint_override=true]',
    legacy: '{Enchantments:[{}]}',
  },
];

/**
 * Returns available presets for an item, sorted with item-specific presets first.
 */
export function getPresetsForItem(itemId: string): NbtPreset[] {
  const cleanId = itemId.trim().toLowerCase();
  const specific: NbtPreset[] = [];
  const universal: NbtPreset[] = [];

  for (const preset of COMMON_NBT_PRESETS) {
    if (preset.applicableTo) {
      if (preset.applicableTo(cleanId)) {
        specific.push(preset);
      }
    } else {
      universal.push(preset);
    }
  }

  return [...specific, ...universal];
}

/**
 * Builds the full Minecraft command line for display or execution.
 */
export function buildGiveCommand(
  playerName: string,
  itemId: string,
  nbt: string | undefined,
  amount = 1,
): string {
  const safePlayer = playerName.trim() || 'Steve';
  const cleanItem = itemId.trim() || 'minecraft:diamond';
  const cleanNbt = (nbt || '').trim();
  const count = amount > 0 ? amount : 1;

  if (!cleanNbt) {
    return `give ${safePlayer} ${cleanItem} ${count}`;
  }

  // If item already ends with [ or { or if nbt starts with [ or {
  if (cleanItem.includes('[') || cleanItem.includes('{')) {
    return `give ${safePlayer} ${cleanItem} ${count}`;
  }

  return `give ${safePlayer} ${cleanItem}${cleanNbt} ${count}`;
}

/**
 * Validates raw NBT or component syntax.
 */
export function validateNbt(nbt: string): { valid: boolean; error?: string } {
  const trimmed = nbt.trim();
  if (!trimmed) return { valid: true };

  // Cannot contain newlines or carriage returns
  if (/[\r\n]/.test(trimmed)) {
    return { valid: false, error: 'NBT cannot contain newlines' };
  }

  // Must start and end with balanced delimiters
  const isBracket = trimmed.startsWith('[') && trimmed.endsWith(']');
  const isBrace = trimmed.startsWith('{') && trimmed.endsWith('}');

  if (!isBracket && !isBrace) {
    return {
      valid: false,
      error: 'Must be enclosed in [] (Modern Components) or {} (Legacy SNBT)',
    };
  }

  // Check balanced brackets and braces
  let square = 0;
  let curly = 0;
  let inString: string | null = null;
  let escape = false;

  for (let i = 0; i < trimmed.length; i++) {
    const ch = trimmed[i];

    if (escape) {
      escape = false;
      continue;
    }

    if (ch === '\\') {
      escape = true;
      continue;
    }

    if (inString) {
      if (ch === inString) {
        inString = null;
      }
      continue;
    }

    if (ch === '"' || ch === "'") {
      inString = ch;
      continue;
    }

    if (ch === '[') square++;
    else if (ch === ']') square--;
    else if (ch === '{') curly++;
    else if (ch === '}') curly--;

    if (square < 0 || curly < 0) {
      return { valid: false, error: 'Mismatched brackets or braces' };
    }
  }

  if (square !== 0) return { valid: false, error: 'Unclosed square bracket [' };
  if (curly !== 0) return { valid: false, error: 'Unclosed curly brace {' };
  if (inString) return { valid: false, error: `Unclosed quote ${inString}` };

  return { valid: true };
}
