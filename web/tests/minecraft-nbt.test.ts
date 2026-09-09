import { describe, it, expect } from 'vitest';
import {
  isModernVersion,
  buildGiveCommand,
  validateNbt,
  getPresetsForItem,
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
