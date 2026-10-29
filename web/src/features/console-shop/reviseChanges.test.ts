import { describe, expect, it } from "vitest";
import { buildChanges, reviseProblem } from "./reviseChanges";
import { shopOrder } from "@/test/fixtures";

const lines = shopOrder().lines;
const edit = (over = {}) => ({ qty: 2, remove: false, substitute: null, ...over });

describe("buildChanges", () => {
  it("leaves out lines that were not touched or are back to what they were", () => {
    expect(buildChanges(lines, {})).toEqual([]);
    expect(buildChanges(lines, { l1: edit({ qty: 2 }) })).toEqual([]);
  });
  it("names one thing per line: quantity, remove or substitute", () => {
    const out = buildChanges(lines, {
      l1: edit({ qty: 1 }),
      l2: edit({ qty: 3, substitute: { shopProductId: "p9", name: "Other milk" } }),
    });
    expect(out).toEqual([{ itemId: "l1", qty: 1 }, { itemId: "l2", substitute: { shopProductId: "p9", qty: 3 } }]);
    expect(buildChanges(lines, { l1: edit({ remove: true, qty: 5 }) })).toEqual([{ itemId: "l1", remove: true }]);
  });
});

describe("reviseProblem", () => {
  it("needs a change, and cannot remove every line", () => {
    expect(reviseProblem(lines, [])).toBe("none");
    expect(reviseProblem(lines, [{ itemId: "l1", remove: true }, { itemId: "l2", remove: true }])).toBe("all");
    expect(reviseProblem(lines, [{ itemId: "l1", remove: true }])).toBeNull();
  });
  it("keeps quantities between 1 and 999", () => {
    expect(reviseProblem(lines, [{ itemId: "l1", qty: 0 }])).toBe("qty");
    expect(reviseProblem(lines, [{ itemId: "l1", qty: 1000 }])).toBe("qty");
    expect(reviseProblem(lines, [{ itemId: "l1", qty: 999 }])).toBeNull();
  });
});
