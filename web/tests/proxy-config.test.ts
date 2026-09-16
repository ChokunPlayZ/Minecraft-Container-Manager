import { describe, expect, it } from 'vitest';
import {
  DEFAULT_BUNGEE_YAML,
  DEFAULT_VELOCITY_TOML,
  extractContainerId,
  generateForwardingSecret,
  parseBungeeYaml,
  parseVelocityToml,
  serializeBungeeYaml,
  serializeVelocityToml,
} from '../src/lib/proxy-config';

describe('proxy-config', () => {
  it('extractContainerId extracts id correctly', () => {
    expect(extractContainerId('mcm-550e8400-e29b-41d4-a716-446655440000:25565')).toBe(
      '550e8400-e29b-41d4-a716-446655440000'
    );
    expect(extractContainerId('mcm-abc-123')).toBe('abc-123');
    expect(extractContainerId('192.168.1.50:25565')).toBeNull();
  });

  it('generateForwardingSecret returns random hex string', () => {
    const s1 = generateForwardingSecret();
    const s2 = generateForwardingSecret();
    expect(s1.length).toBeGreaterThan(10);
    expect(s1).not.toBe(s2);
  });

  it('parses default velocity.toml and serializes back', () => {
    const parsed = parseVelocityToml(DEFAULT_VELOCITY_TOML);
    expect(parsed.bind).toBe('0.0.0.0:25577');
    expect(parsed.playerInfoForwardingMode).toBe('modern');
    expect(parsed.onlineMode).toBe(true);

    // Add a backend server
    parsed.servers.push({
      name: 'lobby',
      address: 'mcm-550e8400-e29b-41d4-a716-446655440000:25565',
    });
    parsed.tryServers.push('lobby');

    const serialized = serializeVelocityToml(parsed);
    expect(serialized).toContain('lobby = "mcm-550e8400-e29b-41d4-a716-446655440000:25565"');
    expect(serialized).toContain('"lobby"');

    const reparsed = parseVelocityToml(serialized);
    expect(reparsed.servers).toHaveLength(1);
    expect(reparsed.servers[0].name).toBe('lobby');
    expect(reparsed.servers[0].address).toBe('mcm-550e8400-e29b-41d4-a716-446655440000:25565');
    expect(reparsed.tryServers).toContain('lobby');
  });

  it('parses default config.yml for bungeecord and serializes back', () => {
    const parsed = parseBungeeYaml(DEFAULT_BUNGEE_YAML);
    expect(parsed.bind).toBe('0.0.0.0:25577');
    expect(parsed.onlineMode).toBe(true);

    parsed.servers = [
      {
        name: 'survival',
        address: 'mcm-survival-id:25565',
        motd: 'Survival World',
        restricted: false,
      },
    ];
    parsed.tryServers = ['survival'];

    const serialized = serializeBungeeYaml(parsed);
    expect(serialized).toContain('survival:');
    expect(serialized).toContain('address: mcm-survival-id:25565');
    expect(serialized).toContain('- survival');

    const reparsed = parseBungeeYaml(serialized);
    expect(reparsed.servers).toHaveLength(1);
    expect(reparsed.servers[0].name).toBe('survival');
    expect(reparsed.servers[0].address).toBe('mcm-survival-id:25565');
    expect(reparsed.tryServers).toContain('survival');
  });
});
