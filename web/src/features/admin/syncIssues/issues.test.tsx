import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { adminShop, superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import type { SyncIssue } from "../types";
import { SyncIssuesScreen } from "./SyncIssuesScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/sync-issues/", useRouter: () => ({ push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const issue = (over: Partial<SyncIssue> = {}): SyncIssue => ({
  id: "i1", connectorId: "c1", shopId: "s1", kind: "UNMATCHED", externalId: "X-9", status: "OPEN", createdAt: "2026-10-29T00:00:00Z", resolvedAt: null,
  payload: { reason: "unknown_barcode", name: "Samba 5kg", barcode: "4791", brand: null, priceCents: 145000, mrpCents: null, stockQty: null, available: true }, ...over,
});
const setup = (rows: SyncIssue[], extra: Record<string, () => unknown> = {}) => {
  const fake = mockApi({
    "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
    "GET /admin/shops": () => ({ data: [adminShop()] }),
    "GET /admin/sync-issues": () => ({ data: rows }),
    ...extra,
  });
  renderScreen(<SyncIssuesScreen />);
  return fake;
};

describe("sync issues", () => {
  it("asks for open issues and names the shop", async () => {
    const fake = setup([issue()]);
    expect(await screen.findByText("Samba 5kg")).toBeInTheDocument();
    expect(await screen.findByText(/Krishna Super · Rs 1,450/)).toBeInTheDocument();
    expect(calls(fake, "GET /admin/sync-issues")[0]![1].query).toMatchObject({ status: "OPEN" });
  });

  it("links an unmatched row to a catalog product", async () => {
    const fake = setup([issue()], {
      "GET /admin/products": () => ({ data: [{ id: "m1", barcode: "4791", name: "Samba Rice", brand: "Araliya", sizeLabel: "5kg" }], nextCursor: "" }),
      "POST /admin/sync-issues/i1/resolve": () => ({}),
    });
    await userEvent.click(await screen.findByRole("button", { name: "Resolve" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Link to a product in our catalog"), "samba");
    await userEvent.click(await within(dialog).findByRole("button", { name: /Samba Rice/ }));
    await waitFor(() => expect(calls(fake, "POST /admin/sync-issues/i1/resolve")).toHaveLength(1));
    expect(calls(fake, "POST /admin/sync-issues/i1/resolve")[0]![1].body).toEqual({ action: "LINK", productId: "m1" });
  });

  it("never offers to approve a price of zero", async () => {
    setup([issue({ kind: "PRICE_HOLD", payload: { ...issue().payload, priceCents: 0, currentPriceCents: 145000, reason: "zero_price" } })]);
    await userEvent.click(await screen.findByRole("button", { name: "Resolve" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Use the new price" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Keep the current price" })).toBeEnabled();
  });
});
