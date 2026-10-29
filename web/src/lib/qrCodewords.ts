// The data side of a QR code: how many bytes fit, and the data and error-correction codewords
// for them (ISO/IEC 18004, level M, byte mode). The picture side is in qr.ts.

// [error codewords per block, blocks in group 1, data per block, blocks in group 2, data per block]
export const BLOCKS = [
  [10, 1, 16, 0, 0], [16, 1, 28, 0, 0], [26, 1, 44, 0, 0], [18, 2, 32, 0, 0], [24, 2, 43, 0, 0],
  [16, 4, 27, 0, 0], [18, 4, 31, 0, 0], [22, 2, 38, 2, 39], [22, 3, 36, 2, 37], [26, 4, 43, 1, 44],
] as const;
export const ALIGN = [[], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50]];

// Arithmetic in GF(256) with the polynomial 0x11d.
const EXP = new Uint8Array(512);
const LOG = new Uint8Array(256);
for (let i = 0, x = 1; i < 255; i++) {
  EXP[i] = x;
  LOG[x] = i;
  x <<= 1;
  if (x & 256) x ^= 0x11d;
}
for (let i = 255; i < 512; i++) EXP[i] = EXP[i - 255]!;
const mul = (a: number, b: number) => (a && b ? EXP[LOG[a]! + LOG[b]!]! : 0);

function errorCodewords(data: number[], degree: number): number[] {
  let gen = [1];
  for (let i = 0; i < degree; i++) {
    const next = new Array<number>(gen.length + 1).fill(0);
    gen.forEach((c, j) => {
      next[j] = next[j]! ^ c;
      next[j + 1] = next[j + 1]! ^ mul(c, EXP[i]!);
    });
    gen = next;
  }
  const rem = new Array<number>(degree).fill(0);
  for (const b of data) {
    const factor = b ^ rem.shift()!;
    rem.push(0);
    gen.slice(1).forEach((c, i) => {
      rem[i] = rem[i]! ^ mul(c, factor);
    });
  }
  return rem;
}

const dataTotal = (v: number) => BLOCKS[v - 1]![1] * BLOCKS[v - 1]![2] + BLOCKS[v - 1]![3] * BLOCKS[v - 1]![4];
const countBits = (v: number) => (v < 10 ? 8 : 16);

export function pickVersion(length: number): number {
  for (let v = 1; v <= BLOCKS.length; v++) if (4 + countBits(v) + 8 * length <= 8 * dataTotal(v)) return v;
  throw new RangeError("Text is too long for the QR code");
}

function dataBytes(bytes: Uint8Array, version: number): number[] {
  const bits: number[] = [];
  const push = (value: number, n: number) => {
    for (let i = n - 1; i >= 0; i--) bits.push((value >> i) & 1);
  };
  push(0b0100, 4);
  push(bytes.length, countBits(version));
  bytes.forEach((b) => push(b, 8));
  const capacity = dataTotal(version) * 8;
  push(0, Math.min(4, capacity - bits.length));
  push(0, (8 - (bits.length % 8)) % 8);
  const out: number[] = [];
  for (let i = 0; i < bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8).join(""), 2));
  for (let pad = 0xec; out.length < dataTotal(version); pad ^= 0xec ^ 0x11) out.push(pad);
  return out;
}

/** Data and error codewords, taken in turn from each block. */
export function codewords(bytes: Uint8Array, version: number): number[] {
  const [ec, g1, d1, g2, d2] = BLOCKS[version - 1]!;
  const data = dataBytes(bytes, version);
  const blocks: { data: number[]; ec: number[] }[] = [];
  let at = 0;
  for (let i = 0; i < g1 + g2; i++) {
    const part = data.slice(at, at + (i < g1 ? d1 : d2));
    at += part.length;
    blocks.push({ data: part, ec: errorCodewords(part, ec) });
  }
  const out: number[] = [];
  for (let i = 0; i < Math.max(d1, d2); i++) for (const b of blocks) if (i < b.data.length) out.push(b.data[i]!);
  for (let i = 0; i < ec; i++) for (const b of blocks) out.push(b.ec[i]!);
  return out;
}
