import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { adminShop as shop, superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { AdminDashboardScreen } from "./DashboardScreen";
import { ShopsScreen } from "./shops/ShopsScreen";
import type { AdminDashboard } from "./types";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
const push = vi.fn();
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/", useRouter: () => ({ push, replace: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const admin = () => bootstrap(me({ roles: superAdmin() }));

const dash: AdminDashboard = {
  from: "2026-10-23", to: "2026-10-29", gmvCents: 12345600, orders: 40, delivered: 35, avgBillCents: 300000, cancelRateBps: 250,
  platformIncomeCents: 500000, marginCents: 400000, serviceFeeCents: 100000, couponCostCents: 20000, smsSent: 80, failedPayments: 2,
  sync: { active: 1, failing: 1, openIssues: 3 },
  days: [{ day: "2026-10-28", salesCents: 100000, orders: 5 }],
  shops: [{ shopId: "s1", name: "Krishna Super", salesCents: 12345600, orders: 40, delivered: 35, platformIncomeCents: 500000 }],
};

describe("admin dashboard", () => {
  it("asks for the last 7 days", async () => {
    const fake = mockApi({ "GET /bootstrap": admin, "GET /admin/dashboard": () => dash });
    renderScreen(<AdminDashboardScreen />);
    expect((await screen.findAllByText("Rs 123,456")).length).toBeGreaterThan(0);
    expect(screen.getByText("2.5%")).toBeInTheDocument();
    expect(screen.getByText(/Syncs on 1 · Syncs failing 1/)).toBeInTheDocument();
    const q = calls(fake, "GET /admin/dashboard")[0]![1].query as { from: string; to: string };
    expect(q.from < q.to).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "30 days" }));
    await waitFor(() => expect(calls(fake, "GET /admin/dashboard")).toHaveLength(2));
  });
});

describe("shops list", () => {
  it("lists shops and opens one", async () => {
    mockApi({ "GET /bootstrap": admin, "GET /admin/shops": () => ({ data: [shop(), shop({ id: "s2", name: "Ceylon Mart", status: "DRAFT", acceptingOrders: false })] }) });
    renderScreen(<ShopsScreen />);
    await userEvent.click(await screen.findByText("Ceylon Mart"));
    expect(push).toHaveBeenCalledWith("/console/admin/shop/s2");
  });

  it("creates a shop with the typed profile and goes to its setup", async () => {
    const fake = mockApi({
      "GET /bootstrap": admin, "GET /admin/shops": () => ({ data: [] }),
      "POST /admin/shops": () => shop({ id: "new1" }),
    });
    renderScreen(<ShopsScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "New shop" }));
    await userEvent.type(screen.getByLabelText("Name"), "Fresh Mart");
    await userEvent.type(screen.getByLabelText("Web address"), "Fresh-Mart");
    // The map box falls back to two number boxes without a Maps key.
    await userEvent.type(screen.getByLabelText(/latitude/i), "6.9");
    await userEvent.type(screen.getByLabelText(/longitude/i), "79.8");
    await userEvent.click(screen.getByRole("button", { name: "Create shop" }));
    await waitFor(() => expect(calls(fake, "POST /admin/shops")).toHaveLength(1));
    expect(calls(fake, "POST /admin/shops")[0]![1].body).toMatchObject({ name: "Fresh Mart", slug: "fresh-mart", lat: 6.9, lng: 79.8 });
    await waitFor(() => expect(push).toHaveBeenCalledWith("/console/admin/shop/new1?tab=settings"));
  });
});
