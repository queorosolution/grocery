import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { writeCartRef } from "@/lib/cartStore";
import { krishna, milk, samba } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { ShopScreen } from "./ShopScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/s/krishna-super/", useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/lib/cartApi", async (orig) => ({
  ...(await orig<typeof import("@/lib/cartApi")>()),
  cartApi: {
    create: vi.fn().mockResolvedValue({ id: "c1", status: "open", lines: [], subtotal: 0, itemCount: 0 }),
    addLine: vi.fn().mockResolvedValue({
      id: "c1", status: "open", subtotal: 143500, itemCount: 1,
      lines: [{ id: "l1", itemId: "p1", name: "Samba Rice", unitPrice: 143500, quantity: 1, lineTotal: 143500 }],
    }),
    get: vi.fn(), setQuantity: vi.fn(), discard: vi.fn(),
  },
}));

const page = (data: unknown[], nextCursor: string | null = null) => ({ data, nextCursor });

beforeEach(() => writeCartRef(null));

describe("ShopScreen", () => {
  it("opens with two calls when signed out: the shop and its products", async () => {
    const fake = mockApi({
      "GET /shops/krishna-super": () => krishna,
      "GET /shops/krishna-super/products": () => page([samba, milk]),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    expect(await screen.findByRole("heading", { name: "Krishna Super" })).toBeInTheDocument();
    expect(await screen.findByText("Samba Rice")).toBeInTheDocument();
    await waitFor(() => expect(calls(fake, "GET /bootstrap")).toHaveLength(1));
    const shopCalls = (fake.mock.calls as [string][]).map(([p]) => p).filter((p) => p.startsWith("/shops"));
    expect(shopCalls.sort()).toEqual(["/shops/krishna-super", "/shops/krishna-super/products"]);
    expect(screen.getByText("Open 08:00 – 21:00 · Min Rs 1,000")).toBeInTheDocument();
  });

  it("adds a third call for buy-again when signed in, and shows that row", async () => {
    const fake = mockApi({
      "GET /shops/krishna-super": () => krishna,
      "GET /shops/krishna-super/products": () => page([milk]),
      "GET /shops/krishna-super/buy-again": () => ({ data: [samba] }),
      "GET /bootstrap": () => bootstrap(me()),
    });
    renderScreen(<ShopScreen />);
    const row = await screen.findByRole("region", { name: "Buy again" });
    expect(within(row).getByText("Samba Rice")).toBeInTheDocument();
    expect(calls(fake, "GET /shops/krishna-super/buy-again")).toHaveLength(1);
  });

  it("loads another category and shows the sticky cart bar after an add", async () => {
    const fake = mockApi({
      "GET /shops/krishna-super": () => krishna,
      "GET /shops/krishna-super/products": (): unknown => page([samba]),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Rice" }));
    await waitFor(() => expect(calls(fake, "GET /shops/krishna-super/products").some(([, o]) => o.query?.category === "rice")).toBe(true));
    // Only top-level categories are chips.
    expect(screen.queryByRole("button", { name: "Basmati" })).not.toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Add" }));
    expect(await screen.findByRole("button", { name: /1 item.*View cart · Rs 1,435\.00/ })).toBeInTheDocument();
  });

  it("marks what cannot be bought and gives it no add button", async () => {
    mockApi({
      "GET /shops/krishna-super": () => krishna,
      "GET /shops/krishna-super/products": () => page([milk]),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    expect(await screen.findByText("Not available")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add" })).not.toBeInTheDocument();
  });

  it("browses without add buttons while the shop has no cart yet", async () => {
    mockApi({
      "GET /shops/krishna-super": () => ({ ...krishna, cart: null }),
      "GET /shops/krishna-super/products": () => page([samba]),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    expect(await screen.findByText("Samba Rice")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add" })).not.toBeInTheDocument();
  });

  it("pages through products", async () => {
    const fake = mockApi({
      "GET /shops/krishna-super": () => krishna,
      "GET /shops/krishna-super/products": (): unknown => page([samba], "next-1"),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Show more" }));
    await waitFor(() => expect(calls(fake, "GET /shops/krishna-super/products")).toHaveLength(2));
    expect(calls(fake, "GET /shops/krishna-super/products")[1]![1].query?.cursor).toBe("next-1");
  });

  it("says so when the shop is not found", async () => {
    mockApi({
      "GET /shops/krishna-super": () => new ApiError(404, "NOT_FOUND", "Not found"),
      "GET /shops/krishna-super/products": () => new ApiError(404, "NOT_FOUND", "Not found"),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    expect(await screen.findByText("We can't find that shop")).toBeInTheDocument();
  });

  it("paints the header only with a plain hex colour from the shop", async () => {
    mockApi({
      "GET /shops/krishna-super": () => ({ ...krishna, themeColor: "red; background:url(x)" }),
      "GET /shops/krishna-super/products": () => page([]),
      "GET /bootstrap": () => bootstrap(null),
    });
    renderScreen(<ShopScreen />);
    const title = await screen.findByRole("heading", { name: "Krishna Super" });
    expect(title.closest("section")?.getAttribute("style")).toBeNull();
  });
});
