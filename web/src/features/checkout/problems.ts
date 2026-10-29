import type { MessageKey } from "@/lib/i18n";
import type { Quote } from "./types";

/**
 * Why "Send to shop" is not possible yet, in the order the person can fix them. These mirror the
 * checks the server repeats on submit (FR-BILL-03); the phone has its own card.
 */
export function problemsOf(quote: Quote, slotMissing: boolean): MessageKey[] {
  const out: MessageKey[] = [];
  if (!quote.checks.shopOpen) out.push("checkout.problemClosed");
  if (!quote.checks.inRange) out.push("checkout.problemRange");
  if (!quote.checks.minOrderMet) out.push("checkout.problemMin");
  if (quote.excluded.length > 0) out.push("checkout.problemExcluded");
  if (slotMissing) out.push("checkout.problemSlot");
  return out;
}
