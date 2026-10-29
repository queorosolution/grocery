import { describe, expect, it } from "vitest";
import { safeColor, textOn } from "./safeColor";

describe("safeColor", () => {
  it("keeps a plain #rrggbb and nothing else", () => {
    expect(safeColor("#0F5132")).toBe("#0F5132");
    for (const bad of ["red", "#fff", "#0F5132; x:y", "url(x)", "", null, undefined, "#0F5132ff"]) expect(safeColor(bad)).toBeNull();
  });
  it("picks readable text", () => {
    expect(textOn("#0F5132")).toBe("#ffffff");
    expect(textOn("#FFE082")).toBe("#222222");
  });
});
