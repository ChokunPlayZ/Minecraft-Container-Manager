export interface DiffLine {
  number: number;
  text: string;
}

export interface FileDiffRow {
  server?: DiffLine;
  yours?: DiffLine;
  changed: boolean;
}

// Bound the LCS work for large files. Common edges are always retained; for a
// very large changed region, compare the whole region without quadratic work.
export function compareFileLines(server: string, yours: string): FileDiffRow[] {
  const a = server === '' ? [] : server.split('\n');
  const b = yours === '' ? [] : yours.split('\n');
  const rows: FileDiffRow[] = [];
  const same = (i: number, j: number) => rows.push({
    server: { number: i + 1, text: a[i] }, yours: { number: j + 1, text: b[j] }, changed: false,
  });
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) {
    same(start, start);
    start++;
  }
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  const changed = (fromA: number, toA: number, fromB: number, toB: number) => {
    for (let n = 0; n < Math.max(toA - fromA, toB - fromB); n++) {
      rows.push({
        server: fromA + n < toA ? { number: fromA + n + 1, text: a[fromA + n] } : undefined,
        yours: fromB + n < toB ? { number: fromB + n + 1, text: b[fromB + n] } : undefined,
        changed: true,
      });
    }
  };
  const height = endA - start;
  const width = endB - start;
  if ((height + 1) * (width + 1) > 250_000) {
    changed(start, endA, start, endB);
  } else {
    const stride = width + 1;
    const lengths = new Uint32Array((height + 1) * stride);
    for (let i = height - 1; i >= 0; i--) {
      for (let j = width - 1; j >= 0; j--) {
        lengths[i * stride + j] = a[start + i] === b[start + j]
          ? 1 + lengths[(i + 1) * stride + j + 1]
          : Math.max(lengths[(i + 1) * stride + j], lengths[i * stride + j + 1]);
      }
    }
    let i = 0;
    let j = 0;
    let fromA = start;
    let fromB = start;
    while (i < height || j < width) {
      if (i < height && j < width && a[start + i] === b[start + j]) {
        changed(fromA, start + i, fromB, start + j);
        same(start + i, start + j);
        i++;
        j++;
        fromA = start + i;
        fromB = start + j;
      } else if (i < height && (j === width || lengths[(i + 1) * stride + j] >= lengths[i * stride + j + 1])) {
        i++;
      } else {
        j++;
      }
    }
    changed(fromA, endA, fromB, endB);
  }
  while (endA < a.length && endB < b.length) same(endA++, endB++);
  return rows;
}
