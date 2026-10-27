import { describe, expect, it } from 'vitest';
import { compareFileLines } from '../src/lib/file-diff';

describe('file comparison', () => {
  it('aligns insertions, deletions, and separate changed lines', () => {
    const rows = compareFileLines('header\nremoved\nkeep\nold\nend', 'header\nkeep\nnew\nadded\nend');
    expect(rows.filter((row) => !row.changed).map((row) => row.server?.text)).toEqual(['header', 'keep', 'end']);
    expect(rows.find((row) => row.server?.text === 'removed')).toMatchObject({ changed: true, yours: undefined });
    expect(rows.find((row) => row.server?.text === 'old')).toMatchObject({ changed: true, yours: { text: 'new', number: 3 } });
    expect(rows.find((row) => row.yours?.text === 'added')).toMatchObject({ changed: true, server: undefined });
    expect(rows.at(-1)).toMatchObject({ server: { number: 5 }, yours: { number: 5 } });
  });

  it('preserves empty files, trailing newlines, and line ending changes', () => {
    expect(compareFileLines('', '')).toEqual([]);
    expect(compareFileLines('', 'new')).toEqual([{ changed: true, server: undefined, yours: { number: 1, text: 'new' } }]);
    expect(compareFileLines('one', 'one\n').at(-1)).toMatchObject({ changed: true, yours: { number: 2, text: '' } });
    expect(compareFileLines('one\r\n', 'one\n')[0].changed).toBe(true);
  });

  it('bounds work on large changed regions and preserves their common edges', () => {
    const before = `header\n${Array.from({ length: 1000 }, (_, i) => `old ${i}`).join('\n')}\nfooter`;
    const after = `header\n${Array.from({ length: 1000 }, (_, i) => `new ${i}`).join('\n')}\nfooter`;
    const rows = compareFileLines(before, after);
    expect(rows).toHaveLength(1002);
    expect(rows[0].changed).toBe(false);
    expect(rows.at(-1)?.changed).toBe(false);
    expect(rows.slice(1, -1).every((row) => row.changed)).toBe(true);
  });
});
