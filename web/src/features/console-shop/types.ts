import type { OrderDetail } from "@/features/orders/types";
import type { OrderStatus, Role } from "@/lib/enums.gen";

/** One row of the queue (docs/06 section 8). */
export interface QueueRow {
  id: string;
  shopId: string;
  code: string;
  status: OrderStatus;
  version: number;
  customer: { name: string; phoneMasked: string };
  slot: { startsAt: string; endsAt: string } | null;
  totalCents: number;
  itemCount: number;
  createdAt: string;
}

/** The order as the shop sees it: also who to call and where to go. */
export interface ShopOrder extends Omit<OrderDetail, "address"> {
  customer?: { name: string; phone?: string; phoneMasked?: string };
  address: OrderDetail["address"] & { lat?: number; lng?: number };
}

export interface ShopProduct {
  id: string;
  productId: string;
  name: string;
  brand: string | null;
  sizeLabel: string | null;
  barcode: string | null;
  imageUrl: string | null;
  shopPriceCents: number;
  mrpCents: number | null;
  onlinePriceCents: number;
  stockQty: number | null;
  isAvailable: boolean;
  maxPerOrder: number;
  managedBy: "MANUAL" | "CONNECTOR";
  updatedAt: string;
}

export interface Member {
  id: string;
  userId: string;
  name: string;
  email: string;
  role: Role;
  canEditProducts: boolean;
}

export interface Invite {
  id: string;
  email: string;
  role: Role;
  canEditProducts: boolean;
  expiresAt: string;
}

export interface Team {
  members: Member[];
  invites: Invite[];
}

export interface Dashboard {
  from: string;
  to: string;
  queue: Partial<Record<OrderStatus, number>>;
  avgReviewSeconds: number;
  avgPickSeconds: number;
  full?: {
    salesCents: number;
    orders: number;
    delivered: number;
    avgBillCents: number;
    cancelRateBps: number;
    shopItemsCents: number;
    discountCents: number;
    marginCents: number;
    payoutDueCents: number;
    days: { day: string; salesCents: number; orders: number }[];
    topProducts: { productId: string; name: string; quantity: string; salesCents: number }[];
  };
}

export interface Payout {
  id: string;
  shopId: string;
  periodFrom: string;
  periodTo: string;
  amountCents: number;
  status: "DRAFT" | "PAID";
  reference: string | null;
  paidAt: string | null;
  createdAt: string;
}
