import type { SubstitutionPref } from "@/lib/enums.gen";
import type { Totals } from "@/features/orders/types";

/** A place for this order only (docs/06 section 6). `label` is left out when empty. */
export interface Destination {
  label?: string;
  line1: string;
  landmark?: string;
  lat: number;
  lng: number;
}

export interface QuoteLine {
  shopProductId: string;
  name: string;
  unit: string;
  qty: number;
  unitPriceCents: number;
  lineCents: number;
  status: string;
  priceChanged: boolean;
  subPref: SubstitutionPref;
}

export interface Slot {
  id: string;
  startsAt: string;
  endsAt: string;
  remaining: number;
}

export interface Quote {
  lines: QuoteLine[];
  excluded: { shopProductId: string; name: string; reason: string }[];
  totals: Totals;
  delivery: { distanceKm: number; weightKg: number; feeRule: string };
  slots: Slot[];
  checks: { shopOpen: boolean; inRange: boolean; minOrderMet: boolean; phoneVerified: boolean };
}

/** Where the order goes: a saved address, or a one-time destination. Exactly one is sent. */
export type Where = { addressId: string } | { destination: Destination };
