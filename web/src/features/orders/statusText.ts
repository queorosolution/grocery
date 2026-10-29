import type { OrderStatus, TicketReason } from "@/lib/enums.gen";
import type { MessageKey } from "@/lib/i18n";

// Records, so a status or reason added to contracts/enums.json fails the type check until it has words.
export const statusText: Record<OrderStatus, MessageKey> = {
  SUBMITTED: "status.SUBMITTED",
  REVISED: "status.REVISED",
  AWAITING_PAYMENT: "status.AWAITING_PAYMENT",
  PAID: "status.PAID",
  PICKING: "status.PICKING",
  PACKED: "status.PACKED",
  OUT_FOR_DELIVERY: "status.OUT_FOR_DELIVERY",
  DELIVERY_FAILED: "status.DELIVERY_FAILED",
  DELIVERED: "status.DELIVERED",
  REJECTED: "status.REJECTED",
  EXPIRED: "status.EXPIRED",
  CANCELLED: "status.CANCELLED",
  CANCELLED_REFUND: "status.CANCELLED_REFUND",
};

export const ticketText: Record<TicketReason, MessageKey> = {
  MISSING_ITEM: "ticket.MISSING_ITEM",
  DAMAGED: "ticket.DAMAGED",
  WRONG_ITEM: "ticket.WRONG_ITEM",
  LATE: "ticket.LATE",
  OTHER: "ticket.OTHER",
};
