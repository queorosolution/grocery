import type { OrderLine } from "./types";

export type LineChange =
  | { kind: "same"; now: OrderLine }
  | { kind: "changed"; now: OrderLine; was: OrderLine }
  | { kind: "added"; now: OrderLine }
  | { kind: "removed"; was: OrderLine };

const gone = (l: OrderLine) => l.state === "REMOVED" || l.state === "MISSING";

/**
 * What the shop changed between two revisions of a bill. A line matches its old self by id, or by
 * `replacesItemId` when the shop swapped one product for another.
 */
export function diffLines(previous: readonly OrderLine[], current: readonly OrderLine[]): LineChange[] {
  const out: LineChange[] = [];
  const matched = new Set<string>();
  for (const now of current) {
    const was = previous.find((p) => p.id === now.id || p.id === now.replacesItemId);
    if (was) matched.add(was.id);
    if (gone(now)) {
      if (was) out.push({ kind: "removed", was });
    } else if (!was) {
      out.push({ kind: "added", now });
    } else if (was.qty !== now.qty || was.unitPriceCents !== now.unitPriceCents || was.name !== now.name) {
      out.push({ kind: "changed", now, was });
    } else {
      out.push({ kind: "same", now });
    }
  }
  for (const was of previous) if (!matched.has(was.id) && !gone(was)) out.push({ kind: "removed", was });
  return out;
}
