import { describe, expect, it } from "vitest";
import { diffLines } from "./revisionDiff";
import type { OrderLine } from "./types";

const line = (id: string, over: Partial<OrderLine> = {}): OrderLine => ({
  id, name: id, unit: "PCS", qty: 1, unitPriceCents: 1000, lineCents: 1000, state: "ACTIVE", subPref: "SIMILAR", revisionNo: 1, replacesItemId: null, ...over,
});

describe("diffLines", () => {
  it("tells same, changed, added and removed apart", () => {
    const before = [line("rice"), line("milk"), line("tea")];
    const after = [line("rice"), line("milk", { qty: 2, lineCents: 2000 }), line("sugar")];
    expect(diffLines(before, after).map((c) => [c.kind, "now" in c ? c.now.id : c.was.id])).toEqual([
      ["same", "rice"], ["changed", "milk"], ["added", "sugar"], ["removed", "tea"],
    ]);
  });
  it("sees a swapped product as a change of the old line", () => {
    const out = diffLines([line("old", { name: "Brand A" })], [line("new", { name: "Brand B", replacesItemId: "old" })]);
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({ kind: "changed", was: { name: "Brand A" }, now: { name: "Brand B" } });
  });
  it("shows a line the shop took out as removed, once", () => {
    const out = diffLines([line("tea")], [line("tea", { state: "REMOVED" })]);
    expect(out.map((c) => c.kind)).toEqual(["removed"]);
  });
});
