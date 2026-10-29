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
