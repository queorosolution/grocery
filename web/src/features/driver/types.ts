import type { OrderStatus } from "@/lib/enums.gen";

/** One order on the driver's list (docs/06 section 9). No code and no line prices. */
export interface Delivery {
  id: string;
  code: string;
  status: OrderStatus;
  version: number;
  customer: { name: string; phone: string };
  address: { label: string; line1: string; landmark: string | null; lat?: number; lng?: number };
  totalCents: number;
  itemCount: number;
}

export interface DeliveryGroup {
  slot: { startsAt: string; endsAt: string } | null;
  orders: Delivery[];
}

export interface DriverStats {
  delivered: number;
  failed: number;
}
