import { describe, expect, it } from "vitest";
import { formatLKR, saveCents } from "./money";

describe("formatLKR", () => {
  it.each([
    [0, "Rs 0.00"],
    [5, "Rs 0.05"],
    [100, "Rs 1.00"],
    [143500, "Rs 1,435.00"],
    [150050, "Rs 1,500.50"],
    [123456789, "Rs 1,234,567.89"],
    [-250, "-Rs 2.50"],
  ])("%d -> %s", (cents, want) => {
    expect(formatLKR(cents)).toBe(want);
  });

  it("can drop .00 in compact mode, but keeps real cents", () => {
    expect(formatLKR(6500, { compact: true })).toBe("Rs 65");
    expect(formatLKR(6550, { compact: true })).toBe("Rs 65.50");
  });

  it("refuses floats and non-numbers: money is whole cents", () => {
    expect(() => formatLKR(10.5)).toThrow(RangeError);
    expect(() => formatLKR(Number.NaN)).toThrow(RangeError);
    expect(() => formatLKR(Number.POSITIVE_INFINITY)).toThrow(RangeError);
  });
});

describe("saveCents", () => {
  it("is the gap to the MRP, never negative", () => {
    expect(saveCents(143500, 150000)).toBe(6500);
    expect(saveCents(150000, 150000)).toBe(0);
    expect(saveCents(160000, 150000)).toBe(0);
    expect(saveCents(100, null)).toBe(0);
    expect(saveCents(100)).toBe(0);
  });
});
