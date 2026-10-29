import type { OrderStatus } from "@/lib/enums.gen";
import type { MessageKey } from "@/lib/i18n";

export interface QueueTab {
  id: string;
  label: MessageKey;
  statuses: readonly OrderStatus[];
}

// docs/08 section 4: New · Waiting · To pick · Packed · Out · Done. "Waiting" is the customer's turn.
export const QUEUE_TABS: readonly QueueTab[] = [
  { id: "new", label: "shopConsole.tabNew", statuses: ["SUBMITTED"] },
  { id: "waiting", label: "shopConsole.tabWaiting", statuses: ["REVISED", "AWAITING_PAYMENT"] },
  { id: "pick", label: "shopConsole.tabPick", statuses: ["PAID", "PICKING"] },
  { id: "packed", label: "shopConsole.tabPacked", statuses: ["PACKED"] },
  { id: "out", label: "shopConsole.tabOut", statuses: ["OUT_FOR_DELIVERY", "DELIVERY_FAILED"] },
  { id: "done", label: "shopConsole.tabDone", statuses: ["DELIVERED", "REJECTED", "EXPIRED", "CANCELLED", "CANCELLED_REFUND"] },
];
