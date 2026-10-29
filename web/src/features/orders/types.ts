import type { OrderStatus, PaymentProvider, SubstitutionPref } from "@/lib/enums.gen";

/** The names in `allowedActions` (docs/06 section 6). The page shows a button only for a name it gets. */
export type OrderAction =
  | "CANCEL" | "ACCEPT_REVISION" | "APPLY_COUPON" | "REMOVE_COUPON" | "PAY" | "REPORT_PROBLEM";

export interface OrderLine {
  id: string;
  name: string;
  unit: string;
  qty: number;
  unitPriceCents: number | null;
  lineCents: number | null;
  state: string;
  subPref: SubstitutionPref;
  revisionNo: number;
  replacesItemId: string | null;
}

export interface Totals {
  itemsCents: number;
  deliveryFeeCents: number;
  serviceFeeCents: number;
  discountCents: number;
  totalCents: number;
}

export interface OrderDetail {
  id: string;
  code: string;
  status: OrderStatus;
  version: number;
  revisionNo: number;
  shop: { id: string; slug: string; name: string; phone: string | null };
  slot: { startsAt: string; endsAt: string } | null;
  address: { label: string; line1: string; landmark: string | null };
  lines: OrderLine[];
  previousLines: OrderLine[];
  totals: Totals;
  coupon: { code: string; discountCents: number } | null;
  payment: { provider: PaymentProvider; status: string };
  deliveryCode: string | null;
  expiresAt?: string;
  timeline: { status: OrderStatus; at: string }[];
  allowedActions: string[];
}

export interface OrderSummary {
  id: string;
  code: string;
  status: OrderStatus;
  shop: { slug: string; name: string };
  totalCents: number;
  itemCount: number;
  createdAt: string;
}

export type PaymentStart =
  | { provider: "PAYHERE"; action: string; fields: Record<string, string> }
  | { provider: "MANUAL"; bank: { bankName: string; accountName: string; accountNumber: string; branch: string }; reference: string; amountCents: number };
