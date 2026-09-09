export type NbtFormat = 'modern' | 'legacy';

export interface NbtPreset {
  id: string;
  label: string;
  description: string;
  modern: string;
  legacy: string;
  applicableTo?: (itemId: string) => boolean;
}

/**
 * Determines whether the given Minecraft version uses Modern Item Components (>= 1.20.5)
 * or Legacy SNBT syntax (< 1.20.5).
 */
export function isModernVersion(version?: string): boolean {
  if (!version) return true;
  // Match e.g. "1.21.4", "1.20.5", "1.20.1", "1.19.4"
  const match = version.match(/^(\d+)\.(\d+)(?:\.(\d+))?/);
  if (!match) return true;
  const major = parseInt(match[1], 10);
  const minor = parseInt(match[2], 10);
  const patch = match[3] ? parseInt(match[3], 10) : 0;

  if (major > 1) return true;
  if (major === 1) {
    if (minor > 20) return true;
    if (minor === 20 && patch >= 5) return true;
    return false;
  }
  return false;
}

export const COMMON_NBT_PRESETS: NbtPreset[] = [
  // Swords
  {
    id: 'god-sword',
    label: '⚡ God Sword (Max Enchants)',
    description: 'Sharpness V, Fire Aspect II, Looting III, Sweeping Edge III, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_sword') || id === 'minecraft:mace',
    modern:
      "[enchantments={levels:{'minecraft:sharpness':5,'minecraft:fire_aspect':2,'minecraft:looting':3,'minecraft:sweeping_edge':3,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:sharpness",lvl:5s},{id:"minecraft:fire_aspect",lvl:2s},{id:"minecraft:looting",lvl:3s},{id:"minecraft:sweeping_edge",lvl:3s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}],Unbreakable:1b}',
  },
  {
    id: 'smite-sword',
    label: '💀 Smite Slayer (Undead)',
    description: 'Smite V, Looting III, Unbreaking III, Mending',
    applicableTo: (id) => id.endsWith('_sword') || id.endsWith('_axe'),
    modern:
      "[enchantments={levels:{'minecraft:smite':5,'minecraft:looting':3,'minecraft:unbreaking':3,'minecraft:mending':1}}]",
    legacy:
      '{Enchantments:[{id:"minecraft:smite",lvl:5s},{id:"minecraft:looting",lvl:3s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}]}',
  },

  // Tools - Pickaxes & Axes
  {
    id: 'god-pickaxe-fortune',
    label: '💎 Fortune Miner (Max)',
    description: 'Efficiency V, Fortune III, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_pickaxe') || id.endsWith('_axe') || id.endsWith('_shovel'),
    modern:
      "[enchantments={levels:{'minecraft:efficiency':5,'minecraft:fortune':3,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:efficiency",lvl:5s},{id:"minecraft:fortune",lvl:3s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}],Unbreakable:1b}',
  },
  {
    id: 'god-pickaxe-silk',
    label: '✨ Silk Touch Master',
    description: 'Efficiency V, Silk Touch, Unbreaking III, Mending, Unbreakable',
    applicableTo: (id) => id.endsWith('_pickaxe') || id.endsWith('_axe') || id.endsWith('_shovel'),
    modern:
      "[enchantments={levels:{'minecraft:efficiency':5,'minecraft:silk_touch':1,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:efficiency",lvl:5s},{id:"minecraft:silk_touch",lvl:1s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}],Unbreakable:1b}',
  },

  // Ranged Weapons
  {
    id: 'god-bow',
    label: '🏹 God Bow (Power V)',
    description: 'Power V, Flame, Infinity, Punch II, Unbreaking III, Unbreakable',
    applicableTo: (id) => id === 'minecraft:bow',
    modern:
      "[enchantments={levels:{'minecraft:power':5,'minecraft:flame':1,'minecraft:infinity':1,'minecraft:punch':2,'minecraft:unbreaking':3}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:power",lvl:5s},{id:"minecraft:flame",lvl:1s},{id:"minecraft:infinity",lvl:1s},{id:"minecraft:punch",lvl:2s},{id:"minecraft:unbreaking",lvl:3s}],Unbreakable:1b}',
  },
  {
    id: 'god-crossbow',
    label: '🎯 Rapid Multi-Crossbow',
    description: 'Quick Charge III, Multishot, Piercing IV, Unbreaking III, Mending',
    applicableTo: (id) => id === 'minecraft:crossbow',
    modern:
      "[enchantments={levels:{'minecraft:quick_charge':3,'minecraft:multishot':1,'minecraft:piercing':4,'minecraft:unbreaking':3,'minecraft:mending':1}}]",
    legacy:
      '{Enchantments:[{id:"minecraft:quick_charge",lvl:3s},{id:"minecraft:multishot",lvl:1s},{id:"minecraft:piercing",lvl:4s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}]}',
  },

  // Mace (1.21+)
  {
    id: 'god-mace',
    label: '🔨 Smash Mace (1.21+)',
    description: 'Density V, Breach IV, Wind Burst III, Unbreaking III, Mending',
    applicableTo: (id) => id === 'minecraft:mace',
    modern:
      "[enchantments={levels:{'minecraft:density':5,'minecraft:breach':4,'minecraft:wind_burst':3,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:density",lvl:5s},{id:"minecraft:breach",lvl:4s},{id:"minecraft:wind_burst",lvl:3s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}],Unbreakable:1b}',
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
    modern:
      "[enchantments={levels:{'minecraft:protection':4,'minecraft:unbreaking':3,'minecraft:mending':1,'minecraft:thorns':3}},unbreakable={}]",
    legacy:
      '{Enchantments:[{id:"minecraft:protection",lvl:4s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s},{id:"minecraft:thorns",lvl:3s}],Unbreakable:1b}',
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
