import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CartError, cartApi, type Cart } from "@/lib/cartApi";
import { readCartRef, writeCartRef } from "@/lib/cartStore";
import { renderScreen } from "@/test/support";
import { otherShop, krishna } from "@/test/fixtures";
import { cartInfo } from "@/features/shop/cartInfo";
import { useCart } from "./cartContext";

vi.mock("@/lib/cartApi", async (orig) => ({
  ...(await orig<typeof import("@/lib/cartApi")>()),
  cartApi: { create: vi.fn(), get: vi.fn(), addLine: vi.fn(), setQuantity: vi.fn(), discard: vi.fn() },
}));
vi.mock("@/lib/api", () => ({ api: vi.fn() }));

const api = vi.mocked(cartApi);
const A = cartInfo(krishna)!;
const B = cartInfo(otherShop)!;

const cartWith = (id: string, itemId?: string, quantity = 1): Cart => ({
  id, status: "open", subtotal: itemId ? quantity * 1000 : 0, itemCount: itemId ? quantity : 0,
  lines: itemId ? [{ id: "ln1", itemId, name: "Samba Rice", unitPrice: 1000, quantity, lineTotal: quantity * 1000 }] : [],
});

function Probe({ shop, itemId }: { shop: typeof A; itemId: string }) {
  const cart = useCart();
  return (
    <>
      <button onClick={() => cart.setQty(shop, itemId, cart.qtyOf(itemId) + 1)}>add {shop.name}</button>
      <output data-testid={`qty-${shop.slug}`}>{cart.qtyOf(itemId)}</output>
      <output data-testid="count">{cart.count}</output>
    </>
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  writeCartRef(null);
  api.create.mockResolvedValue(cartWith("c-new"));
  api.discard.mockResolvedValue(undefined);
});

const refA = { shopId: "s1", shopSlug: "krishna-super", shopName: "Krishna Super", tenantId: "ten_k", publicKey: "pk_k", cartId: "c-old" };

describe("cart quantity taps", () => {
  it("shows taps at once and sends one call for the whole burst", async () => {
    api.addLine.mockResolvedValue(cartWith("c-new", "p1", 3));
    renderScreen(<Probe shop={A} itemId="p1" />);
    const add = screen.getByRole("button", { name: "add Krishna Super" });
    fireEvent.click(add); fireEvent.click(add); fireEvent.click(add);
    expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("3");
    expect(api.create).not.toHaveBeenCalled(); // nothing is sent before the pause
    await waitFor(() => expect(api.addLine).toHaveBeenCalledTimes(1));
    expect(api.create).toHaveBeenCalledWith("pk_k");
    expect(api.addLine).toHaveBeenCalledWith("pk_k", "c-new", "p1", 3);
    await waitFor(() => expect(screen.getByTestId("count")).toHaveTextContent("3"));
    expect(readCartRef()?.cartId).toBe("c-new");
  });

  it("changes an existing line instead of adding it twice", async () => {
    writeCartRef(refA);
    api.get.mockResolvedValue(cartWith("c-old", "p1", 2));
    api.setQuantity.mockResolvedValue(cartWith("c-old", "p1", 3));
    renderScreen(<Probe shop={A} itemId="p1" />);
    await waitFor(() => expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("2"));
    await userEvent.click(screen.getByRole("button", { name: "add Krishna Super" }));
    await waitFor(() => expect(api.setQuantity).toHaveBeenCalledWith("pk_k", "c-old", "ln1", 3));
    expect(api.addLine).not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
  });

  it("rolls the number back and says so when the cart service fails", async () => {
    api.addLine.mockRejectedValue(new CartError(502, "UNKNOWN"));
    renderScreen(<Probe shop={A} itemId="p1" />);
    await userEvent.click(screen.getByRole("button", { name: "add Krishna Super" }));
    expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("1");
    expect(await screen.findByText("Can't update cart, try again.")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("0"));
  });

  it("forgets a cart that has expired", async () => {
    writeCartRef(refA);
    api.get.mockRejectedValue(new CartError(404, "CART_NOT_FOUND"));
    renderScreen(<Probe shop={A} itemId="p1" />);
    await waitFor(() => expect(readCartRef()).toBeNull());
  });
});

describe("switching shop", () => {
  beforeEach(() => {
    writeCartRef(refA);
    api.get.mockResolvedValue(cartWith("c-old", "p1", 1));
  });

  it("asks first, and keeps the cart when the person says no", async () => {
    renderScreen(<><Probe shop={A} itemId="p1" /><Probe shop={B} itemId="q1" /></>);
    await waitFor(() => expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("1"));
    await userEvent.click(screen.getByRole("button", { name: "add Ceylon Mart" }));
    expect(await screen.findByText("Your cart has items from Krishna Super. Clear it and start a new cart with Ceylon Mart?")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await act(async () => { await new Promise((r) => setTimeout(r, 500)); });
    expect(api.create).not.toHaveBeenCalled();
    expect(api.discard).not.toHaveBeenCalled();
    expect(readCartRef()?.cartId).toBe("c-old");
  });

  it("clears the old cart and starts a new one when the person agrees", async () => {
    api.addLine.mockResolvedValue(cartWith("c-new", "q1", 1));
    renderScreen(<><Probe shop={A} itemId="p1" /><Probe shop={B} itemId="q1" /></>);
    await waitFor(() => expect(screen.getByTestId("qty-krishna-super")).toHaveTextContent("1"));
    await userEvent.click(screen.getByRole("button", { name: "add Ceylon Mart" }));
    await userEvent.click(await screen.findByRole("button", { name: "Start new cart" }));
    await waitFor(() => expect(api.addLine).toHaveBeenCalledWith("pk_c", "c-new", "q1", 1));
    expect(api.discard).toHaveBeenCalledWith("pk_k", "c-old");
    expect(api.create).toHaveBeenCalledWith("pk_c");
    expect(readCartRef()).toMatchObject({ shopSlug: "ceylon-mart", cartId: "c-new" });
  });

  it("does not ask when the old cart is empty", async () => {
    api.get.mockResolvedValue(cartWith("c-old"));
    api.addLine.mockResolvedValue(cartWith("c-new", "q1", 1));
    renderScreen(<Probe shop={B} itemId="q1" />);
    await userEvent.click(screen.getByRole("button", { name: "add Ceylon Mart" }));
    await waitFor(() => expect(api.addLine).toHaveBeenCalled());
    expect(screen.queryByText("Start a new cart?")).not.toBeInTheDocument();
    expect(readCartRef()?.shopSlug).toBe("ceylon-mart");
  });
});
