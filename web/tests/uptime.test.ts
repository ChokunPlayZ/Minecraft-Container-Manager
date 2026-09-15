import { describe, expect, it } from 'vitest';
import {
  formatUptime,
  formatUptimeFromStartedAt,
  getUptimeSeconds,
} from '../src/lib/uptime';

describe('Uptime utilities', () => {
  describe('formatUptime', () => {
    it('formats 0 and negative seconds as 0s', () => {
      expect(formatUptime(0)).toBe('0s');
      expect(formatUptime(-10)).toBe('0s');
    });

    it('formats sub-minute seconds correctly', () => {
      expect(formatUptime(45)).toBe('45s');
      expect(formatUptime(1)).toBe('1s');
      expect(formatUptime(59)).toBe('59s');
    });

    it('formats minutes and seconds correctly', () => {
      expect(formatUptime(60)).toBe('1m 0s');
      expect(formatUptime(75)).toBe('1m 15s');
      expect(formatUptime(185)).toBe('3m 5s');
    });

    it('formats hours, minutes, and seconds correctly', () => {
      expect(formatUptime(3600)).toBe('1h 0m 0s');
      expect(formatUptime(3665)).toBe('1h 1m 5s');
      expect(formatUptime(7325)).toBe('2h 2m 5s');
    });

    it('formats days, hours, and minutes correctly', () => {
      expect(formatUptime(86400)).toBe('1d 0h 0m');
      expect(formatUptime(90061)).toBe('1d 1h 1m');
      expect(formatUptime(176400)).toBe('2d 1h 0m');
    });
  });

  describe('getUptimeSeconds', () => {
    it('calculates seconds elapsed from startedAt timestamp', () => {
      const past = new Date(Date.now() - 125 * 1000).toISOString();
      const secs = getUptimeSeconds(past);
      expect(secs).toBeGreaterThanOrEqual(124);
      expect(secs).toBeLessThanOrEqual(127);
    });

    it('falls back to fallbackSeconds if startedAt is missing or invalid', () => {
      expect(getUptimeSeconds(null, 42)).toBe(42);
      expect(getUptimeSeconds(undefined, 100)).toBe(100);
      expect(getUptimeSeconds('invalid-date', 50)).toBe(50);
    });

    it('returns null if neither startedAt nor fallbackSeconds are provided', () => {
      expect(getUptimeSeconds(null)).toBeNull();
      expect(getUptimeSeconds(undefined)).toBeNull();
    });
  });

  describe('formatUptimeFromStartedAt', () => {
    it('formats directly from timestamp', () => {
      const past = new Date(Date.now() - 65 * 1000).toISOString();
      const formatted = formatUptimeFromStartedAt(past);
      expect(formatted).toMatch(/1m \d+s/);
    });

    it('uses fallback seconds when timestamp is invalid', () => {
      expect(formatUptimeFromStartedAt(null, 75)).toBe('1m 15s');
    });

    it('returns null when no data is provided', () => {
      expect(formatUptimeFromStartedAt(null)).toBeNull();
    });
  });
});
