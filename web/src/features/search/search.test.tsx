import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { krishna, milk, samba } from "@/test/fixtures";
import { calls, mockApi, renderScreen } from "@/test/support";
import { SearchScreen } from "./SearchScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
let params = new URLSearchParams();
vi.mock("next/navigation", () => ({ useSearchParams: () => params, useRouter: () => ({ push: vi.fn() }) }));

const shopCard = { id: "s1", slug: "krishna-super", name: "Krishna Super", logoUrl: null, themeColor: null, openNow: true, distanceKm: null };
const found = (extra = {}) => ({ shops: [], products: { data: [{ ...samba, shopSlug: "krishna-super" }, { ...milk, shopSlug: "krishna-super" }], nextCursor: null }, ...extra });
const base = { "GET /home": () => ({ shops: [shopCard], deals: [], promos: [] }) };

beforeEach(() => { params = new URLSearchParams(); });

describe("SearchScreen", () => {
  it("waits for two letters and for the typing to stop: one call", async () => {
    const fake = mockApi({ ...base, "GET /search": () => found() });
    renderScreen(<SearchScreen />);
    const box = screen.getByLabelText("What do you need?");
    await userEvent.type(box, "r");
    await new Promise((r) => setTimeout(r, 400));
    expect(calls(fake, "GET /search")).toHaveLength(0);
    await userEvent.type(box, "ice");
    await waitFor(() => expect(calls(fake, "GET /search")).toHaveLength(1));
    const [, options] = calls(fake, "GET /search")[0]!;
    expect(options.query).toMatchObject({ q: "rice", shop: null });
    expect(options.signal).toBeInstanceOf(AbortSignal); // a newer search can cancel this one
    expect(await screen.findByText("Samba Rice")).toBeInTheDocument();
  });

  it("opens the shop from a result when no shop was chosen", async () => {
    params = new URLSearchParams("q=rice");
    mockApi({ ...base, "GET /search": () => found() });
    renderScreen(<SearchScreen />);
    const link = await screen.findByRole("link", { name: /Samba Rice/ });
    expect(link).toHaveAttribute("href", "/s/krishna-super");
    expect(link).toHaveTextContent("Krishna Super");
  });

  it("lets the person add from the results when one shop was chosen", async () => {
    params = new URLSearchParams("q=rice&shop=krishna-super");
    const fake = mockApi({ ...base, "GET /search": () => found(), "GET /shops/krishna-super": () => krishna });
    renderScreen(<SearchScreen />);
    expect(await screen.findByRole("button", { name: "Add" })).toBeInTheDocument();
    expect(calls(fake, "GET /search")[0]![1].query).toMatchObject({ shop: "krishna-super" });
  });

  it("says when nothing was found", async () => {
    params = new URLSearchParams("q=zzzz");
    mockApi({ ...base, "GET /search": () => ({ shops: [], products: { data: [], nextCursor: null } }) });
    renderScreen(<SearchScreen />);
    expect(await screen.findByText("Nothing found")).toBeInTheDocument();
  });

  it("lists shops whose name matches", async () => {
    params = new URLSearchParams("q=krishna");
    mockApi({ ...base, "GET /search": () => ({ shops: [shopCard], products: { data: [], nextCursor: null } }) });
    renderScreen(<SearchScreen />);
    expect(await screen.findByRole("link", { name: /Krishna Super/ })).toHaveAttribute("href", "/s/krishna-super");
  });
});
