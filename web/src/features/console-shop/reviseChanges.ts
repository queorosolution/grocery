import type { LineChange } from "./api";
import type { ShopOrder } from "./types";

/** What the shop typed for one line of the bill. */
export interface LineEdit {
  qty: number;
  remove: boolean;
  substitute: { shopProductId: string; name: string } | null;
}

export const MAX_QTY = 999;

/** The shop's edits as the `changes` list of POST /review (docs/06 section 8). Untouched lines are left out. */
export function buildChanges(lines: ShopOrder["lines"], edits: Readonly<Record<string, LineEdit>>): LineChange[] {
  return lines.flatMap((l): LineChange[] => {
    const e = edits[l.id];
    if (!e) return [];
    if (e.remove) return [{ itemId: l.id, remove: true }];
    if (e.substitute) return [{ itemId: l.id, substitute: { shopProductId: e.substitute.shopProductId, qty: e.qty } }];
    return e.qty !== l.qty ? [{ itemId: l.id, qty: e.qty }] : [];
  });
}

/** Why the bill cannot be sent as it stands, or null. The server checks the same things. */
export function reviseProblem(lines: ShopOrder["lines"], changes: readonly LineChange[]): "none" | "all" | "qty" | null {
  if (changes.length === 0) return "none";
  if (changes.filter((c) => c.remove).length >= lines.length) return "all";
  const bad = changes.some((c) => (c.qty ?? c.substitute?.qty ?? 1) < 1 || (c.qty ?? c.substitute?.qty ?? 1) > MAX_QTY);
  return bad ? "qty" : null;
}
