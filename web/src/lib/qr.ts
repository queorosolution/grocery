// A small QR code encoder: byte mode, error correction M, versions 1 to 10 (213 bytes).
// A library is not allowed (docs/08 section 1) and the authenticator link is about 120 bytes.
// Checked against golden matrices from an independent encoder and decoded with a scanner
// (qr.test.ts). Steps follow ISO/IEC 18004.

import { ALIGN, codewords, pickVersion } from "./qrCodewords";

export type QrMatrix = boolean[][];
type Put = (x: number, y: number, dark: boolean) => void;

const MASKS: ((x: number, y: number) => boolean)[] = [
  (x, y) => (x + y) % 2 === 0,
  (_, y) => y % 2 === 0,
  (x) => x % 3 === 0,
  (x, y) => (x + y) % 3 === 0,
  (x, y) => (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0,
  (x, y) => ((x * y) % 2) + ((x * y) % 3) === 0,
  (x, y) => (((x * y) % 2) + ((x * y) % 3)) % 2 === 0,
  (x, y) => (((x + y) % 2) + ((x * y) % 3)) % 2 === 0,
];

const bit = (value: number, i: number) => ((value >>> i) & 1) === 1;

function drawFormat(put: Put, size: number, mask: number): void {
  const data = mask; // level M is 00
  let rem = data;
  for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
  const bits = ((data << 10) | rem) ^ 0x5412;
  for (let i = 0; i <= 5; i++) put(8, i, bit(bits, i));
  put(8, 7, bit(bits, 6));
  put(8, 8, bit(bits, 7));
  put(7, 8, bit(bits, 8));
  for (let i = 9; i < 15; i++) put(14 - i, 8, bit(bits, i));
  for (let i = 0; i < 8; i++) put(size - 1 - i, 8, bit(bits, i));
  for (let i = 8; i < 15; i++) put(8, size - 15 + i, bit(bits, i));
  put(8, size - 8, true); // the always-dark module
}

function drawVersion(put: Put, size: number, version: number): void {
  if (version < 7) return;
  let rem = version;
  for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25);
  const bits = (version << 12) | rem;
  for (let i = 0; i < 18; i++) {
    const a = size - 11 + (i % 3);
    const b = Math.floor(i / 3);
    put(a, b, bit(bits, i));
    put(b, a, bit(bits, i));
  }
}

/** Finder, timing and alignment patterns, and room for the format and version bits. */
function drawFrame(put: Put, size: number, version: number): void {
  for (let i = 0; i < size; i++) {
    put(6, i, i % 2 === 0);
    put(i, 6, i % 2 === 0);
  }
  for (const [cx, cy] of [[3, 3], [size - 4, 3], [3, size - 4]] as const) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const d = Math.max(Math.abs(dx), Math.abs(dy));
        if (cx + dx >= 0 && cx + dx < size && cy + dy >= 0 && cy + dy < size) put(cx + dx, cy + dy, d !== 2 && d !== 4);
      }
    }
  }
  const centres = ALIGN[version - 1]!;
  const last = centres.length - 1;
  centres.forEach((cx, i) =>
    centres.forEach((cy, j) => {
      if ((i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0)) return; // under a finder
      for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) put(cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
    }),
  );
  drawFormat(put, size, 0);
  drawVersion(put, size, version);
}

/** Penalty points of ISO 18004 section 7.8.3: the lowest score wins the mask. */
function penalty(g: QrMatrix): number {
  const size = g.length;
  const lines = [
    ...g.map((row) => row.map(Number).join("")),
    ...g.map((_, x) => g.map((row) => Number(row[x])).join("")),
  ];
  let score = 0;
  for (const line of lines) {
    for (const run of line.match(/0+|1+/g) ?? []) if (run.length >= 5) score += run.length - 2;
    score += 40 * ((line.match(/(?=00001011101|10111010000)/g) ?? []).length);
  }
  for (let y = 0; y < size - 1; y++) {
    for (let x = 0; x < size - 1; x++) {
      const v = g[y]![x];
      if (v === g[y]![x + 1] && v === g[y + 1]![x] && v === g[y + 1]![x + 1]) score += 3;
    }
  }
  const dark = g.reduce((n, row) => n + row.filter(Boolean).length, 0);
  return score + (Math.ceil(Math.abs(dark * 20 - size * size * 10) / (size * size)) - 1) * 10;
}

/**
 * The modules of a QR code for `text` (true = dark), without the quiet zone.
 * `mask` forces one mask; leave it out to take the best one (only tests force it).
 */
export function qrMatrix(text: string, mask?: number): QrMatrix {
  const bytes = new TextEncoder().encode(text);
  const version = pickVersion(bytes.length);
  const size = version * 4 + 17;
  const modules: QrMatrix = Array.from({ length: size }, () => new Array<boolean>(size).fill(false));
  const fixed: QrMatrix = Array.from({ length: size }, () => new Array<boolean>(size).fill(false));
  drawFrame((x, y, dark) => {
    modules[y]![x] = dark;
    fixed[y]![x] = true;
  }, size, version);

  // Bits run up and down in pairs of columns, right to left, skipping the timing column.
  const cw = codewords(bytes, version);
  let i = 0;
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5;
    for (let vert = 0; vert < size; vert++) {
      for (let j = 0; j < 2; j++) {
        const x = right - j;
        const y = ((right + 1) & 2) === 0 ? size - 1 - vert : vert;
        if (!fixed[y]![x] && i < cw.length * 8) {
          modules[y]![x] = bit(cw[i >>> 3]!, 7 - (i & 7));
          i++;
        }
      }
    }
  }

  const candidates = mask === undefined ? MASKS.keys() : [mask];
  let best: QrMatrix = modules;
  let bestScore = Infinity;
  for (const m of candidates) {
    const g = modules.map((row, y) => row.map((v, x) => (fixed[y]![x] ? v : v !== MASKS[m]!(x, y))));
    drawFormat((x, y, dark) => {
      g[y]![x] = dark;
    }, size, m);
    const score = penalty(g);
    if (score < bestScore) [best, bestScore] = [g, score];
  }
  return best;
}
