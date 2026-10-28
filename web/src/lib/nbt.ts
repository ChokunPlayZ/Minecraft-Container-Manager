import type { NBTTag } from '../api/nbt';

export const TAG_TYPES = [
  '',
  'Byte',
  'Short',
  'Int',
  'Long',
  'Float',
  'Double',
  'Byte array',
  'String',
  'List',
  'Compound',
  'Int array',
  'Long array',
];
export const isNBTFile = (name: string) =>
  /\.(dat|dat_old|nbt|schematic|schem|litematic)(\.(gz|zlib))?$/i.test(name);
export const isContainer = (tag: NBTTag) => tag.type === 9 || tag.type === 10;
export const isArray = (tag: NBTTag) => [7, 11, 12].includes(tag.type);
export const tagAt = (root: NBTTag, path: number[]): NBTTag =>
  path.reduce((tag, index) => tag.children![index], root);
export function updateTag(root: NBTTag, path: number[], update: (tag: NBTTag) => NBTTag): NBTTag {
  if (!path.length) return update(root);
  const [index, ...rest] = path;
  return {
    ...root,
    children: root.children?.map((child, i) => (i === index ? updateTag(child, rest, update) : child)),
  };
}
export function newTag(type: number, name = ''): NBTTag {
  return {
    type,
    name,
    ...(isContainer({ type, name })
      ? { children: [], ...(type === 9 ? { list_type: 0 } : {}) }
      : isArray({ type, name })
        ? { values: [] }
        : { value: type === 8 ? '' : '0' }),
  };
}
export function tagSummary(tag: NBTTag): string {
  if (isContainer(tag)) return `${tag.children?.length ?? 0} tags`;
  if (isArray(tag)) return `${tag.values?.length ?? 0} values`;
  return tag.value?.slice(0, 100) ?? '';
}
export function valueError(type: number, value: string): string | null {
  const bits = ({ 1: 8, 2: 16, 3: 32, 4: 64, 7: 8, 11: 32, 12: 64 } as Record<number, number>)[type];
  if (bits) {
    if (!/^[+-]?\d+$/.test(value)) return 'Enter a whole number.';
    const n = BigInt(value),
      limit = 1n << BigInt(bits - 1);
    if (n < -limit || n >= limit)
      return `Enter a signed ${bits}-bit integer (${String(-limit)} to ${String(limit - 1n)}).`;
  } else if (type === 5 || type === 6) {
    if (/^(NaN(:0x[\da-f]+)?|[+-]?Inf(inity)?)$/i.test(value)) return null;
    const n = Number(value);
    if (
      !value.trim() ||
      !/^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$/.test(value) ||
      !Number.isFinite(n) ||
      (type === 5 && !Number.isFinite(Math.fround(n)))
    )
      return `Enter a valid ${TAG_TYPES[type].toLowerCase()}.`;
  } else if (type === 8 && new TextEncoder().encode(value).length > 65535) return 'This string is too long.';
  return null;
}
export function documentError(tag: NBTTag): string | null {
  if (isContainer(tag)) {
    if (tag.type === 10) {
      const names = new Set<string>();
      for (const child of tag.children ?? []) {
        if (names.has(child.name))
          return `Duplicate tag name: ${child.name || '(unnamed)'}. Choose a unique name.`;
        names.add(child.name);
      }
    }
    for (const child of tag.children ?? []) {
      const error = documentError(child);
      if (error) return error;
    }
    return null;
  }
  if (isArray(tag)) {
    for (const value of tag.values ?? []) {
      const error = valueError(tag.type, value);
      if (error) return `${tag.name || TAG_TYPES[tag.type]}: ${error}`;
    }
    return null;
  }
  const error = valueError(tag.type, tag.value ?? '');
  return error ? `${tag.name || TAG_TYPES[tag.type]}: ${error}` : null;
}
export function playerRootPath(root: NBTTag): number[] | null {
  if (root.type !== 10) return null;
  if (root.children?.some((t) => t.name === 'Health' || t.name === 'foodLevel' || t.name === 'Inventory'))
    return [];
  const data = root.children?.findIndex((t) => t.name === 'Data' && t.type === 10) ?? -1;
  const player =
    data >= 0
      ? (root.children![data].children?.findIndex((t) => t.name === 'Player' && t.type === 10) ?? -1)
      : -1;
  return player >= 0 ? [data, player] : null;
}
