import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { playChime, unlockChime } from "@/lib/chime";
import { owner, queueRow, staff } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { OrdersPage } from "./OrdersPage";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/shop/s1/orders/", useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/lib/chime", () => ({ playChime: vi.fn(), unlockChime: vi.fn(() => true) }));

const page = (...rows: ReturnType<typeof queueRow>[]) => ({ data: rows, nextCursor: null });
const base = (roles = owner()) => ({ "GET /bootstrap": () => bootstrap(me({ roles })) });

beforeEach(() => {
  vi.mocked(playChime).mockClear();
  vi.mocked(unlockChime).mockClear();
});
afterEach(() => {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
  vi.useRealTimers();
});

describe("order queue", () => {
  it("shows the shop, the tabs this person may use, and the new bills first", async () => {
    mockApi({ ...base(staff()), "GET /console/shops/s1/orders": () => page(queueRow()) });
    renderScreen(<OrdersPage />);
    expect(await screen.findByRole("heading", { name: "Krishna Super" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /KS-7Q2M9X · Nimal Perera/ })).toHaveAttribute("href", "/console/shop/s1/order/o1");
    expect(screen.getByRole("link", { name: "Orders" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dashboard" })).toBeInTheDocument();
    // Staff do not see the owner's pages.
    expect(screen.queryByRole("link", { name: "Team" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Payouts" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "New (1)" })).toBeInTheDocument();
  });

  it("asks for the statuses of the chosen tab", async () => {
    const fake = mockApi({ ...base(), "GET /console/shops/s1/orders": () => page() });
    renderScreen(<OrdersPage />);
    await userEvent.click(await screen.findByRole("tab", { name: "To pick" }));
    await waitFor(() => expect(calls(fake, "GET /console/shops/s1/orders").some(([, o]) => o.query?.status === "PAID,PICKING")).toBe(true));
    await userEvent.click(screen.getByRole("tab", { name: "Done" }));
    await waitFor(() => expect(calls(fake, "GET /console/shops/s1/orders").some(([, o]) => o.query?.status === "DELIVERED,REJECTED,EXPIRED,CANCELLED,CANCELLED_REFUND")).toBe(true));
  });

  it("says so when there is nothing", async () => {
    mockApi({ ...base(), "GET /console/shops/s1/orders": () => page() });
    renderScreen(<OrdersPage />);
    expect(await screen.findByText("Nothing here right now.")).toBeInTheDocument();
  });

  it("turns someone with no place in this shop away", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(me({ roles: [] })) });
    renderScreen(<OrdersPage />);
    expect(await screen.findByText("This page isn't for your account")).toBeInTheDocument();
  });
});

describe("new bill chime", () => {
  it("rings for a bill that was not there before, and not for the first answer", async () => {
    let rows = [queueRow()];
    vi.useFakeTimers({ shouldAdvanceTime: true });
    mockApi({ ...base(), "GET /console/shops/s1/orders": () => page(...rows) });
    const { client } = renderScreen(<OrdersPage />);
    await userEvent.click(await screen.findByRole("button", { name: /Sound is off/ }));
    expect(unlockChime).toHaveBeenCalledOnce();
    expect(playChime).not.toHaveBeenCalled();
    rows = [queueRow(), queueRow({ id: "o2", code: "KS-NEW222" })];
    await act(async () => { await client.invalidateQueries({ queryKey: ["shopConsole"] }); });
    await waitFor(() => expect(playChime).toHaveBeenCalledOnce());
  });

  it("stays quiet while the sound is off", async () => {
    let rows = [queueRow()];
    mockApi({ ...base(), "GET /console/shops/s1/orders": () => page(...rows) });
    const { client } = renderScreen(<OrdersPage />);
    await screen.findByRole("link", { name: /KS-7Q2M9X/ });
    rows = [queueRow(), queueRow({ id: "o2", code: "KS-NEW222" })];
    await act(async () => { await client.invalidateQueries({ queryKey: ["shopConsole"] }); });
    await screen.findByRole("link", { name: /KS-NEW222/ });
    expect(playChime).not.toHaveBeenCalled();
  });
});

describe("polling", () => {
  it("asks again every 20 s while visible, and stops while the tab is hidden", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const fake = mockApi({ ...base(), "GET /console/shops/s1/orders": () => page(queueRow()) });
    renderScreen(<OrdersPage />);
    await screen.findByRole("link", { name: /KS-7Q2M9X/ });
    const n = () => calls(fake, "GET /console/shops/s1/orders").length;
    const first = n();
    await act(async () => { await vi.advanceTimersByTimeAsync(21_000); });
    expect(n()).toBeGreaterThan(first);

    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
    await act(async () => { document.dispatchEvent(new Event("visibilitychange")); });
    const hiddenAt = n();
    await act(async () => { await vi.advanceTimersByTimeAsync(65_000); });
    expect(n()).toBe(hiddenAt);
  });
});
