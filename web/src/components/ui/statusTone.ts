import type { OrderStatus } from "@/lib/enums.gen";

export type Tone = "neutral" | "info" | "success" | "warning" | "danger";

// The ONE map from order status to colour. It is a Record, so adding a status to
// contracts/enums.json fails the type check here until it gets a tone.
const tones: Record<OrderStatus, Tone> = {
  SUBMITTED: "info",
  REVISED: "warning",
  AWAITING_PAYMENT: "warning",
  PAID: "success",
  PICKING: "info",
  PACKED: "info",
  OUT_FOR_DELIVERY: "info",
  DELIVERY_FAILED: "danger",
  DELIVERED: "success",
  REJECTED: "danger",
  EXPIRED: "neutral",
  CANCELLED: "neutral",
  CANCELLED_REFUND: "danger",
};

export const statusTone = (status: OrderStatus): Tone => tones[status];
