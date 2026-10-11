import { describe, expect, it } from 'vitest';
import { parseDiff } from './DiffView';

describe('parseDiff', () => {
  it('numbers each hunk from its own header', () => {
    const diff = [
      '--- a/f.yaml',
      '+++ b/f.yaml',
      '@@ -2,3 +2,3 @@',
      ' a',
      '-b',
      '+B',
      ' c',
      '@@ -40,2 +40,2 @@',
      ' x',
      '-y',
      '+Y',
    ].join('\n');
    const lines = parseDiff(diff);
    const del = lines.filter((l) => l.type === 'del');
    const add = lines.filter((l) => l.type === 'add');
    expect(del.map((l) => l.lineOld)).toEqual([3, 41]);
    expect(add.map((l) => l.lineNew)).toEqual([3, 41]);
    expect(lines.find((l) => l.text === 'x')).toMatchObject({ lineOld: 40, lineNew: 40 });
  });

  it('starts an added file at line 1', () => {
    const lines = parseDiff(['--- /dev/null', '+++ b/n.txt', '@@ -0,0 +1,2 @@', '+one', '+two'].join('\n'));
    expect(lines.filter((l) => l.type === 'add').map((l) => l.lineNew)).toEqual([1, 2]);
  });

  it('treats the missing-newline marker as metadata, not a line', () => {
    const lines = parseDiff(['@@ -1,1 +1,1 @@', '-a', '\\ No newline at end of file', '+b'].join('\n'));
    expect(lines[2]).toMatchObject({ type: 'meta' });
    expect(lines[3]).toMatchObject({ type: 'add', lineNew: 1 });
  });
});
