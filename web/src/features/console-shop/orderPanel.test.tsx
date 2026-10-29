import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { owner, shopOrder, staff } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { ConsoleOrderScreen } from "./ConsoleOrderScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/shop/s1/order/o1/", useRouter: () => ({ push: vi.fn() }) }));

const O = "/console/shops/s1/orders/o1";
const base = (order = shopOrder(), roles = owner()) => ({ "GET /bootstrap": () => bootstrap(me({ roles })), [`GET ${O}`]: () => order });
const buttons = () => screen.queryAllByRole("button").map((b) => b.textContent ?? "");

describe("review panel: buttons come from allowedActions", () => {
  it("shows approve, change and decline on a new bill, and the lines and totals", async () => {
    mockApi(base());
    renderScreen(<ConsoleOrderScreen />);
    expect(await screen.findByRole("button", { name: "Approve bill" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Change bill" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Decline" })).toBeInTheDocument();
    expect(screen.getByText(/Samba Rice 5kg/)).toBeInTheDocument();
    expect(screen.getByText("Nimal Perera")).toBeInTheDocument();
    expect(screen.getByText("077•••4567")).toBeInTheDocument(); // masked, no call link yet
    expect(screen.queryByRole("link", { name: /077/ })).not.toBeInTheDocument();
  });

  it("shows nothing to do when the server lists nothing, whatever the status", async () => {
    mockApi(base(shopOrder({ allowedActions: [] })));
    renderScreen(<ConsoleOrderScreen />);
    await screen.findByText("Nimal Perera");
    expect(buttons().filter((b) => /Approve|Change|Decline|Start|Mark|Send|Assign|Cancel|Refund/.test(b))).toEqual([]);
  });

  it("approves with the version the shop saw", async () => {
    const fake = mockApi({ ...base(), [`POST ${O}/review`]: () => shopOrder({ status: "AWAITING_PAYMENT", allowedActions: [], version: 2 }) });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Approve bill" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/review`)).toHaveLength(1));
    expect(calls(fake, `POST ${O}/review`)[0]![1].body).toEqual({ version: 1, action: "APPROVE" });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Approve bill" })).not.toBeInTheDocument());
  });

  it("declines only with a reason of three letters or more", async () => {
    const fake = mockApi({ ...base(), [`POST ${O}/review`]: () => shopOrder({ status: "REJECTED", allowedActions: [] }) });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Decline" }));
    const dialog = screen.getByRole("dialog");
    const decline = within(dialog).getByRole("button", { name: "Decline" });
    expect(decline).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText("Note for the customer"), "ab");
    expect(decline).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText("Note for the customer"), "c");
    await userEvent.click(decline);
    await waitFor(() => expect(calls(fake, `POST ${O}/review`)[0]![1].body).toEqual({ version: 1, action: "REJECT", reason: "abc" }));
  });

  it("shows a stale version as the API's sentence", async () => {
    mockApi({ ...base(), [`POST ${O}/review`]: () => new ApiError(409, "STALE_ORDER", "This order changed. Reload and try again.") });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Approve bill" }));
    expect(await screen.findByText("This order changed. Reload and try again.")).toBeInTheDocument();
  });
});

describe("changing the bill", () => {
  it("sends a quantity change and a removal, and refuses to remove every line", async () => {
    const fake = mockApi({ ...base(), [`POST ${O}/review`]: () => shopOrder({ status: "REVISED", allowedActions: [], version: 2 }) });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Change bill" }));
    const dialog = screen.getByRole("dialog");
    const send = within(dialog).getByRole("button", { name: "Send changed bill" });
    expect(send).toBeDisabled(); // nothing changed yet
    const removes = within(dialog).getAllByRole("button", { name: "Remove" });
    await userEvent.click(removes[0]!);
    await userEvent.click(within(dialog).getAllByRole("button", { name: "Remove" })[0]!); // the second line is now first
    expect(within(dialog).getByText("You cannot remove every line. Decline the bill instead.")).toBeInTheDocument();
    expect(send).toBeDisabled();
    await userEvent.click(within(dialog).getAllByRole("button", { name: "Keep it" })[1]!);
    await userEvent.click(within(dialog).getByRole("button", { name: "Send changed bill" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/review`)).toHaveLength(1));
    expect(calls(fake, `POST ${O}/review`)[0]![1].body).toEqual({ version: 1, action: "REVISE", changes: [{ itemId: "l1", remove: true }] });
  });

  it("sends another item for a line, picked from the shop's own products", async () => {
    const fake = mockApi({
      ...base(),
      "GET /console/shops/s1/products": () => ({ data: [{ id: "p9", productId: "g9", name: "Nipuna Samba 5kg", brand: null, sizeLabel: "5kg", barcode: null, imageUrl: null, shopPriceCents: 120000, mrpCents: null, onlinePriceCents: 130000, stockQty: 5, isAvailable: true, maxPerOrder: 10, managedBy: "MANUAL", updatedAt: "2026-11-01T00:00:00Z" }], nextCursor: "" }),
      [`POST ${O}/review`]: () => shopOrder({ status: "REVISED", allowedActions: [] }),
    });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Change bill" }));
    await userEvent.click(within(screen.getByRole("dialog")).getAllByRole("button", { name: "Send other item" })[0]!);
    await userEvent.type(screen.getByLabelText("Search your products"), "nipuna");
    await userEvent.click(await screen.findByRole("button", { name: /Nipuna Samba 5kg/ }));
    const editor = screen.getByRole("dialog");
    await userEvent.click(within(editor).getByRole("button", { name: "Send changed bill" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/review`)).toHaveLength(1));
    expect(calls(fake, `POST ${O}/review`)[0]![1].body).toEqual({ version: 1, action: "REVISE", changes: [{ itemId: "l1", substitute: { shopProductId: "p9", qty: 2 } }] });
  });
});

describe("picking and sending out", () => {
  const picking = (extra = {}) => shopOrder({ status: "PICKING", version: 5, allowedActions: ["MARK_PACKED", "REQUEST_PARTIAL_REFUND"], customer: { name: "Nimal Perera", phone: "+94771234567" }, ...extra });

  it("shows the pick list, a call link with a clean number, and the next step", async () => {
    const fake = mockApi({ ...base(picking()), [`POST ${O}/status`]: () => picking({ status: "PACKED", allowedActions: [] }) });
    renderScreen(<ConsoleOrderScreen />);
    expect(await screen.findByText("Pick list (0 of 2)")).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText("2 × Samba Rice 5kg"));
    expect(screen.getByText("Pick list (1 of 2)")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /\+94771234567/ })).toHaveAttribute("href", "tel:+94771234567");
    await userEvent.click(screen.getByRole("button", { name: "Mark packed" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/status`)[0]![1].body).toEqual({ version: 5, to: "PACKED" }));
  });

  it("assigns a driver of the shop, then sends the order out", async () => {
    const order = shopOrder({ status: "PACKED", version: 7, allowedActions: ["ASSIGN_DRIVER", "SEND_OUT"] });
    const fake = mockApi({
      ...base(order), "GET /console/shops/s1/drivers": () => ({ data: [{ id: "d1", name: "Kamal" }] }),
      [`POST ${O}/driver`]: () => ({ ...order, version: 8 }), [`POST ${O}/status`]: () => ({ ...order, status: "OUT_FOR_DELIVERY", allowedActions: [] }),
    });
    renderScreen(<ConsoleOrderScreen />);
    await screen.findByRole("option", { name: "Kamal" });
    await userEvent.selectOptions(screen.getByLabelText("Driver"), "d1");
    await userEvent.click(screen.getByRole("button", { name: "Assign" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/driver`)[0]![1].body).toEqual({ version: 7, driverId: "d1" }));
    await userEvent.click(await screen.findByRole("button", { name: "Send out for delivery" }));
    await waitFor(() => expect(calls(fake, `POST ${O}/status`)[0]![1].body).toEqual({ version: 8, to: "OUT_FOR_DELIVERY" }));
  });

  it("asks for a part refund with the line and a reason", async () => {
    const fake = mockApi({ ...base(picking()), [`POST ${O}/refund-requests`]: () => ({ id: "r1" }) });
    renderScreen(<ConsoleOrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Refund one item" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.selectOptions(within(dialog).getByLabelText("Item"), "l2");
    await userEvent.type(within(dialog).getByLabelText("Note for the customer"), "Out of stock");
    await userEvent.click(within(dialog).getByRole("button", { name: "Ask for refund" }));
    expect(await screen.findByText(/Sent to the platform/)).toBeInTheDocument();
    expect(calls(fake, `POST ${O}/refund-requests`)[0]![1].body).toEqual({ type: "PARTIAL", reason: "Out of stock", orderItemId: "l2" });
  });

  it("shows the order panel for staff too, but nobody else's order", async () => {
    mockApi({ ...base(shopOrder(), staff()), [`GET ${O}`]: () => new ApiError(404, "NOT_FOUND", "Not found") });
    renderScreen(<ConsoleOrderScreen />);
    expect(await screen.findByText("We can't find that order")).toBeInTheDocument();
  });
});
