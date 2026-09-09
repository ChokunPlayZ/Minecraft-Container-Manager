export type ItemCategory =
  | 'combat'
  | 'tools'
  | 'armor'
  | 'food'
  | 'brewing'
  | 'redstone'
  | 'blocks'
  | 'materials'
  | 'misc';

export interface MinecraftItem {
  id: string; // e.g. "minecraft:diamond_sword"
  name: string; // e.g. "Diamond Sword"
  category: ItemCategory;
  maxStack?: number;
}

export const ITEM_CATEGORIES: { id: ItemCategory; label: string }[] = [
  { id: 'combat', label: 'Combat' },
  { id: 'tools', label: 'Tools' },
  { id: 'armor', label: 'Armor' },
  { id: 'food', label: 'Food' },
  { id: 'brewing', label: 'Brewing' },
  { id: 'redstone', label: 'Redstone' },
  { id: 'blocks', label: 'Blocks' },
  { id: 'materials', label: 'Materials' },
  { id: 'misc', label: 'Miscellaneous' },
];

export const MINECRAFT_ITEMS: MinecraftItem[] = [
  // Combat - Swords
  { id: 'minecraft:netherite_sword', name: 'Netherite Sword', category: 'combat' },
  { id: 'minecraft:diamond_sword', name: 'Diamond Sword', category: 'combat' },
  { id: 'minecraft:iron_sword', name: 'Iron Sword', category: 'combat' },
  { id: 'minecraft:golden_sword', name: 'Golden Sword', category: 'combat' },
  { id: 'minecraft:stone_sword', name: 'Stone Sword', category: 'combat' },
  { id: 'minecraft:wooden_sword', name: 'Wooden Sword', category: 'combat' },

  // Combat - Ranged & Weapons
  { id: 'minecraft:bow', name: 'Bow', category: 'combat', maxStack: 1 },
  { id: 'minecraft:crossbow', name: 'Crossbow', category: 'combat', maxStack: 1 },
  { id: 'minecraft:trident', name: 'Trident', category: 'combat', maxStack: 1 },
  { id: 'minecraft:mace', name: 'Mace', category: 'combat', maxStack: 1 },
  { id: 'minecraft:shield', name: 'Shield', category: 'combat', maxStack: 1 },
  { id: 'minecraft:arrow', name: 'Arrow', category: 'combat' },
  { id: 'minecraft:spectral_arrow', name: 'Spectral Arrow', category: 'combat' },
  { id: 'minecraft:tipped_arrow', name: 'Tipped Arrow', category: 'combat' },
  { id: 'minecraft:totem_of_undying', name: 'Totem of Undying', category: 'combat', maxStack: 1 },
  { id: 'minecraft:wind_charge', name: 'Wind Charge', category: 'combat' },

  // Tools - Pickaxes
  { id: 'minecraft:netherite_pickaxe', name: 'Netherite Pickaxe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:diamond_pickaxe', name: 'Diamond Pickaxe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:iron_pickaxe', name: 'Iron Pickaxe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:golden_pickaxe', name: 'Golden Pickaxe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:stone_pickaxe', name: 'Stone Pickaxe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:wooden_pickaxe', name: 'Wooden Pickaxe', category: 'tools', maxStack: 1 },

  // Tools - Axes
  { id: 'minecraft:netherite_axe', name: 'Netherite Axe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:diamond_axe', name: 'Diamond Axe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:iron_axe', name: 'Iron Axe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:golden_axe', name: 'Golden Axe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:stone_axe', name: 'Stone Axe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:wooden_axe', name: 'Wooden Axe', category: 'tools', maxStack: 1 },

  // Tools - Shovels & Hoes
  { id: 'minecraft:netherite_shovel', name: 'Netherite Shovel', category: 'tools', maxStack: 1 },
  { id: 'minecraft:diamond_shovel', name: 'Diamond Shovel', category: 'tools', maxStack: 1 },
  { id: 'minecraft:iron_shovel', name: 'Iron Shovel', category: 'tools', maxStack: 1 },
  { id: 'minecraft:netherite_hoe', name: 'Netherite Hoe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:diamond_hoe', name: 'Diamond Hoe', category: 'tools', maxStack: 1 },
  { id: 'minecraft:iron_hoe', name: 'Iron Hoe', category: 'tools', maxStack: 1 },

  // Tools - Utility Tools
  { id: 'minecraft:shears', name: 'Shears', category: 'tools', maxStack: 1 },
  { id: 'minecraft:flint_and_steel', name: 'Flint and Steel', category: 'tools', maxStack: 1 },
  { id: 'minecraft:fishing_rod', name: 'Fishing Rod', category: 'tools', maxStack: 1 },
  { id: 'minecraft:compass', name: 'Compass', category: 'tools' },
  { id: 'minecraft:clock', name: 'Clock', category: 'tools' },
  { id: 'minecraft:spyglass', name: 'Spyglass', category: 'tools', maxStack: 1 },
  { id: 'minecraft:lead', name: 'Lead', category: 'tools' },
  { id: 'minecraft:name_tag', name: 'Name Tag', category: 'tools' },

  // Armor - Netherite
  { id: 'minecraft:netherite_helmet', name: 'Netherite Helmet', category: 'armor', maxStack: 1 },
  { id: 'minecraft:netherite_chestplate', name: 'Netherite Chestplate', category: 'armor', maxStack: 1 },
  { id: 'minecraft:netherite_leggings', name: 'Netherite Leggings', category: 'armor', maxStack: 1 },
  { id: 'minecraft:netherite_boots', name: 'Netherite Boots', category: 'armor', maxStack: 1 },

  // Armor - Diamond
  { id: 'minecraft:diamond_helmet', name: 'Diamond Helmet', category: 'armor', maxStack: 1 },
  { id: 'minecraft:diamond_chestplate', name: 'Diamond Chestplate', category: 'armor', maxStack: 1 },
  { id: 'minecraft:diamond_leggings', name: 'Diamond Leggings', category: 'armor', maxStack: 1 },
  { id: 'minecraft:diamond_boots', name: 'Diamond Boots', category: 'armor', maxStack: 1 },

  // Armor - Iron
  { id: 'minecraft:iron_helmet', name: 'Iron Helmet', category: 'armor', maxStack: 1 },
  { id: 'minecraft:iron_chestplate', name: 'Iron Chestplate', category: 'armor', maxStack: 1 },
  { id: 'minecraft:iron_leggings', name: 'Iron Leggings', category: 'armor', maxStack: 1 },
  { id: 'minecraft:iron_boots', name: 'Iron Boots', category: 'armor', maxStack: 1 },

  // Armor - Special
  { id: 'minecraft:elytra', name: 'Elytra', category: 'armor', maxStack: 1 },
  { id: 'minecraft:turtle_helmet', name: 'Turtle Shell', category: 'armor', maxStack: 1 },
  { id: 'minecraft:golden_helmet', name: 'Golden Helmet', category: 'armor', maxStack: 1 },
  { id: 'minecraft:golden_chestplate', name: 'Golden Chestplate', category: 'armor', maxStack: 1 },
  { id: 'minecraft:golden_leggings', name: 'Golden Leggings', category: 'armor', maxStack: 1 },
  { id: 'minecraft:golden_boots', name: 'Golden Boots', category: 'armor', maxStack: 1 },

  // Food
  { id: 'minecraft:enchanted_golden_apple', name: 'Enchanted Golden Apple', category: 'food' },
  { id: 'minecraft:golden_apple', name: 'Golden Apple', category: 'food' },
  { id: 'minecraft:golden_carrot', name: 'Golden Carrot', category: 'food' },
  { id: 'minecraft:cooked_beef', name: 'Cooked Beef (Steak)', category: 'food' },
  { id: 'minecraft:cooked_porkchop', name: 'Cooked Porkchop', category: 'food' },
  { id: 'minecraft:cooked_mutton', name: 'Cooked Mutton', category: 'food' },
  { id: 'minecraft:cooked_chicken', name: 'Cooked Chicken', category: 'food' },
  { id: 'minecraft:cooked_salmon', name: 'Cooked Salmon', category: 'food' },
  { id: 'minecraft:bread', name: 'Bread', category: 'food' },
  { id: 'minecraft:apple', name: 'Apple', category: 'food' },
  { id: 'minecraft:cookie', name: 'Cookie', category: 'food' },
  { id: 'minecraft:sweet_berries', name: 'Sweet Berries', category: 'food' },
  { id: 'minecraft:glow_berries', name: 'Glow Berries', category: 'food' },
  { id: 'minecraft:honey_bottle', name: 'Honey Bottle', category: 'food', maxStack: 16 },

  // Brewing & Potions
  { id: 'minecraft:potion', name: 'Potion', category: 'brewing', maxStack: 1 },
  { id: 'minecraft:splash_potion', name: 'Splash Potion', category: 'brewing', maxStack: 1 },
  { id: 'minecraft:lingering_potion', name: 'Lingering Potion', category: 'brewing', maxStack: 1 },
  { id: 'minecraft:experience_bottle', name: 'Bottle o\' Enchanting', category: 'brewing' },
  { id: 'minecraft:brewing_stand', name: 'Brewing Stand', category: 'brewing' },
  { id: 'minecraft:cauldron', name: 'Cauldron', category: 'brewing' },
  { id: 'minecraft:blaze_rod', name: 'Blaze Rod', category: 'brewing' },
  { id: 'minecraft:blaze_powder', name: 'Blaze Powder', category: 'brewing' },
  { id: 'minecraft:ghast_tear', name: 'Ghast Tear', category: 'brewing' },
  { id: 'minecraft:nether_wart', name: 'Nether Wart', category: 'brewing' },
  { id: 'minecraft:glistering_melon_slice', name: 'Glistering Melon Slice', category: 'brewing' },
  { id: 'minecraft:fermented_spider_eye', name: 'Fermented Spider Eye', category: 'brewing' },
  { id: 'minecraft:phantom_membrane', name: 'Phantom Membrane', category: 'brewing' },
  { id: 'minecraft:dragon_breath', name: 'Dragon\'s Breath', category: 'brewing' },

  // Materials & Minerals
  { id: 'minecraft:netherite_ingot', name: 'Netherite Ingot', category: 'materials' },
  { id: 'minecraft:netherite_scrap', name: 'Netherite Scrap', category: 'materials' },
  { id: 'minecraft:diamond', name: 'Diamond', category: 'materials' },
  { id: 'minecraft:emerald', name: 'Emerald', category: 'materials' },
  { id: 'minecraft:gold_ingot', name: 'Gold Ingot', category: 'materials' },
  { id: 'minecraft:iron_ingot', name: 'Iron Ingot', category: 'materials' },
  { id: 'minecraft:copper_ingot', name: 'Copper Ingot', category: 'materials' },
  { id: 'minecraft:coal', name: 'Coal', category: 'materials' },
  { id: 'minecraft:charcoal', name: 'Charcoal', category: 'materials' },
  { id: 'minecraft:lapis_lazuli', name: 'Lapis Lazuli', category: 'materials' },
  { id: 'minecraft:amethyst_shard', name: 'Amethyst Shard', category: 'materials' },
  { id: 'minecraft:quartz', name: 'Nether Quartz', category: 'materials' },
  { id: 'minecraft:ender_pearl', name: 'Ender Pearl', category: 'materials', maxStack: 16 },
  { id: 'minecraft:eye_of_ender', name: 'Eye of Ender', category: 'materials' },
  { id: 'minecraft:slime_ball', name: 'Slimeball', category: 'materials' },
  { id: 'minecraft:gunpowder', name: 'Gunpowder', category: 'materials' },
  { id: 'minecraft:feather', name: 'Feather', category: 'materials' },
  { id: 'minecraft:leather', name: 'Leather', category: 'materials' },
  { id: 'minecraft:string', name: 'String', category: 'materials' },
  { id: 'minecraft:firework_rocket', name: 'Firework Rocket', category: 'materials' },

  // Redstone
  { id: 'minecraft:redstone', name: 'Redstone Dust', category: 'redstone' },
  { id: 'minecraft:redstone_torch', name: 'Redstone Torch', category: 'redstone' },
  { id: 'minecraft:redstone_block', name: 'Block of Redstone', category: 'redstone' },
  { id: 'minecraft:repeater', name: 'Redstone Repeater', category: 'redstone' },
  { id: 'minecraft:comparator', name: 'Redstone Comparator', category: 'redstone' },
  { id: 'minecraft:target', name: 'Target', category: 'redstone' },
  { id: 'minecraft:lever', name: 'Lever', category: 'redstone' },
  { id: 'minecraft:piston', name: 'Sticky Piston', category: 'redstone' },
  { id: 'minecraft:observer', name: 'Observer', category: 'redstone' },
  { id: 'minecraft:hopper', name: 'Hopper', category: 'redstone' },
  { id: 'minecraft:dispenser', name: 'Dispenser', category: 'redstone' },
  { id: 'minecraft:dropper', name: 'Dropper', category: 'redstone' },
  { id: 'minecraft:tnt', name: 'TNT', category: 'redstone' },
  { id: 'minecraft:daylight_detector', name: 'Daylight Detector', category: 'redstone' },
  { id: 'minecraft:crafter', name: 'Crafter', category: 'redstone' },

  // Blocks
  { id: 'minecraft:grass_block', name: 'Grass Block', category: 'blocks' },
  { id: 'minecraft:stone', name: 'Stone', category: 'blocks' },
  { id: 'minecraft:cobblestone', name: 'Cobblestone', category: 'blocks' },
  { id: 'minecraft:deepslate', name: 'Deepslate', category: 'blocks' },
  { id: 'minecraft:obsidian', name: 'Obsidian', category: 'blocks' },
  { id: 'minecraft:crying_obsidian', name: 'Crying Obsidian', category: 'blocks' },
  { id: 'minecraft:bedrock', name: 'Bedrock', category: 'blocks' },
  { id: 'minecraft:oak_log', name: 'Oak Log', category: 'blocks' },
  { id: 'minecraft:oak_planks', name: 'Oak Planks', category: 'blocks' },
  { id: 'minecraft:glass', name: 'Glass', category: 'blocks' },
  { id: 'minecraft:sponge', name: 'Sponge', category: 'blocks' },
  { id: 'minecraft:wet_sponge', name: 'Wet Sponge', category: 'blocks' },
  { id: 'minecraft:beacon', name: 'Beacon', category: 'blocks' },
  { id: 'minecraft:conduit', name: 'Conduit', category: 'blocks' },
  { id: 'minecraft:shulker_box', name: 'Shulker Box', category: 'blocks', maxStack: 1 },
  { id: 'minecraft:ender_chest', name: 'Ender Chest', category: 'blocks' },
  { id: 'minecraft:chest', name: 'Chest', category: 'blocks' },
  { id: 'minecraft:crafting_table', name: 'Crafting Table', category: 'blocks' },
  { id: 'minecraft:furnace', name: 'Furnace', category: 'blocks' },
  { id: 'minecraft:anvil', name: 'Anvil', category: 'blocks' },
  { id: 'minecraft:enchanting_table', name: 'Enchanting Table', category: 'blocks' },

  // Miscellaneous
  { id: 'minecraft:enchanted_book', name: 'Enchanted Book', category: 'misc', maxStack: 1 },
  { id: 'minecraft:book', name: 'Book', category: 'misc' },
  { id: 'minecraft:writable_book', name: 'Book and Quill', category: 'misc', maxStack: 1 },
  { id: 'minecraft:written_book', name: 'Written Book', category: 'misc', maxStack: 16 },
  { id: 'minecraft:saddle', name: 'Saddle', category: 'misc', maxStack: 1 },
  { id: 'minecraft:water_bucket', name: 'Water Bucket', category: 'misc', maxStack: 1 },
  { id: 'minecraft:lava_bucket', name: 'Lava Bucket', category: 'misc', maxStack: 1 },
  { id: 'minecraft:milk_bucket', name: 'Milk Bucket', category: 'misc', maxStack: 1 },
  { id: 'minecraft:bundle', name: 'Bundle', category: 'misc', maxStack: 1 },
  { id: 'minecraft:trial_key', name: 'Trial Key', category: 'misc' },
  { id: 'minecraft:ominous_trial_key', name: 'Ominous Trial Key', category: 'misc' },
  { id: 'minecraft:dragon_egg', name: 'Dragon Egg', category: 'misc', maxStack: 1 },
  { id: 'minecraft:nether_star', name: 'Nether Star', category: 'misc' },
  { id: 'minecraft:heavy_core', name: 'Heavy Core', category: 'misc' },
];

/**
 * Filter items by search query and optional category.
 * Matches on item name (e.g. "Diamond Sword") and identifier (e.g. "minecraft:diamond_sword").
 */
export function searchMinecraftItems(
  query: string,
  category: ItemCategory | 'all' = 'all',
  limit = 20,
): MinecraftItem[] {
  const clean = query.trim().toLowerCase().replace(/^minecraft:/, '');

  let filtered = MINECRAFT_ITEMS;
  if (category !== 'all') {
    filtered = filtered.filter((i) => i.category === category);
  }

  if (!clean) {
    return filtered.slice(0, limit);
  }

  return filtered
    .filter((item) => {
      const shortId = item.id.replace('minecraft:', '').toLowerCase();
      const name = item.name.toLowerCase();
      return (
        shortId.includes(clean) ||
        name.includes(clean) ||
        item.id.toLowerCase().includes(clean)
      );
    })
    .sort((a, b) => {
      const aShort = a.id.replace('minecraft:', '').toLowerCase();
      const bShort = b.id.replace('minecraft:', '').toLowerCase();
      const aExact = aShort === clean || a.name.toLowerCase() === clean;
      const bExact = bShort === clean || b.name.toLowerCase() === clean;
      if (aExact && !bExact) return -1;
      if (bExact && !aExact) return 1;

      const aStarts = aShort.startsWith(clean) || a.name.toLowerCase().startsWith(clean);
      const bStarts = bShort.startsWith(clean) || b.name.toLowerCase().startsWith(clean);
      if (aStarts && !bStarts) return -1;
      if (bStarts && !aStarts) return 1;

      return 0;
    })
    .slice(0, limit);
}
