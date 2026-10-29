import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { Bank, PaymentAccount } from "../types";
import type { PaymentMode, PaymentProvider, PaymentAccountStatus } from "@/lib/enums.gen";

const key = ["payment-accounts"] as const;

export const usePaymentAccounts = () => useAdminQuery<{ data: PaymentAccount[] }>(key, "/payment-accounts");

export interface NewAccount {
  ownerType: "SHOP";
  shopId: string;
  provider: PaymentProvider;
  mode: PaymentMode;
  merchantId: string;
  /** Write-only: it goes to the server once and is never shown again. */
  merchantSecret: string;
  bank: Bank | null;
}

export const useCreateAccount = () => useAdminMutation((body: NewAccount) => adminCall("POST", "/payment-accounts", body), [key, ["shop"]]);

export interface AccountChange {
  mode?: PaymentMode;
  merchantId?: string;
  merchantSecret?: string;
  status?: PaymentAccountStatus;
  bank?: Bank;
}

export const useUpdateAccount = (id: string) => useAdminMutation((body: AccountChange) => adminCall("PATCH", `/payment-accounts/${id}`, body), [key, ["shop"]]);
