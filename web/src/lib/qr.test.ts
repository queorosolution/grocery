import { describe, expect, it } from "vitest";
import golden from "./qr.golden.json";
import { qrMatrix } from "./qr";

// qr.golden.json was made by the Python "qrcode" encoder (level M, byte mode) with the mask the
// spec scoring picks; each row is one hex number. A scanner (OpenCV) also read every sample
// from this encoder's own output.
const toRows = (hex: string[]) => hex.map((h) => BigInt(`0x${h}`).toString(2).padStart(hex.length, "0"));
const asText = (m: boolean[][]) => m.map((row) => row.map((v) => (v ? "1" : "0")).join(""));

describe("qrMatrix", () => {
  it.each(Object.entries(golden.auto))("matches the reference for sample %s (version, mask and every module)", (_, g) => {
    const m = qrMatrix(g.text);
    expect(m.length).toBe(g.version * 4 + 17);
    expect(asText(m)).toEqual(toRows(g.rows));
  });

  it.each(Object.entries(golden.masks))("matches the reference with mask %s forced", (mask, rows) => {
    const text = golden.auto.C.text;
    expect(asText(qrMatrix(text, Number(mask)))).toEqual(toRows(rows));
  });

  it("grows the version with the text and refuses what does not fit", () => {
    expect(qrMatrix("a").length).toBe(21);
    expect(qrMatrix("a".repeat(213)).length).toBe(57);
    expect(() => qrMatrix("a".repeat(214))).toThrow(RangeError);
  });

  it("encodes text as UTF-8", () => {
    expect(() => qrMatrix("සිංහල")).not.toThrow();
  });
});
