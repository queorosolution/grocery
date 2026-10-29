import { describe, expect, it } from "vitest";
import { bpToText, parseDec2, parseInt10, parsePercentBp } from "./form";
import { toRule } from "./shops/RuleForm";

describe("number boxes", () => {
  it("takes whole numbers only", () => {
    expect(parseInt10("20")).toBe(20);
    expect(parseInt10(" 7 ")).toBe(7);
    for (const bad of ["", "-1", "1.5", "abc", "1e3", "1234567890"]) expect(parseInt10(bad)).toBeNull();
  });

  it("takes at most 2 decimals for km and kg", () => {
    expect(parseDec2("1.25")).toBe(1.25);
    expect(parseDec2("3")).toBe(3);
    for (const bad of ["", "1.234", "-2", ".5", "1,5"]) expect(parseDec2(bad)).toBeNull();
  });
});

describe("fee rule from text", () => {
  const draft = { baseCents: "100", perKmCents: "25.50", perKgCents: "0", minCents: "150", freeAboveCents: "", maxCents: "1,000", freeKm: "1", freeKg: "2.5", maxKm: "10", roadFactor: "1.3" };

  it("turns rupees into exact cents and a blank limit into null", () => {
    expect(toRule(draft)).toEqual({
      baseCents: 10000, perKmCents: 2550, perKgCents: 0, minCents: 15000, freeAboveCents: null, maxCents: 100000,
      freeKm: 1, freeKg: 2.5, maxKm: 10, roadFactor: 1.3,
    });
  });

  it("refuses a box that is not a number", () => {
    expect(toRule({ ...draft, baseCents: "ten" })).toBeNull();
    expect(toRule({ ...draft, maxKm: "1.234" })).toBeNull();
    expect(toRule({ ...draft, maxCents: "1.234" })).toBeNull();
  });
});

describe("percent as basis points", () => {
  it("splits the text, no floating point", () => {
    expect(parsePercentBp("12.5")).toBe(1250);
    expect(parsePercentBp("7.25")).toBe(725);
    expect(parsePercentBp("100")).toBe(10000);
    for (const bad of ["", "0", "100.01", "1.234", "-5", "abc"]) expect(parsePercentBp(bad)).toBeNull();
  });

  it("writes basis points back as percent text", () => {
    expect(bpToText(1250)).toBe("12.5");
    expect(bpToText(1205)).toBe("12.05");
    expect(bpToText(1200)).toBe("12");
    expect(bpToText(5)).toBe("0.05");
  });
});
