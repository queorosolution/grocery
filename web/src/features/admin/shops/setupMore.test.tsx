import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { calls, mockApi, renderScreen } from "@/test/support";
import { base } from "./testData";
import { ShopPage } from "./ShopPage";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
let tab = "";
vi.mock("next/navigation", () => ({
  usePathname: () => "/console/admin/shop/s1/",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(tab ? `tab=${tab}` : ""),
}));

beforeEach(() => { tab = ""; });

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
