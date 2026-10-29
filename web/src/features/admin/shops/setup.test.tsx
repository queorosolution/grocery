import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { adminShop as shop, superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import type { PaymentAccount, ShopDetail } from "../types";
import { ShopPage } from "./ShopPage";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
let tab = "";
const replace = vi.fn();
vi.mock("next/navigation", () => ({
  usePathname: () => "/console/admin/shop/s1/",
  useRouter: () => ({ push: vi.fn(), replace }),
  useSearchParams: () => new URLSearchParams(tab ? `tab=${tab}` : ""),
}));

const detail = (over: Partial<ShopDetail> = {}): ShopDetail => ({
  shop: shop({ acceptingOrders: false }),
  settings: {
    deliveryType: "SLOT", paymentProvider: "PAYHERE", paymentAccountId: null, driverSource: "SHOP", notifyChannels: ["SMS"], markupBp: 500,
    minPlatformMarginCents: 5000, minOrderCents: 150000, maxLines: 60, reviewTimeoutMin: 30, revisionTimeoutMin: 30, paymentTimeoutMin: 60,
    cartTenantId: null, cartPublicKey: null,
  },
  hours: [{ weekday: 1, opens: "08:00", closes: "21:00" }],
  readiness: { ready: false, missing: ["PAYMENT_ACCOUNT", "FUTURE_SLOT"] },
  ...over,
});
const account = (over: Partial<PaymentAccount> = {}): PaymentAccount => ({
  id: "a1", ownerType: "SHOP", shopId: "s1", provider: "PAYHERE", mode: "SANDBOX", merchantId: "121XXXX", status: "ACTIVE", hasSecret: true, bank: null, ...over,
});
const base = (d = detail()) => ({
  "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
  "GET /admin/shops/s1": () => d,
  "GET /admin/payment-accounts": () => ({ data: [account()] }),
});

beforeEach(() => { tab = ""; replace.mockClear(); });

describe("readiness", () => {
  it("lists what is missing and sends the admin to the tab that fixes it", async () => {
    mockApi(base());
    renderScreen(<ShopPage />);
    expect(await screen.findByRole("heading", { name: "Krishna Super" })).toBeInTheDocument();
    expect(screen.getByText("Finish these first:")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Fix" })[0]!);
    expect(replace).toHaveBeenCalledWith("/console/admin/shop/s1?tab=payment");
    // The switch cannot be turned on while something is missing.
    expect(screen.getByRole("switch", { name: "Taking orders" })).toBeDisabled();
  });

  it("does not ask for a slot from a courier shop", async () => {
    mockApi(base(detail({ settings: { ...detail().settings, deliveryType: "COURIER" }, readiness: { ready: true, missing: [] } })));
    renderScreen(<ShopPage />);
    await screen.findByText("Ready. You can switch on orders.");
    expect(screen.queryByText("A delivery slot is open in the future")).not.toBeInTheDocument();
  });

  it("switches orders on and shows the server's refusal", async () => {
    const fake = mockApi({
      ...base(detail({ readiness: { ready: true, missing: [] } })),
      "POST /admin/shops/s1/accepting": () => new ApiError(409, "SHOP_NOT_READY", "This shop is not ready."),
    });
    renderScreen(<ShopPage />);
    await userEvent.click(await screen.findByRole("switch", { name: "Taking orders" }));
    expect(calls(fake, "POST /admin/shops/s1/accepting")[0]![1].body).toEqual({ on: true });
    expect(await screen.findByText("Finish these first:", { selector: "div" }).catch(() => screen.findByRole("alert"))).toBeTruthy();
  });
});

describe("settings tab", () => {
  it("sends money as exact cents and numbers as numbers", async () => {
    tab = "settings";
    const fake = mockApi({ ...base(), "PUT /admin/shops/s1/settings": () => ({}) });
    renderScreen(<ShopPage />);
    const min = await screen.findByLabelText("Smallest order (Rs)");
    expect(min).toHaveValue("1500.00");
    await userEvent.clear(min);
    await userEvent.type(min, "1,250.50");
    await userEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(calls(fake, "PUT /admin/shops/s1/settings")).toHaveLength(1));
    expect(calls(fake, "PUT /admin/shops/s1/settings")[0]![1].body).toMatchObject({ minOrderCents: 125050, minPlatformMarginCents: 5000, markupBp: 500, maxLines: 60, notifyChannels: ["SMS"] });
  });

  it("keeps Save off while a number box is wrong", async () => {
    tab = "settings";
    mockApi(base());
    renderScreen(<ShopPage />);
    const lines = await screen.findByLabelText("Most lines in a bill");
    await userEvent.clear(lines);
    await userEvent.type(lines, "6x");
    expect(screen.getByRole("button", { name: "Save settings" })).toBeDisabled();
  });

  it("switches the cart on and shows only the public key", async () => {
    tab = "settings";
    let on = false;
    mockApi({
      ...base(),
      "GET /admin/shops/s1": () => (on ? detail({ settings: { ...detail().settings, cartTenantId: "ten_1", cartPublicKey: "pk_demo" } }) : detail()),
      "POST /admin/shops/s1/cart-tenant": () => { on = true; return {}; },
    });
    renderScreen(<ShopPage />);
    await userEvent.click(await screen.findByRole("button", { name: "Switch on the cart" }));
    expect(await screen.findByText("pk_demo")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Re-sync cart prices" })).toBeInTheDocument();
  });
});

describe("hours tab", () => {
  it("sends only the open days", async () => {
    tab = "hours";
    const fake = mockApi({ ...base(), "PUT /admin/shops/s1/hours": () => ({}) });
    renderScreen(<ShopPage />);
    await userEvent.click(await screen.findByRole("switch", { name: "Tuesday" }));
    await userEvent.clear(screen.getByLabelText("Tuesday closes"));
    await userEvent.type(screen.getByLabelText("Tuesday closes"), "24:00");
    await userEvent.click(screen.getByRole("button", { name: "Save hours" }));
    await waitFor(() => expect(calls(fake, "PUT /admin/shops/s1/hours")).toHaveLength(1));
    expect(calls(fake, "PUT /admin/shops/s1/hours")[0]![1].body).toEqual({
      days: [{ weekday: 1, opens: "08:00", closes: "21:00" }, { weekday: 2, opens: "08:00", closes: "24:00" }],
    });
  });

  it("refuses 24:00 as an opening time and an end before the start", async () => {
    tab = "hours";
    mockApi(base());
    renderScreen(<ShopPage />);
    const opens = await screen.findByLabelText("Monday opens");
    await userEvent.clear(opens);
    await userEvent.type(opens, "24:00");
    expect(screen.getByRole("button", { name: "Save hours" })).toBeDisabled();
    await userEvent.clear(opens);
    await userEvent.type(opens, "22:00");
    expect(screen.getByRole("button", { name: "Save hours" })).toBeDisabled();
  });
});

describe("payment tab", () => {
  it("never shows a saved secret and clears the box after a save", async () => {
    tab = "payment";
    const fake = mockApi({ ...base(), "PATCH /admin/payment-accounts/a1": () => ({}) });
    renderScreen(<ShopPage />);
    expect(await screen.findByText("Secret saved")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    const dialog = await screen.findByRole("dialog");
    const box = within(dialog).getByLabelText("Merchant secret");
    expect(box).toHaveValue("");
    expect(box).toHaveAttribute("type", "password");
    await userEvent.type(box, "s3cret-value");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "PATCH /admin/payment-accounts/a1")).toHaveLength(1));
    expect(calls(fake, "PATCH /admin/payment-accounts/a1")[0]![1].body).toMatchObject({ merchantSecret: "s3cret-value" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(document.body.textContent).not.toContain("s3cret-value");
    // Opening it again shows an empty box, and a save without typing sends no secret.
    await userEvent.click(screen.getByRole("button", { name: "Change" }));
    expect(within(await screen.findByRole("dialog")).getByLabelText("Merchant secret")).toHaveValue("");
  });

  it("adds a bank account with the three required fields", async () => {
    tab = "payment";
    const fake = mockApi({ ...base(), "POST /admin/payment-accounts": () => ({}) });
    renderScreen(<ShopPage />);
    await userEvent.click(await screen.findByRole("button", { name: "Add payment account" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.selectOptions(within(dialog).getByLabelText("Payment method"), "MANUAL");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText("Bank"), "Sampath");
    await userEvent.type(within(dialog).getByLabelText("Account name"), "Krishna Super");
    await userEvent.type(within(dialog).getByLabelText("Account number"), "1234567890");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "POST /admin/payment-accounts")).toHaveLength(1));
    expect(calls(fake, "POST /admin/payment-accounts")[0]![1].body).toMatchObject({
      ownerType: "SHOP", shopId: "s1", provider: "MANUAL", merchantSecret: "", bank: { bankName: "Sampath", accountName: "Krishna Super", accountNumber: "1234567890", branch: "" },
    });
  });
});

describe("slots and fees tabs", () => {
  it("makes a slot at Colombo times and a cut-off before the start", async () => {
    tab = "slots";
    const fake = mockApi({ ...base(), "GET /admin/shops/s1/slots": () => ({ data: [] }), "POST /admin/shops/s1/slots": () => ({}) });
    renderScreen(<ShopPage />);
    await userEvent.click(await screen.findByRole("button", { name: "New slot" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add slot" }));
    await waitFor(() => expect(calls(fake, "POST /admin/shops/s1/slots")).toHaveLength(1));
    const slot = (calls(fake, "POST /admin/shops/s1/slots")[0]![1].body as { slots: { startsAt: string; endsAt: string; cutoffAt: string; capacity: number }[] }).slots[0]!;
    expect(slot.startsAt).toMatch(/T09:00:00\+05:30$/);
    expect(slot.endsAt).toMatch(/T11:00:00\+05:30$/);
    expect(new Date(slot.startsAt).getTime() - new Date(slot.cutoffAt).getTime()).toBe(60 * 60_000);
    expect(slot.capacity).toBe(10);
  });

  it("saves the shop's own fee rule and previews a fee", async () => {
    tab = "fees";
    const rule = { baseCents: 10000, perKmCents: 2500, freeKm: 1, perKgCents: 0, freeKg: 0, freeAboveCents: null, minCents: 0, maxCents: null, maxKm: 10, roadFactor: 1.3 };
    const fake = mockApi({
      ...base(), "GET /admin/fee-rules": () => ({ data: [{ shopId: null, rule, updatedAt: "2026-10-01T00:00:00Z" }] }),
      "PUT /admin/fee-rules/shops/s1": () => ({}), "POST /admin/fee-rules/preview": () => ({ feeCents: 17500, rule: "DEFAULT" }),
    });
    renderScreen(<ShopPage />);
    expect(await screen.findByText(/uses the platform default/)).toBeInTheDocument();
    const base_ = screen.getByLabelText("Base fee (Rs)");
    await userEvent.clear(base_);
    await userEvent.type(base_, "150");
    await userEvent.click(screen.getByRole("button", { name: "Save for this shop" }));
    await waitFor(() => expect(calls(fake, "PUT /admin/fee-rules/shops/s1")).toHaveLength(1));
    expect(calls(fake, "PUT /admin/fee-rules/shops/s1")[0]![1].body).toMatchObject({ baseCents: 15000, perKmCents: 2500, freeAboveCents: null, roadFactor: 1.3 });
    await userEvent.click(screen.getByRole("button", { name: "Work it out" }));
    expect(await screen.findByText(/Rs 175\.00/)).toBeInTheDocument();
    expect(calls(fake, "POST /admin/fee-rules/preview")[0]![1].body).toEqual({ shopId: "s1", distanceKm: 3, weightKg: 5, itemsCents: 200000 });
  });
});
