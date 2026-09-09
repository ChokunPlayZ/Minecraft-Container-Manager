import { describe, expect, it } from 'vitest';
import {
  deletePropertyFromLines,
  getCustomProperties,
  getPropertiesMap,
  parseProperties,
  PROPERTY_SCHEMAS,
  serializeProperties,
  updatePropertyInLines,
} from '../src/lib/server-properties';

describe('server-properties parser & serializer', () => {
  const sampleRaw = `# Minecraft server properties
# Mon Jan 01 00:00:00 UTC 2026
server-port=25565
motd=A Minecraft Server
pvp=true
difficulty=easy

# Custom plugins
my-custom-mod-setting=enabled
`;

  it('parses properties preserving comments and empty lines', () => {
    const lines = parseProperties(sampleRaw);
    expect(lines.length).toBeGreaterThan(0);

    const comments = lines.filter((l) => l.type === 'comment');
    const properties = lines.filter((l) => l.type === 'property');
    const empties = lines.filter((l) => l.type === 'empty');

    expect(comments.length).toBe(3); // 2 at top + 1 before custom
    expect(properties.length).toBe(5);
    expect(empties.length).toBe(2);
  });

  it('extracts properties map accurately', () => {
    const lines = parseProperties(sampleRaw);
    const map = getPropertiesMap(lines);

    expect(map['server-port']).toBe('25565');
    expect(map['motd']).toBe('A Minecraft Server');
    expect(map['pvp']).toBe('true');
    expect(map['difficulty']).toBe('easy');
    expect(map['my-custom-mod-setting']).toBe('enabled');
  });

  it('updates an existing property in place without shifting surrounding lines', () => {
    const lines = parseProperties(sampleRaw);
    const updated = updatePropertyInLines(lines, 'difficulty', 'hard');
    const serialized = serializeProperties(updated);

    expect(serialized).toContain('difficulty=hard');
    expect(serialized).not.toContain('difficulty=easy');
    expect(serialized).toContain('# Minecraft server properties');
    expect(serialized).toContain('my-custom-mod-setting=enabled');
  });

  it('appends a new property if it does not exist', () => {
    const lines = parseProperties(sampleRaw);
    const updated = updatePropertyInLines(lines, 'allow-flight', 'true');
    const serialized = serializeProperties(updated);

    expect(serialized).toContain('allow-flight=true');
    const map = getPropertiesMap(updated);
    expect(map['allow-flight']).toBe('true');
  });

  it('deletes a property cleanly', () => {
    const lines = parseProperties(sampleRaw);
    const updated = deletePropertyFromLines(lines, 'pvp');
    const map = getPropertiesMap(updated);

    expect(map['pvp']).toBeUndefined();
    const serialized = serializeProperties(updated);
    expect(serialized).not.toContain('pvp=true');
  });

  it('identifies custom/unmapped properties', () => {
    const lines = parseProperties(sampleRaw);
    const custom = getCustomProperties(lines);

    expect(custom.length).toBe(1);
    expect(custom[0].key).toBe('my-custom-mod-setting');
    expect(custom[0].value).toBe('enabled');
  });

  it('has comprehensive schemas with required properties', () => {
    const keys = PROPERTY_SCHEMAS.map((s) => s.key);
    expect(keys).toContain('motd');
    expect(keys).toContain('server-port');
    expect(keys).toContain('max-players');
    expect(keys).toContain('gamemode');
    expect(keys).toContain('difficulty');
    expect(keys).toContain('online-mode');
    expect(keys).toContain('enable-rcon');
  });
});
