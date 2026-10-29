import type {
  DeliveryType, DriverSource, NotifyChannel, PaymentAccountStatus, PaymentMode, PaymentProvider, ShopStatus, SlotStatus,
} from "@/lib/enums.gen";

// What the admin API sends (docs/06 section 10). Money is whole cents; distances and weights are numbers with 2 decimals.

export interface Shop {
  id: string;
  slug: string;
  name: string;
  phone: string | null;
  addressText: string | null;
  lat: number;
  lng: number;
  logoUrl: string | null;
  themeColor: string;
  status: ShopStatus;
  acceptingOrders: boolean;
  updatedAt: string;
}

export interface Settings {
  deliveryType: DeliveryType;
  paymentProvider: PaymentProvider;
  paymentAccountId: string | null;
  driverSource: DriverSource;
  notifyChannels: NotifyChannel[];
  markupBp: number;
  minPlatformMarginCents: number;
  minOrderCents: number;
  maxLines: number;
  reviewTimeoutMin: number;
  revisionTimeoutMin: number;
  paymentTimeoutMin: number;
}

export interface HoursDay {
  weekday: number;
  opens: string;
  closes: string;
}

export type ReadinessItem = "SHOP_ACTIVE" | "HOURS" | "PAYMENT_ACCOUNT" | "FUTURE_SLOT";

export interface ShopDetail {
  shop: Shop;
  settings: Settings & { cartTenantId: string | null; cartPublicKey: string | null };
  hours: HoursDay[];
  readiness: { ready: boolean; missing: ReadinessItem[] };
}

export interface Slot {
  id: string;
  shopId: string;
  startsAt: string;
  endsAt: string;
  cutoffAt: string;
  capacity: number;
  booked: number;
  status: SlotStatus;
}

export interface Bank {
  bankName: string;
  accountName: string;
  accountNumber: string;
  branch: string;
}

/** The merchant secret is never in this shape: only whether one is saved. */
export interface PaymentAccount {
  id: string;
  ownerType: "PLATFORM" | "SHOP";
  shopId: string | null;
  provider: PaymentProvider;
  mode: PaymentMode;
  merchantId: string | null;
  status: PaymentAccountStatus;
  hasSecret: boolean;
  bank: Bank | null;
}

export interface FeeRule {
  baseCents: number;
  perKmCents: number;
  freeKm: number;
  perKgCents: number;
  freeKg: number;
  freeAboveCents: number | null;
  minCents: number;
  maxCents: number | null;
  maxKm: number;
  roadFactor: number;
}

export interface FeeRow {
  shopId: string | null;
  rule: FeeRule;
  updatedAt: string;
}

export interface AdminDashboard {
  from: string;
  to: string;
  gmvCents: number;
  orders: number;
  delivered: number;
  avgBillCents: number;
  cancelRateBps: number;
  platformIncomeCents: number;
  marginCents: number;
  serviceFeeCents: number;
  couponCostCents: number;
  smsSent: number;
  failedPayments: number;
  sync: { active: number; failing: number; openIssues: number };
  days: { day: string; salesCents: number; orders: number }[];
  shops: { shopId: string; name: string; salesCents: number; orders: number; delivered: number; platformIncomeCents: number }[];
}

// Connector to the shop's own database (docs/06 section 10, docs/09 section 5). The connection string is never in these shapes.

export const COLUMN_KEYS = ["externalId", "name", "price", "barcode", "mrp", "stock", "available", "brand", "unit", "weightG", "category", "imageUrl", "updatedAt"] as const;
export type ColumnKey = (typeof COLUMN_KEYS)[number];
export const REQUIRED_COLUMNS: readonly ColumnKey[] = ["externalId", "name", "price"];

export interface Connector {
  id: string;
  shopId: string;
  status: "DRAFT" | "ACTIVE" | "PAUSED" | "FAILING";
  host: string;
  sourceTable: string | null;
  moneyUnit: "RUPEES" | "CENTS";
  changeColumn: string | null;
  intervalMin: number;
  autoCreateProducts: boolean;
  columns: Partial<Record<ColumnKey, string>>;
  lastRunAt: string | null;
}

export interface Mapping {
  sourceTable: string;
  moneyUnit: Connector["moneyUnit"];
  changeColumn: string;
  intervalMin: number;
  autoCreateProducts: boolean;
  columns: Partial<Record<ColumnKey, string>>;
}

export interface PreviewRow {
  externalId: string;
  name: string | null;
  barcode: string | null;
  priceCents: number | null;
  mrpCents: number | null;
  stockQty: number | null;
  available: boolean | null;
  action: "create" | "update" | "unmatched" | "hold" | "invalid";
  reason: string | null;
}

export interface SyncRun {
  id: string;
  trigger: "SCHEDULE" | "MANUAL" | "WEBHOOK";
  status: "RUNNING" | "SUCCESS" | "FAILED" | "SKIPPED";
  rowsRead: number;
  rowsChanged: number;
  rowsUnmatched: number;
  rowsHeld: number;
  error: "source_failed" | "apply_failed" | "timed_out" | null;
  startedAt: string;
  finishedAt: string | null;
}

export interface SyncIssue {
  id: string;
  connectorId: string;
  shopId: string;
  kind: "UNMATCHED" | "PRICE_HOLD";
  externalId: string;
  payload: {
    reason: string;
    name: string | null;
    barcode: string | null;
    brand: string | null;
    priceCents: number | null;
    mrpCents: number | null;
    stockQty: number | null;
    available: boolean | null;
    shopProductId?: string;
    currentPriceCents?: number;
  };
  status: "OPEN" | "RESOLVED" | "IGNORED";
  createdAt: string;
  resolvedAt: string | null;
}

export interface MasterProduct {
  id: string;
  barcode: string | null;
  name: string;
  brand: string | null;
  sizeLabel: string | null;
}
