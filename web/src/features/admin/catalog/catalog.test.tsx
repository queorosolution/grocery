import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import type { Category, Product } from "../types";
import { CatalogScreen } from "./CatalogScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/catalog/", useRouter: () => ({ push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const cat = (over: Partial<Category> = {}): Category => ({ id: "k1", parentId: null, slug: "rice", name: "Rice", sort: 0, defaultWeightG: 1000, ...over });
const product = (over: Partial<Product> = {}): Product => ({
  id: "m1", barcode: "4791234567", name: "Samba Rice", brand: "Araliya", sizeLabel: "5kg", unit: "KG", weightG: 5000, categoryId: "k1",
  imageUrl: null, keywords: "rice samba", status: "ACTIVE", ...over,
});
const base = (extra: Record<string, () => unknown> = {}) => ({
  "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
  "GET /admin/categories": () => ({ data: [cat(), cat({ id: "k2", parentId: "k1", slug: "samba", name: "Samba" })] }),
  "GET /admin/products": () => ({ data: [product()], nextCursor: "" }),
  ...extra,
});

describe("products", () => {
  it("lists products with their category and searches by word", async () => {
    const fake = mockApi(base());
    renderScreen(<CatalogScreen />);
    expect(await screen.findByText("Samba Rice")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Search products"), "samba");
    await waitFor(() => expect(calls(fake, "GET /admin/products").some(([, o]) => o.query?.q === "samba")).toBe(true));
  });

  it("changes a product, sending a blank box as null", async () => {
    const fake = mockApi(base({ "PATCH /admin/products/m1": () => ({}) }));
    renderScreen(<CatalogScreen />);
    await userEvent.click(await screen.findByText("Samba Rice"));
    const dialog = await screen.findByRole("dialog");
    await userEvent.clear(within(dialog).getByLabelText("Barcode"));
    await userEvent.clear(within(dialog).getByLabelText("Brand"));
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "PATCH /admin/products/m1")).toHaveLength(1));
    expect(calls(fake, "PATCH /admin/products/m1")[0]![1].body).toMatchObject({ barcode: null, brand: null, sizeLabel: "5kg", unit: "KG", weightG: 5000, categoryId: "k1" });
  });

  it("refuses a short barcode before asking and shows a taken one on the box", async () => {
    mockApi(base({ "POST /admin/products": () => new ApiError(400, "VALIDATION_ERROR", "Check the fields.", { barcode: "taken" }) }));
    renderScreen(<CatalogScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "New product" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Milk Powder");
    await userEvent.type(within(dialog).getByLabelText("Barcode"), "123");
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText("Barcode"), "456");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByText("Already used")).toBeInTheDocument();
  });
});

describe("categories", () => {
  it("shows two levels and keeps a category with children at the top", async () => {
    mockApi(base());
    renderScreen(<CatalogScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Categories" }));
    await userEvent.click(await screen.findByRole("button", { name: "Rice" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Inside")).toBeDisabled();
  });

  it("adds a category", async () => {
    const fake = mockApi(base({ "POST /admin/categories": () => ({}) }));
    renderScreen(<CatalogScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Categories" }));
    await userEvent.click(await screen.findByRole("button", { name: "New category" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Dairy");
    await userEvent.type(within(dialog).getByLabelText("Web name"), "Dairy-Items");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls(fake, "POST /admin/categories")).toHaveLength(1));
    expect(calls(fake, "POST /admin/categories")[0]![1].body).toEqual({ name: "Dairy", slug: "dairy-items", parentId: null, sort: 0, defaultWeightG: 500 });
  });
});
