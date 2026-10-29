import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { submitPayHere } from "@/lib/payhere";
import { order } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { OrderScreen } from "./OrderScreen";
import { OrdersScreen } from "./OrdersScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("@/features/auth/AuthProvider", () => ({ useAuth: () => ({ openSignIn: vi.fn() }) }));
vi.mock("next/navigation", () => ({ usePathname: () => "/order/o1/", useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }));
vi.mock("@/lib/payhere", () => ({ submitPayHere: vi.fn() }));

const signedIn = { "GET /bootstrap": () => bootstrap(me()) };
const names = () => screen.queryAllByRole("button").map((b) => b.textContent);

beforeEach(() => {
  vi.mocked(submitPayHere).mockReset();
});

describe("OrderScreen buttons come from allowedActions", () => {
  it("shows pay, coupon and cancel when the server lists them", async () => {
    mockApi({ ...signedIn, "GET /orders/o1": () => order() });
    renderScreen(<OrderScreen />);
    expect(await screen.findByRole("heading", { name: "KS-7Q2M9X" })).toBeInTheDocument();
    expect(screen.getAllByText("Ready to pay").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Pay now" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel order" })).toBeInTheDocument();
    expect(screen.getByLabelText("Coupon code")).toBeInTheDocument();
    expect(screen.getByText("Rs 1,585.00")).toBeInTheDocument();
  });

  it("shows none of them for the same status when the server lists nothing", async () => {
    mockApi({ ...signedIn, "GET /orders/o1": () => order({ allowedActions: [] }) });
    renderScreen(<OrderScreen />);
    await screen.findByRole("heading", { name: "KS-7Q2M9X" });
    expect(screen.queryByRole("button", { name: "Pay now" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel order" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Coupon code")).not.toBeInTheDocument();
    expect(names().filter((n) => n && !/Account|menu/i.test(n))).toEqual([]);
  });

  it("cancels after a confirmation, with the version the person saw", async () => {
    const fake = mockApi({
      ...signedIn, "GET /orders/o1": () => order(),
      "POST /orders/o1/cancel": () => order({ status: "CANCELLED", allowedActions: [], version: 5 }),
    });
    renderScreen(<OrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Cancel order" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel order" }));
    await waitFor(() => expect(calls(fake, "POST /orders/o1/cancel")).toHaveLength(1));
    expect(calls(fake, "POST /orders/o1/cancel")[0]![1].body).toEqual({ version: 4 });
    expect((await screen.findAllByText("Cancelled")).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Pay now" })).not.toBeInTheDocument();
  });

  it("says when the order is not there (or not theirs)", async () => {
    mockApi({ ...signedIn, "GET /orders/o1": () => new ApiError(404, "NOT_FOUND", "Not found") });
    renderScreen(<OrderScreen />);
    expect(await screen.findByText("We can't find that order")).toBeInTheDocument();
  });

  it("shows the delivery code only when the order carries one", async () => {
    mockApi({ ...signedIn, "GET /orders/o1": () => order({ status: "OUT_FOR_DELIVERY", deliveryCode: "482913", allowedActions: ["REPORT_PROBLEM"] }) });
    renderScreen(<OrderScreen />);
    expect(await screen.findByText("482913")).toBeInTheDocument();
  });
});

describe("revision", () => {
  it("compares old and new, and accepts with the revision number", async () => {
    const revised = order({
      status: "REVISED", revisionNo: 2, allowedActions: ["ACCEPT_REVISION", "CANCEL"],
      lines: [{ ...order().lines[0]!, qty: 2, lineCents: 287000 }],
      previousLines: [order().lines[0]!],
    });
    const fake = mockApi({ ...signedIn, "GET /orders/o1": () => revised, "POST /orders/o1/accept-revision": () => order({ version: 5 }) });
    renderScreen(<OrderScreen />);
    expect(await screen.findByText("The shop changed your bill")).toBeInTheDocument();
    expect(screen.getByText(/Before: Samba Rice 5kg × 1/)).toBeInTheDocument();
    expect(screen.getByText(/Now: Samba Rice 5kg × 2/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Accept changes" }));
    await waitFor(() => expect(calls(fake, "POST /orders/o1/accept-revision")).toHaveLength(1));
    expect(calls(fake, "POST /orders/o1/accept-revision")[0]![1].body).toEqual({ version: 4, revisionNo: 2 });
  });
});

describe("coupon", () => {
  it("applies a code, then shows it with a remove button", async () => {
    const withCoupon = order({ version: 5, coupon: { code: "WELCOME10", discountCents: 15000 }, allowedActions: ["PAY", "REMOVE_COUPON", "CANCEL"] });
    const fake = mockApi({ ...signedIn, "GET /orders/o1": () => order(), "POST /orders/o1/coupon": () => withCoupon });
    renderScreen(<OrderScreen />);
    await userEvent.type(await screen.findByLabelText("Coupon code"), "welcome10");
    await userEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(await screen.findByText("WELCOME10 applied, you save Rs 150.00")).toBeInTheDocument();
    expect(calls(fake, "POST /orders/o1/coupon")[0]![1].body).toEqual({ version: 4, code: "welcome10" });
    expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
  });

  it("shows the API's reason when the code is refused", async () => {
    mockApi({ ...signedIn, "GET /orders/o1": () => order(), "POST /orders/o1/coupon": () => new ApiError(400, "COUPON_INVALID", "That coupon code isn't valid.") });
    renderScreen(<OrderScreen />);
    await userEvent.type(await screen.findByLabelText("Coupon code"), "nope");
    await userEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(await screen.findByText("That coupon code isn't valid.")).toBeInTheDocument();
  });
});

describe("payment", () => {
  it("sends the person to PayHere with what the server signed", async () => {
    const fake = mockApi({
      ...signedIn, "GET /orders/o1": () => order(),
      "POST /orders/o1/payment": () => ({ provider: "PAYHERE", action: "https://sandbox.payhere.lk/pay/checkout", fields: { merchant_id: "1", hash: "x" } }),
    });
    renderScreen(<OrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Pay now" }));
    await waitFor(() => expect(submitPayHere).toHaveBeenCalledWith("https://sandbox.payhere.lk/pay/checkout", { merchant_id: "1", hash: "x" }));
    const [, o] = calls(fake, "POST /orders/o1/payment")[0]!;
    expect(o.body).toEqual({ version: 4 });
    expect((fake.mock.calls.find(([p]) => p === "/orders/o1/payment")![1] as { idempotencyKey: string }).idempotencyKey).toMatch(/^[A-Za-z0-9_-]{16,64}$/);
  });

  it("shows a failure when the payment form is refused", async () => {
    vi.mocked(submitPayHere).mockImplementation(() => { throw new Error("refused"); });
    mockApi({ ...signedIn, "GET /orders/o1": () => order(), "POST /orders/o1/payment": () => ({ provider: "PAYHERE", action: "https://evil.example/", fields: {} }) });
    renderScreen(<OrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Pay now" }));
    expect(await screen.findByText("We could not start the payment.")).toBeInTheDocument();
  });

  it("shows bank details for a manual payment", async () => {
    mockApi({
      ...signedIn, "GET /orders/o1": () => order({ payment: { provider: "MANUAL", status: "INITIATED" } }),
      "POST /orders/o1/payment": () => ({ provider: "MANUAL", bank: { bankName: "Bank of Ceylon", accountName: "Krishna Super", accountNumber: "1234567", branch: "Fort" }, reference: "KS-7Q2M9X", amountCents: 158500 }),
    });
    renderScreen(<OrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Pay now" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Bank of Ceylon")).toBeInTheDocument();
    expect(within(dialog).getByText("KS-7Q2M9X")).toBeInTheDocument();
    expect(within(dialog).getByText(/Transfer Rs 1,585\.00/)).toBeInTheDocument();
    expect(submitPayHere).not.toHaveBeenCalled();
  });
});

describe("report a problem", () => {
  it("sends the reason and note for a paid order", async () => {
    const fake = mockApi({
      ...signedIn, "GET /orders/o1": () => order({ status: "DELIVERED", allowedActions: ["REPORT_PROBLEM"] }),
      "POST /orders/o1/tickets": () => ({ id: "t1" }),
    });
    renderScreen(<OrderScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Report a problem" }));
    await userEvent.selectOptions(screen.getByLabelText("Reason"), "DAMAGED");
    await userEvent.type(screen.getByLabelText("Tell us more (optional)"), "Eggs broken");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText(/Thanks, we have it/)).toBeInTheDocument();
    expect(calls(fake, "POST /orders/o1/tickets")[0]![1].body).toEqual({ reason: "DAMAGED", note: "Eggs broken" });
  });
});

describe("OrdersScreen", () => {
  it("lists orders as links, newest first", async () => {
    mockApi({
      ...signedIn,
      "GET /orders": () => ({ data: [{ id: "o1", code: "KS-7Q2M9X", status: "PAID", shop: { slug: "k", name: "Krishna Super" }, totalCents: 158500, itemCount: 3, createdAt: "2026-11-01T04:00:00Z" }], nextCursor: null }),
    });
    renderScreen(<OrdersScreen />);
    const link = await screen.findByRole("link", { name: /Krishna Super/ });
    expect(link).toHaveAttribute("href", "/order/o1");
    expect(link).toHaveTextContent("Rs 1,585.00");
    expect(link).toHaveTextContent("Paid");
  });
  it("has a friendly empty state", async () => {
    mockApi({ ...signedIn, "GET /orders": () => ({ data: [], nextCursor: null }) });
    renderScreen(<OrdersScreen />);
    expect(await screen.findByText("No orders yet")).toBeInTheDocument();
  });
});
