import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { calls, mockApi, renderScreen } from "@/test/support";
import { base, detail } from "./testData";
import { ShopPage } from "./ShopPage";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
let tab = "";
const replace = vi.fn();
vi.mock("next/navigation", () => ({
  usePathname: () => "/console/admin/shop/s1/",
  useRouter: () => ({ push: vi.fn(), replace }),
  useSearchParams: () => new URLSearchParams(tab ? `tab=${tab}` : ""),
}));

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
