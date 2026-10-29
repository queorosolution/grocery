import type { Role } from "@/lib/enums.gen";

// The shapes of GET /bootstrap and the `me` object (docs/06 section 3).

export type AdminTotp = "NOT_REQUIRED" | "SETUP_NEEDED" | "CODE_NEEDED" | "OK";

export interface RoleInfo {
  role: Role;
  shopId: string | null;
  shopSlug: string | null;
  shopName: string | null;
  /** A hint for the console; the API checks it again on every product edit. */
  canEditProducts?: boolean;
}

export interface Me {
  id: string;
  name: string;
  email: string;
  phoneMasked: string | null;
  phoneVerified: boolean;
  needsOnboarding: boolean;
  roles: RoleInfo[];
  adminTotp: AdminTotp;
}

export interface Bootstrap {
  platform: { name: string; domain: string; supportPhone: string | null; singleShopSlug: string | null };
  features: { whatsapp: boolean; manualPayment: boolean };
  me: Me | null;
}

export interface TotpSetup {
  otpauthUri: string;
  secretMasked: string;
}
