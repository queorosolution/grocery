import { screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { cartApi } from "@/lib/cartApi";
import { readCartRef, writeCartRef } from "@/lib/cartStore";
import { krishna, order } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { CheckoutScreen } from "./CheckoutScreen";
import type { Quote } from "./types";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("@/features/auth/AuthProvider", () => ({ useAuth: () => ({ openSignIn: vi.fn() }) }));
const push = vi.fn();
const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, replace }), usePathname: () => "/checkout/" }));
vi.mock("@/features/auth/PhoneVerify", () => ({
  PhoneVerify: ({ onVerified }: { onVerified: (m: unknown) => void }) => <button onClick={() => onVerified({ ...me(), phoneVerified: true })}>Fake verify</button>,
}));
vi.mock("@/lib/cartApi", async (orig) => ({
  ...(await orig<typeof import("@/lib/cartApi")>()),
  cartApi: { create: vi.fn(), addLine: vi.fn(), setQuantity: vi.fn(), discard: vi.fn().mockResolvedValue(undefined), get: vi.fn() },
}));

const home = { id: "a1", label: "Home", line1: "12 Galle Road", landmark: null, lat: 6.9, lng: 79.86, isDefault: true };
const work = { id: "a2", label: "Work", line1: "5 Union Place", landmark: null, lat: 6.92, lng: 79.85, isDefault: false };
const quote = (over: Partial<Quote> = {}): Quote => ({
  lines: [{ shopProductId: "p1", name: "Samba Rice 5kg", unit: "PCS", qty: 1, unitPriceCents: 143500, lineCents: 143500, status: "OK", priceChanged: false, subPref: "SIMILAR" }],
  excluded: [], totals: { itemsCents: 143500, deliveryFeeCents: 15000, serviceFeeCents: 0, discountCents: 0, totalCents: 158500 },
  delivery: { distanceKm: 2.4, weightKg: 5, feeRule: "base" },
  slots: [{ id: "sl1", startsAt: "2026-11-02T03:00:00Z", endsAt: "2026-11-02T05:00:00Z", remaining: 4 }, { id: "sl2", startsAt: "2026-11-02T05:00:00Z", endsAt: "2026-11-02T07:00:00Z", remaining: 0 }],
  checks: { shopOpen: true, inRange: true, minOrderMet: true, phoneVerified: true }, ...over,
});

function setup(handlers: Record<string, (b: unknown) => unknown> = {}, who = me()) {
  const fake = mockApi({
    "GET /bootstrap": () => bootstrap(who),
    "GET /shops/krishna-super": () => krishna,
    "GET /me/addresses": () => ({ data: [home, work] }),
    "POST /bills/quote": () => quote(),
    "POST /bills": () => order({ id: "o1" }),
    ...handlers,
  });
  renderScreen(<CheckoutScreen />);
  return fake;
}
const quotes = (fake: ReturnType<typeof setup>) => calls(fake, "POST /bills/quote");
const sent = (fake: ReturnType<typeof setup>) => fake.mock.calls.filter(([p, o]) => p === "/bills" && o?.method === "POST") as unknown as [string, { body: Record<string, unknown>; idempotencyKey: string }][];

beforeEach(() => {
  push.mockReset();
  replace.mockReset();
  writeCartRef({ shopId: "s1", shopSlug: "krishna-super", shopName: "Krishna Super", tenantId: "ten_k", publicKey: "pk_k", cartId: "c1" });
  vi.mocked(cartApi.get).mockResolvedValue({
    id: "c1", status: "open", subtotal: 143500, itemCount: 1,
    lines: [{ id: "ln1", itemId: "p1", name: "Samba Rice 5kg", unitPrice: 143500, quantity: 1, lineTotal: 143500 }],
  });
});

describe("CheckoutScreen", () => {
  it("asks for the bill once, to the main address, and needs a delivery window", async () => {
    const fake = setup();
    expect(await screen.findByText("Your bill")).toBeInTheDocument();
    expect(quotes(fake)).toHaveLength(1);
    expect(quotes(fake)[0]![1].body).toEqual({ shopId: "s1", cartId: "c1", addressId: "a1", lines: [] });
    expect(screen.getByText("Choose a delivery window.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send to shop" })).toBeDisabled();
    expect(screen.getByLabelText(/Full/)).toBeDisabled();
  });

  it("asks again for a new window or address, but not for a substitution choice", async () => {
    const fake = setup();
    await screen.findByText("Your bill");
    await userEvent.click(screen.getByLabelText(/4 left/));
    await waitFor(() => expect(quotes(fake)).toHaveLength(2));
    expect(quotes(fake)[1]![1].body).toMatchObject({ addressId: "a1", slotId: "sl1" });
    await userEvent.selectOptions(screen.getByLabelText("If Samba Rice 5kg is out of stock"), "CALL_ME");
    await userEvent.click(screen.getByLabelText(/Work/));
    await waitFor(() => expect(quotes(fake)).toHaveLength(3));
    expect(quotes(fake)[2]![1].body).toMatchObject({ addressId: "a2", slotId: "sl1", lines: [{ itemId: "p1", subPref: "CALL_ME" }] });
  });

  it("sends the bill with a key, then empties the cart and opens the order", async () => {
    const fake = setup();
    await userEvent.click(await screen.findByLabelText(/4 left/));
    await userEvent.selectOptions(screen.getByLabelText("If Samba Rice 5kg is out of stock"), "REMOVE");
    await userEvent.type(screen.getByLabelText("Note for the shop (optional)"), "  call at gate ");
    await waitFor(() => expect(screen.getByRole("button", { name: "Send to shop" })).toBeEnabled());
    await userEvent.click(screen.getByRole("button", { name: "Send to shop" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/order/o1"));
    const [, o] = sent(fake)[0]!;
    expect(o.body).toEqual({ shopId: "s1", cartId: "c1", addressId: "a1", slotId: "sl1", lines: [{ itemId: "p1", subPref: "REMOVE" }], note: "call at gate" });
    expect(o.idempotencyKey).toMatch(/^[A-Za-z0-9_-]{16,64}$/);
    expect(readCartRef()).toBeNull();
  });

  it("reuses the key when the same bill is sent again, and takes a new one when it changed", async () => {
    let fail = true;
    const fake = setup({ "POST /bills": () => (fail ? new ApiError(0, "NETWORK", "Check your connection and try again.") : order({ id: "o1" })) });
    await userEvent.click(await screen.findByLabelText(/4 left/));
    const send = screen.getByRole("button", { name: "Send to shop" });
    await waitFor(() => expect(send).toBeEnabled());
    await userEvent.click(send);
    expect(await screen.findByText("Check your connection and try again.")).toBeInTheDocument();
    await userEvent.click(send);
    await waitFor(() => expect(sent(fake)).toHaveLength(2));
    expect(sent(fake)[1]![1].idempotencyKey).toBe(sent(fake)[0]![1].idempotencyKey);
    await userEvent.type(screen.getByLabelText("Note for the shop (optional)"), "x");
    fail = false;
    await userEvent.click(send);
    await waitFor(() => expect(sent(fake)).toHaveLength(3));
    expect(sent(fake)[2]![1].idempotencyKey).not.toBe(sent(fake)[0]![1].idempotencyKey);
  });
});

describe("deliver to another place", () => {
  it("sends a one-time destination instead of an address, and saves nothing", async () => {
    const fake = setup();
    await screen.findByText("Your bill");
    await userEvent.click(screen.getByLabelText(/Another place/));
    await userEvent.type(screen.getByLabelText("Street address"), "8 Lake Drive, Colombo 08");
    await userEvent.type(screen.getByLabelText("Landmark (optional)"), "Blue gate");
    await userEvent.type(screen.getByLabelText("Name this place (optional)"), "Mum's house");
    fireEvent.change(screen.getByLabelText("Latitude"), { target: { value: "6.93" } });
    fireEvent.change(screen.getByLabelText("Longitude"), { target: { value: "79.87" } });
    const place = { destination: { line1: "8 Lake Drive, Colombo 08", lat: 6.93, lng: 79.87, label: "Mum's house", landmark: "Blue gate" } };
    await waitFor(() => expect(quotes(fake).at(-1)![1].body).toMatchObject(place));
    expect(quotes(fake).at(-1)![1].body).not.toHaveProperty("addressId");
    // While the place is incomplete nothing was asked for it.
    expect(quotes(fake).filter(([, o]) => "destination" in o.body && !(o.body.destination as { lat?: number }).lat)).toHaveLength(0);

    await userEvent.click(await screen.findByLabelText(/4 left/));
    await waitFor(() => expect(screen.getByRole("button", { name: "Send to shop" })).toBeEnabled());
    await userEvent.click(screen.getByRole("button", { name: "Send to shop" }));
    await waitFor(() => expect(sent(fake)).toHaveLength(1));
    expect(sent(fake)[0]![1].body).toMatchObject({ ...place, slotId: "sl1" });
    expect(sent(fake)[0]![1].body).not.toHaveProperty("addressId");
    expect(calls(fake, "POST /me/addresses")).toHaveLength(0);
  });

  it("is the way in for a person with no saved address", async () => {
    setup({ "GET /me/addresses": () => ({ data: [] }) });
    expect(await screen.findByLabelText("Street address")).toBeInTheDocument();
    expect(screen.getByLabelText(/Another place/)).toBeChecked();
  });
});

describe("what blocks sending", () => {
  it("says why when the place is too far", async () => {
    setup({ "POST /bills/quote": () => quote({ checks: { shopOpen: true, inRange: false, minOrderMet: true, phoneVerified: true } }) });
    expect(await screen.findByText("This place is too far for this shop. Pick another place.")).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText(/4 left/));
    expect(screen.getByRole("button", { name: "Send to shop" })).toBeDisabled();
  });

  it("lists items that cannot be sold and blocks sending", async () => {
    setup({ "POST /bills/quote": () => quote({ excluded: [{ shopProductId: "p9", name: "Fresh Milk", reason: "ITEM_UNAVAILABLE" }] }) });
    expect(await screen.findByText("Fresh Milk")).toBeInTheDocument();
    expect(screen.getByText("Remove the unavailable items from your cart.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send to shop" })).toBeDisabled();
  });

  it("asks an unverified person to verify the phone, then asks for the bill again", async () => {
    let verified = false;
    const fake = setup({ "POST /bills/quote": () => quote({ checks: { shopOpen: true, inRange: true, minOrderMet: true, phoneVerified: verified } }) });
    await userEvent.click(await screen.findByLabelText(/4 left/));
    expect(await screen.findByText("Verify your phone")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send to shop" })).toBeDisabled();
    verified = true;
    await userEvent.click(screen.getByRole("button", { name: "Fake verify" }));
    await waitFor(() => expect(quotes(fake)).toHaveLength(3)); // open, slot, after verify
    await waitFor(() => expect(screen.getByRole("button", { name: "Send to shop" })).toBeEnabled());
    expect(screen.queryByText("Verify your phone")).not.toBeInTheDocument();
  });
});

describe("who may be here", () => {
  it("shows an empty cart as empty", async () => {
    writeCartRef(null);
    setup();
    expect(await screen.findByText("Your cart is empty")).toBeInTheDocument();
  });

  it("sends a person who is not set up yet to onboarding first", async () => {
    setup({}, me({ needsOnboarding: true }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/onboarding?next=%2Fcheckout"));
  });
});
