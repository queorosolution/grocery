import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { adminShop, superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import type { Coupon } from "../types";
import { CouponsScreen } from "./CouponsScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/coupons/", useRouter: () => ({ push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const coupon = (over: Partial<Coupon> = {}): Coupon => ({
  id: "c1", code: "WELCOME", shopId: null, type: "PERCENT", value: 1250, maxDiscountCents: 50000, minOrderCents: 100000,
  startsAt: "2026-10-01T00:00:00Z", endsAt: "2026-12-31T18:30:00Z", usageLimit: 100, perUserLimit: 1, usedCount: 7, fundedBy: "PLATFORM", allowLoss: false, status: "ACTIVE",
  createdAt: "2026-09-30T00:00:00Z", ...over,
});
const base = (extra: Record<string, () => unknown> = {}) => ({
  "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
  "GET /admin/shops": () => ({ data: [adminShop()] }),
  "GET /admin/coupons": () => ({ data: [coupon(), coupon({ id: "c2", code: "FLAT200", type: "FIXED", value: 20000, usageLimit: null, status: "PAUSED" })] }),
  ...extra,
});

describe("coupons", () => {
  it("shows percent from basis points and rupees from cents", async () => {
    mockApi(base());
    renderScreen(<CouponsScreen />);
    expect(await screen.findByText("12.5%")).toBeInTheDocument();
    expect(screen.getByText("Rs 200.00")).toBeInTheDocument();
  });

  it("makes a percent coupon in basis points with Colombo times", async () => {
    const fake = mockApi(base({ "POST /admin/coupons": () => ({}) }));
    renderScreen(<CouponsScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "New coupon" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Code"), "summer10");
    await userEvent.type(within(dialog).getByLabelText("Percent (like 12.5)"), "7.25");
    await userEvent.type(within(dialog).getByLabelText("Ends"), "2026-12-31T23:59");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "POST /admin/coupons")).toHaveLength(1));
    expect(calls(fake, "POST /admin/coupons")[0]![1].body).toMatchObject({
      code: "SUMMER10", type: "PERCENT", value: 725, shopId: null, maxDiscountCents: null, minOrderCents: 0, usageLimit: null, perUserLimit: 1, fundedBy: "PLATFORM",
      endsAt: "2026-12-31T18:29:00.000Z",
    });
  });

  it("makes a shop coupon only with a shop", async () => {
    mockApi(base());
    renderScreen(<CouponsScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "New coupon" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Code"), "SHOP1234");
    await userEvent.type(within(dialog).getByLabelText("Percent (like 12.5)"), "5");
    await userEvent.type(within(dialog).getByLabelText("Ends"), "2026-12-31T23:59");
    await userEvent.selectOptions(within(dialog).getByLabelText("Who pays"), "SHOP");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(within(dialog).getByLabelText("Allow the platform to lose money on it")).toBeDisabled();
    await userEvent.selectOptions(within(dialog).getByLabelText("Shop"), "s1");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("changes only what may change and keeps code, type and value off", async () => {
    const fake = mockApi(base({ "PATCH /admin/coupons/c1": () => ({}) }));
    renderScreen(<CouponsScreen />);
    await userEvent.click(await screen.findByText("WELCOME"));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Code")).toBeDisabled();
    expect(within(dialog).getByLabelText("Type")).toBeDisabled();
    expect(within(dialog).getByLabelText("Percent (like 12.5)")).toHaveValue("12.5");
    await userEvent.selectOptions(within(dialog).getByLabelText("Status"), "PAUSED");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "PATCH /admin/coupons/c1")).toHaveLength(1));
    const body = calls(fake, "PATCH /admin/coupons/c1")[0]![1].body;
    expect(body).toMatchObject({ status: "PAUSED", usageLimit: 100, maxDiscountCents: 50000, minOrderCents: 100000 });
    expect(body).not.toHaveProperty("code");
    expect(body).not.toHaveProperty("value");
  });
});
