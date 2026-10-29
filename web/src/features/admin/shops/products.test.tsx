import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { calls, mockApi, renderScreen } from "@/test/support";
import { base } from "./testData";
import { ShopProductsTab } from "./ShopProductsTab";
import { detail } from "./testData";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/shop/s1/", useRouter: () => ({ push: vi.fn(), replace: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const cats = { data: [
  { id: "k1", parentId: null, slug: "rice", name: "Rice", sort: 0, defaultWeightG: 1000 },
  { id: "k2", parentId: null, slug: "dairy", name: "Dairy", sort: 1, defaultWeightG: 500 },
] };

describe("shop products tab", () => {
  it("sends only the categories that have a markup", async () => {
    const fake = mockApi({
      ...base(), "GET /admin/categories": () => cats, "GET /admin/shops/s1/category-markups": () => ({ data: [{ categoryId: "k1", markupBp: 300 }] }),
      "PUT /admin/shops/s1/category-markups": () => ({}),
    });
    renderScreen(<ShopProductsTab detail={detail()} />);
    const dairy = await screen.findByLabelText("Dairy");
    expect(screen.getByLabelText("Rice")).toHaveValue("300");
    await userEvent.type(dairy, "2500");
    expect(screen.getByRole("button", { name: "Save markups" })).toBeDisabled();
    await userEvent.clear(dairy);
    await userEvent.type(dairy, "150");
    await userEvent.click(screen.getByRole("button", { name: "Save markups" }));
    await waitFor(() => expect(calls(fake, "PUT /admin/shops/s1/category-markups")).toHaveLength(1));
    expect(calls(fake, "PUT /admin/shops/s1/category-markups")[0]![1].body).toEqual({ markups: [{ categoryId: "k1", markupBp: 300 }, { categoryId: "k2", markupBp: 150 }] });
  });

  it("puts a catalog product on sale with exact cents", async () => {
    const fake = mockApi({
      ...base(), "GET /admin/categories": () => ({ data: [] }), "GET /admin/shops/s1/category-markups": () => ({ data: [] }),
      "GET /admin/products": () => ({ data: [{ id: "m1", barcode: null, name: "Samba Rice", brand: null, sizeLabel: "5kg" }], nextCursor: "" }),
      "POST /admin/shops/s1/products": () => ({}),
    });
    renderScreen(<ShopProductsTab detail={detail()} />);
    await userEvent.type(await screen.findByLabelText("Find a catalog product"), "samba");
    await userEvent.click(await screen.findByRole("button", { name: /Samba Rice/ }));
    await userEvent.type(screen.getByLabelText("Shop price (Rs)"), "1,450.50");
    await userEvent.click(screen.getByRole("button", { name: "Put on sale" }));
    await waitFor(() => expect(calls(fake, "POST /admin/shops/s1/products")).toHaveLength(1));
    expect(calls(fake, "POST /admin/shops/s1/products")[0]![1].body).toEqual({ productId: "m1", shopPriceCents: 145050, isAvailable: true, maxPerOrder: 20 });
  });
});
