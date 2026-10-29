import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { adminShop, superAdmin } from "@/test/fixtures";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { ConnectorTab } from "./ConnectorTab";
import type { Connector, ShopDetail } from "../types";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => "/console/admin/", useRouter: () => ({ push: vi.fn(), replace: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const detail = { shop: adminShop() } as ShopDetail;
const connector = (over: Partial<Connector> = {}): Connector => ({
  id: "c1", shopId: "s1", status: "DRAFT", host: "postgres://gp_reader:•••@db.example.com/shop", sourceTable: null, moneyUnit: "RUPEES",
  changeColumn: null, intervalMin: 60, autoCreateProducts: false, columns: {}, lastRunAt: null, ...over,
});
const notFound = () => new ApiError(404, "NOT_FOUND", "Not found.");
const base = (c: Connector | Error) => ({
  "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
  "GET /admin/shops/s1/connector": () => c,
  "GET /admin/connectors/c1/runs": () => ({ data: [] }),
  "GET /admin/connectors/c1/tables": () => ({ data: [{ schema: "public", name: "gp_products", kind: "VIEW" }] }),
  "GET /admin/connectors/c1/columns": () => ({ data: ["external_id", "name", "price", "updated_at"].map((n) => ({ name: n, type: "text" })) }),
});

describe("connect", () => {
  it("sends the connection string once, empties the box and never shows it", async () => {
    let made = false;
    const fake = mockApi({
      ...base(notFound()),
      "GET /admin/shops/s1/connector": () => (made ? connector() : notFound()),
      "POST /admin/shops/s1/connector": () => { made = true; return connector(); },
    });
    renderScreen(<ConnectorTab detail={detail} />);
    const box = await screen.findByLabelText("Connection string");
    expect(box).toHaveAttribute("type", "password");
    await userEvent.type(box, "postgresql://u:topsecret@db.example.com/shop?sslmode=require");
    await userEvent.click(screen.getByRole("button", { name: "Save string" }));
    await waitFor(() => expect(calls(fake, "POST /admin/shops/s1/connector")).toHaveLength(1));
    expect(calls(fake, "POST /admin/shops/s1/connector")[0]![1].body).toEqual({ connectionString: "postgresql://u:topsecret@db.example.com/shop?sslmode=require" });
    expect(await screen.findByText(/gp_reader:•••@db.example.com/)).toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain("topsecret");
    // The replace box starts empty too.
    expect(screen.getByLabelText("Replace the connection string")).toHaveValue("");
  });

  it("shows a refused string on the field and tests the connection", async () => {
    mockApi({
      ...base(connector()),
      "PUT /admin/connectors/c1/secret": () => new ApiError(400, "VALIDATION_ERROR", "Check the fields.", { connectionString: "invalid" }),
      "POST /admin/connectors/c1/test": () => ({ ok: true, readOnly: false, serverVersion: "16.2", warnings: ["login can write"] }),
    });
    renderScreen(<ConnectorTab detail={detail} />);
    const box = await screen.findByLabelText("Replace the connection string");
    await userEvent.type(box, "mysql://nope");
    await userEvent.click(screen.getByRole("button", { name: "Save string" }));
    expect(await screen.findByText("Not valid")).toBeInTheDocument();
    expect(box).toHaveValue("");
    await userEvent.click(screen.getByRole("button", { name: "Test connection" }));
    expect(await screen.findByText("Connected. Server 16.2.")).toBeInTheDocument();
    expect(screen.getByText(/can write to the shop's database/)).toBeInTheDocument();
  });
});

describe("mapping and preview", () => {
  it("needs the three required columns, then saves the mapping", async () => {
    const fake = mockApi({ ...base(connector({ sourceTable: "public.gp_products" })), "PUT /admin/connectors/c1/mapping": () => ({}) });
    renderScreen(<ConnectorTab detail={detail} />);
    const save = await screen.findByRole("button", { name: "Save mapping" });
    expect(save).toBeDisabled();
    await waitFor(() => expect(screen.getByLabelText("Name *").querySelectorAll("option").length).toBeGreaterThan(1));
    await userEvent.selectOptions(screen.getByLabelText("Id in the shop's table *"), "external_id");
    await userEvent.selectOptions(screen.getByLabelText("Name *"), "name");
    await userEvent.selectOptions(screen.getByLabelText("Price *"), "price");
    await userEvent.selectOptions(screen.getByLabelText("Last changed time"), "updated_at");
    expect(save).toBeEnabled();
    await userEvent.click(save);
    await waitFor(() => expect(calls(fake, "PUT /admin/connectors/c1/mapping")).toHaveLength(1));
    expect(calls(fake, "PUT /admin/connectors/c1/mapping")[0]![1].body).toEqual({
      sourceTable: "public.gp_products", moneyUnit: "RUPEES", changeColumn: "updated_at", intervalMin: 60, autoCreateProducts: false,
      columns: { externalId: "external_id", name: "name", price: "price", updatedAt: "updated_at" },
    });
  });

  it("previews rows with what would happen to each", async () => {
    mockApi({
      ...base(connector({ sourceTable: "public.gp_products" })),
      "POST /admin/connectors/c1/preview": () => ({ rows: [
        { externalId: "1", name: "Samba 5kg", barcode: "4791", priceCents: 145000, mrpCents: null, stockQty: 3, available: true, action: "update", reason: "barcode" },
        { externalId: "2", name: "Free milk", barcode: null, priceCents: 0, mrpCents: null, stockQty: null, available: null, action: "hold", reason: "zero_price" },
      ] }),
    });
    renderScreen(<ConnectorTab detail={detail} />);
    await userEvent.click(await screen.findByRole("button", { name: "Read 20 rows" }));
    expect(await screen.findByText("Samba 5kg")).toBeInTheDocument();
    expect(screen.getByText("Rs 1,450.00")).toBeInTheDocument();
    expect(screen.getByText("Held")).toBeInTheDocument();
    expect(screen.getByText("Price is zero")).toBeInTheDocument();
  });
});

describe("go live", () => {
  it("switches on, and shows a failed run with a fixed message", async () => {
    let live = false;
    const fake = mockApi({
      ...base(connector({ sourceTable: "public.gp_products" })),
      "GET /admin/shops/s1/connector": () => connector({ sourceTable: "public.gp_products", status: live ? "ACTIVE" : "DRAFT" }),
      "POST /admin/connectors/c1/activate": () => { live = true; return {}; },
      "GET /admin/connectors/c1/runs": () => ({ data: [{ id: "r1", trigger: "SCHEDULE", status: "FAILED", rowsRead: 0, rowsChanged: 0, rowsUnmatched: 0, rowsHeld: 0, error: "source_failed", startedAt: "2026-10-29T04:00:00Z", finishedAt: null }] }),
    });
    renderScreen(<ConnectorTab detail={detail} />);
    expect(await screen.findByText("Could not read the shop's database")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Switch on" }));
    await waitFor(() => expect(calls(fake, "POST /admin/connectors/c1/activate")).toHaveLength(1));
    expect(await screen.findByRole("button", { name: "Pause" })).toBeInTheDocument();
  });

  it("shows the trigger secret once and forgets it when the window closes", async () => {
    mockApi({ ...base(connector({ sourceTable: "public.gp_products" })), "POST /admin/connectors/c1/trigger-secret": () => ({ secret: "trig-secret-123", example: "curl -H \"X-GP-Signature: $TRIGGER_SECRET\"" }) });
    renderScreen(<ConnectorTab detail={detail} />);
    await userEvent.click(await screen.findByRole("button", { name: "Make a new secret" }));
    const dialog = await screen.findByRole("dialog", { name: "Trigger secret" });
    expect(within(dialog).getByText("trig-secret-123")).toBeInTheDocument();
    expect(within(dialog).getByText(/\$TRIGGER_SECRET/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "I have copied it" }));
    await waitFor(() => expect(document.body.textContent).not.toContain("trig-secret-123"));
  });
});
