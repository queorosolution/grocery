import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { owner, staff } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { DashboardScreen } from "./DashboardScreen";
import { PayoutsScreen } from "./PayoutsScreen";
import { ProductsScreen } from "./ProductsScreen";
import { TeamScreen } from "./TeamScreen";
import type { ShopProduct } from "./types";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
let path = "/console/shop/s1/products/";
vi.mock("next/navigation", () => ({ usePathname: () => path }));

const S = "/console/shops/s1";
const product = (over: Partial<ShopProduct> = {}): ShopProduct => ({
  id: "p1", productId: "g1", name: "Samba Rice", brand: "Araliya", sizeLabel: "5kg", barcode: null, imageUrl: null, shopPriceCents: 125000,
  mrpCents: 150000, onlinePriceCents: 143500, stockQty: 12, isAvailable: true, maxPerOrder: 20, managedBy: "MANUAL", updatedAt: "2026-11-01T00:00:00Z", ...over,
});
const list = (...rows: ShopProduct[]) => ({ data: rows, nextCursor: "" });
const who = (roles = owner()) => ({ "GET /bootstrap": () => bootstrap(me({ id: "me1", roles })) });

beforeEach(() => { path = "/console/shop/s1/products/"; });

describe("products", () => {
  it("lists products and lets an owner edit prices in rupees, sent as exact cents", async () => {
    const fake = mockApi({ ...who(), [`GET ${S}/products`]: () => list(product()), [`PATCH ${S}/products/p1`]: () => product() });
    renderScreen(<ProductsScreen />);
    expect(await screen.findByText("Samba Rice")).toBeInTheDocument();
    expect(screen.getByText("Rs 1,250.00")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Edit Samba Rice" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Customers pay Rs 1,435.00 online.")).toBeInTheDocument();
    const price = within(dialog).getByLabelText("Shop price (Rs)");
    await userEvent.clear(price);
    await userEvent.type(price, "1,199.90");
    await userEvent.clear(within(dialog).getByLabelText("Stock count (empty = not counted)"));
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, `PATCH ${S}/products/p1`)).toHaveLength(1));
    expect(calls(fake, `PATCH ${S}/products/p1`)[0]![1].body).toEqual({ shopPriceCents: 119990, mrpCents: 150000, stockQty: null, isAvailable: true, maxPerOrder: 20 });
  });

  it("does not send a price that is not money", async () => {
    const fake = mockApi({ ...who(), [`GET ${S}/products`]: () => list(product()) });
    renderScreen(<ProductsScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Edit Samba Rice" }));
    const dialog = screen.getByRole("dialog");
    const price = within(dialog).getByLabelText("Shop price (Rs)");
    await userEvent.clear(price);
    await userEvent.type(price, "12.345");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByText("Enter rupees, like 1435 or 1435.50.")).toBeInTheDocument();
    expect(calls(fake, `PATCH ${S}/products/p1`)).toHaveLength(0);
  });

  it("lets a connector-managed product change only the order limit", async () => {
    const fake = mockApi({ ...who(), [`GET ${S}/products`]: () => list(product({ managedBy: "CONNECTOR" })), [`PATCH ${S}/products/p1`]: () => product() });
    renderScreen(<ProductsScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Edit Samba Rice" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByLabelText("Shop price (Rs)")).toBeDisabled();
    expect(within(dialog).getByLabelText("On sale")).toBeDisabled();
    const max = within(dialog).getByLabelText("Max per order");
    await userEvent.clear(max);
    await userEvent.type(max, "8");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, `PATCH ${S}/products/p1`)[0]![1].body).toEqual({ maxPerOrder: 8 }));
  });

  it("shows no edit button to staff without the right, and does to staff with it", async () => {
    mockApi({ ...who(staff(false)), [`GET ${S}/products`]: () => list(product()) });
    const first = renderScreen(<ProductsScreen />);
    await screen.findByText("Samba Rice");
    expect(screen.queryByRole("button", { name: /Edit/ })).not.toBeInTheDocument();
    first.unmount();
    mockApi({ ...who(staff(true)), [`GET ${S}/products`]: () => list(product()) });
    renderScreen(<ProductsScreen />);
    expect(await screen.findByRole("button", { name: "Edit Samba Rice" })).toBeInTheDocument();
  });

  it("searches while typing and filters to what is on sale", async () => {
    const fake = mockApi({ ...who(), [`GET ${S}/products`]: () => list(product()) });
    renderScreen(<ProductsScreen />);
    await screen.findByText("Samba Rice");
    await userEvent.type(screen.getByLabelText("Search products"), "rice");
    await userEvent.click(screen.getByLabelText("Only available"));
    await waitFor(() => expect(calls(fake, `GET ${S}/products`).some(([, o]) => o.query?.q === "rice" && o.query?.available === true)).toBe(true));
  });
});

describe("team", () => {
  const team = { members: [
    { id: "m1", userId: "me1", name: "Owner Person", email: "owner@example.com", role: "SHOP_OWNER", canEditProducts: false },
    { id: "m2", userId: "u2", name: "Sunil", email: "sunil@example.com", role: "SHOP_STAFF", canEditProducts: false },
  ], invites: [{ id: "i1", email: "waiting@example.com", role: "SHOP_STAFF", canEditProducts: false, expiresAt: "2026-11-08T00:00:00Z" }] };

  beforeEach(() => { path = "/console/shop/s1/team/"; });

  it("lists members and waiting invites; the owner cannot be removed", async () => {
    mockApi({ ...who(), [`GET ${S}/team`]: () => team });
    renderScreen(<TeamScreen />);
    expect(await screen.findByText("Sunil")).toBeInTheDocument();
    expect(screen.getByText("waiting@example.com")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Remove" })).toHaveLength(1);
  });

  it("makes an invite and shows the link once", async () => {
    const token = "A".repeat(43);
    const fake = mockApi({ ...who(), [`GET ${S}/team`]: () => team, [`POST ${S}/team/invites`]: () => ({ invite: { id: "i2" }, token }) });
    renderScreen(<TeamScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Invite someone" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Their Google e-mail"), "new@example.com");
    await userEvent.click(within(dialog).getByLabelText("Can edit products"));
    await userEvent.click(within(dialog).getByRole("button", { name: "Create invite link" }));
    const link = await screen.findByLabelText("Invite link");
    expect(link).toHaveValue(`${window.location.origin}/?invite=${token}`);
    expect(calls(fake, `POST ${S}/team/invites`)[0]![1].body).toEqual({ email: "new@example.com", role: "SHOP_STAFF", canEditProducts: true });
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByDisplayValue(new RegExp(token))).not.toBeInTheDocument();
  });

  it("removes a member after asking", async () => {
    const fake = mockApi({ ...who(), [`GET ${S}/team`]: () => team, [`DELETE ${S}/team/m2`]: () => undefined });
    renderScreen(<TeamScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Remove" }));
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(calls(fake, `DELETE ${S}/team/m2`)).toHaveLength(1));
  });

  it("is not for staff", async () => {
    mockApi({ ...who(staff()), [`GET ${S}/team`]: () => new ApiError(403, "FORBIDDEN", "No") });
    renderScreen(<TeamScreen />);
    expect(await screen.findByText("This page isn't for your account")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Team" })).not.toBeInTheDocument();
  });
});

describe("dashboard and payouts", () => {
  const dash = (full = true) => ({
    from: "2026-10-26", to: "2026-11-01", queue: { SUBMITTED: 2, AWAITING_PAYMENT: 1, PAID: 3, OUT_FOR_DELIVERY: 1 }, avgReviewSeconds: 180, avgPickSeconds: 600,
    ...(full ? { full: { salesCents: 1000000, orders: 4, delivered: 3, avgBillCents: 250000, cancelRateBps: 1250, shopItemsCents: 900000, discountCents: 0, marginCents: 100000, payoutDueCents: 800000,
      days: [{ day: "2026-10-31", salesCents: 400000, orders: 2 }, { day: "2026-11-01", salesCents: 600000, orders: 2 }], topProducts: [{ productId: "g1", name: "Samba Rice", quantity: "6", salesCents: 800000 }] } } : {}),
  });

  it("shows the queue to staff but no sales", async () => {
    path = "/console/shop/s1/dashboard/";
    mockApi({ ...who(staff()), [`GET ${S}/dashboard`]: () => dash(false) });
    renderScreen(<DashboardScreen />);
    expect(await screen.findByText("3 min")).toBeInTheDocument(); // average review
    expect(screen.getByText("10 min")).toBeInTheDocument();
    expect(screen.queryByText("Sales")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Payouts" })).not.toBeInTheDocument();
  });

  it("shows the owner sales, the chart, top products, and asks for the chosen range", async () => {
    path = "/console/shop/s1/dashboard/";
    const fake = mockApi({ ...who(), [`GET ${S}/dashboard`]: () => dash() });
    renderScreen(<DashboardScreen />);
    expect(await screen.findByText("Rs 10,000")).toBeInTheDocument();
    expect(screen.getByText("12.5%")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Sales per day" })).toBeInTheDocument();
    expect(screen.getByText("Samba Rice")).toBeInTheDocument();
    const q = calls(fake, `GET ${S}/dashboard`)[0]![1].query as { from: string; to: string };
    expect(q.to >= q.from).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "30 days" }));
    await waitFor(() => expect(calls(fake, `GET ${S}/dashboard`)).toHaveLength(2));
  });

  it("shows the balance and the payouts to the owner", async () => {
    path = "/console/shop/s1/payouts/";
    mockApi({ ...who(), [`GET ${S}/payouts`]: () => ({ balanceCents: 800000, data: [{ id: "x1", shopId: "s1", periodFrom: "2026-10-01T00:00:00Z", periodTo: "2026-10-15T00:00:00Z", amountCents: 500000, status: "PAID", reference: "BOC-123", paidAt: "2026-10-16T00:00:00Z", createdAt: "2026-10-16T00:00:00Z" }] }) });
    renderScreen(<PayoutsScreen />);
    expect(await screen.findByText("Rs 8,000.00")).toBeInTheDocument();
    expect(screen.getByText("Rs 5,000.00")).toBeInTheDocument();
    expect(screen.getByText("BOC-123")).toBeInTheDocument();
  });
});
