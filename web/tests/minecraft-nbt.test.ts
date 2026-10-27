import { describe, it, expect } from 'vitest';
import {
  isModernVersion,
  buildGiveCommand,
  validateNbt,
  getPresetsForItem,
  getPresetNbt,
  COMMON_NBT_PRESETS,
} from '../src/lib/minecraft-nbt';
import { searchMinecraftItems } from '../src/lib/minecraft-items';

describe('Minecraft Items Dataset & Search', () => {
  it('searches items by name and ID', () => {
    const swordResults = searchMinecraftItems('sword');
    expect(swordResults.length).toBeGreaterThan(0);
    expect(swordResults.some((i) => i.id === 'minecraft:diamond_sword')).toBe(true);

    const diamondResults = searchMinecraftItems('diamond');
    expect(diamondResults.some((i) => i.id === 'minecraft:diamond')).toBe(true);
    expect(diamondResults.some((i) => i.id === 'minecraft:diamond_sword')).toBe(true);
  });

  it('filters by category', () => {
    const combatItems = searchMinecraftItems('', 'combat');
    expect(combatItems.every((i) => i.category === 'combat')).toBe(true);
    expect(combatItems.some((i) => i.id === 'minecraft:diamond_sword')).toBe(true);

    const toolsItems = searchMinecraftItems('', 'tools');
    expect(toolsItems.every((i) => i.category === 'tools')).toBe(true);
    expect(toolsItems.some((i) => i.id === 'minecraft:diamond_pickaxe')).toBe(true);
  });
});

describe('Minecraft NBT Utilities', () => {
  it('detects modern Minecraft versions (>= 1.20.5)', () => {
    expect(isModernVersion('1.21.4')).toBe(true);
    expect(isModernVersion('1.21')).toBe(true);
    expect(isModernVersion('1.20.5')).toBe(true);
    expect(isModernVersion('1.20.4')).toBe(false);
    expect(isModernVersion('1.20.1')).toBe(false);
    expect(isModernVersion('1.19.4')).toBe(false);
    expect(isModernVersion(undefined)).toBe(true);
  });

  it('returns appropriate presets for swords and tools', () => {
    const swordPresets = getPresetsForItem('minecraft:diamond_sword');
    expect(swordPresets.some((p) => p.id === 'god-sword')).toBe(true);
    expect(swordPresets.some((p) => p.id === 'unbreakable')).toBe(true);

    const pickPresets = getPresetsForItem('minecraft:diamond_pickaxe');
    expect(pickPresets.some((p) => p.id === 'god-pickaxe-fortune')).toBe(true);
    expect(pickPresets.some((p) => p.id === 'god-pickaxe-silk')).toBe(true);
  });

  const enchantedPresets = [
    'god-sword',
    'smite-sword',
    'god-pickaxe-fortune',
    'god-pickaxe-silk',
    'god-bow',
    'god-crossbow',
    'god-mace',
    'god-armor',
  ];

  it.each(enchantedPresets)('uses flat enchantments for %s on 1.21.5+', (id) => {
    const preset = COMMON_NBT_PRESETS.find((p) => p.id === id)!;
    for (const version of ['1.21.5', '1.21.10', '1.22', '26.1', '1.21.5-pre1', 'latest', undefined]) {
      const nbt = getPresetNbt(preset, 'modern', version);
      const enchantments = nbt.match(/^\[enchantments=\{([^{}]+)\}/)?.[1];
      expect(enchantments).toBeDefined();
      expect(enchantments).not.toContain('levels');
      expect(enchantments?.split(',').every((entry) => /^'minecraft:[a-z_]+':\d+$/.test(entry))).toBe(true);
      expect(validateNbt(nbt).valid).toBe(true);
    }
  });

  it.each(enchantedPresets)('keeps nested enchantments for %s on 1.20.5–1.21.4', (id) => {
    const preset = COMMON_NBT_PRESETS.find((p) => p.id === id)!;
    for (const version of ['1.20.5', '1.20.6', '1.21', '1.21.1', '1.21.4']) {
      const nbt = getPresetNbt(preset, 'modern', version);
      expect(nbt).toMatch(/^\[enchantments=\{levels:\{/);
      expect(validateNbt(nbt).valid).toBe(true);
    }
  });

  it.each(COMMON_NBT_PRESETS)('preserves legacy syntax for $id', (preset) => {
    expect(getPresetNbt(preset, 'legacy', '1.20.1')).toBe(preset.legacy);
    expect(getPresetNbt(preset, 'legacy', '1.21.5')).toBe(preset.legacy);
  });

  it('builds give command correctly with or without NBT', () => {
    expect(buildGiveCommand('Steve', 'minecraft:diamond', undefined, 5)).toBe(
      'give Steve minecraft:diamond 5',
    );
    expect(
      buildGiveCommand(
        'Steve',
        'minecraft:diamond_sword',
        "[enchantments={levels:{'minecraft:sharpness':5}}]",
        1,
      ),
    ).toBe("give Steve minecraft:diamond_sword[enchantments={levels:{'minecraft:sharpness':5}}] 1");
  });

  it('validates NBT syntax safely', () => {
    expect(validateNbt('').valid).toBe(true);
    expect(validateNbt("[enchantments={levels:{'minecraft:sharpness':5}}]").valid).toBe(true);
    expect(validateNbt('{Enchantments:[{id:"sharpness",lvl:5s}]}').valid).toBe(true);

    // Unclosed bracket
    expect(validateNbt('[enchantments=').valid).toBe(false);

    // Unclosed brace
    expect(validateNbt('{Enchantments:[').valid).toBe(false);

    // Newlines
    expect(validateNbt('[enchantments\n]').valid).toBe(false);

    // Not enclosed in [] or {}
    expect(validateNbt('sharpness:5').valid).toBe(false);
  });
});
