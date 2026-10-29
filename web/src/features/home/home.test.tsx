import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { writePin } from "@/lib/pinStore";
import { bootstrap, calls, mockApi, renderScreen } from "@/test/support";
import { HomeScreen } from "./HomeScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, replace: vi.fn() }) }));

const shop = { id: "s1", slug: "krishna-super", name: "Krishna Super", logoUrl: null, themeColor: "#0F5132", openNow: true, nextSlot: "2026-11-02T03:00:00Z", distanceKm: 1.84 };
const deal = { shopProductId: "p1", shopSlug: "krishna-super", shopName: "Krishna Super", name: "Samba Rice 5kg", imageUrl: null, onlinePriceCents: 143500, mrpCents: 150000, saveCents: 6500 };

beforeEach(() => {
  push.mockClear();
  writePin(null);
});
afterEach(() => vi.unstubAllGlobals());

describe("HomeScreen", () => {
  it("asks for the home data once and lists shops and deals as links", async () => {
    const fake = mockApi({ "GET /home": () => ({ shops: [shop], deals: [deal], promos: [] }) });
    renderScreen(<HomeScreen />);
    const [link] = await screen.findAllByRole("link", { name: /Krishna Super/ });
    expect(link).toHaveAttribute("href", "/s/krishna-super");
    expect(screen.getByText(/1\.8 km/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Samba Rice 5kg/ })).toHaveAttribute("href", "/s/krishna-super");
    expect(calls(fake, "GET /home")).toHaveLength(1);
    expect(fake).toHaveBeenCalledTimes(1);
  });

  it("goes to the search page with the text and the chosen shop", async () => {
    mockApi({ "GET /home": () => ({ shops: [shop], deals: [], promos: [] }) });
    renderScreen(<HomeScreen />);
    await screen.findByRole("link", { name: /Krishna Super/ });
    await userEvent.selectOptions(screen.getByLabelText("Shop"), "krishna-super");
    const box = screen.getByLabelText("What do you need?");
    await userEvent.type(box, "r");
    expect(screen.getByRole("button", { name: "Search" })).toBeDisabled(); // two letters at least
    await userEvent.type(box, "ice{enter}");
    expect(push).toHaveBeenCalledWith("/search?q=rice&shop=krishna-super");
  });

  it("keeps only a rough place after 'Near me', and asks again with it", async () => {
    const fake = mockApi({ "GET /home": () => ({ shops: [shop], deals: [], promos: [] }) });
    vi.stubGlobal("navigator", { geolocation: { getCurrentPosition: (ok: PositionCallback) => ok({ coords: { latitude: 6.93517, longitude: 79.84821 } } as GeolocationPosition) } });
    renderScreen(<HomeScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Near me" }));
    await waitFor(() => expect(calls(fake, "GET /home")).toHaveLength(2));
    expect(calls(fake, "GET /home")[1]![1].query).toEqual({ lat: 6.94, lng: 79.85 });
    expect(window.localStorage.getItem("gp_pin")).toBe('{"lat":6.94,"lng":79.85}');
    await userEvent.click(screen.getByRole("button", { name: "Show all" }));
    expect(window.localStorage.getItem("gp_pin")).toBeNull();
  });

  it("refuses a place outside Sri Lanka", async () => {
    mockApi({ "GET /home": () => ({ shops: [], deals: [], promos: [] }) });
    vi.stubGlobal("navigator", { geolocation: { getCurrentPosition: (ok: PositionCallback) => ok({ coords: { latitude: 12.97, longitude: 77.59 } } as GeolocationPosition) } });
    renderScreen(<HomeScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Near me" }));
    expect(await screen.findByText("That place is outside Sri Lanka.")).toBeInTheDocument();
    expect(window.localStorage.getItem("gp_pin")).toBeNull();
  });

  it("shows a friendly empty state", async () => {
    mockApi({ "GET /home": () => ({ shops: [], deals: [], promos: [] }), "GET /bootstrap": () => bootstrap(null) });
    renderScreen(<HomeScreen />);
    expect(await screen.findByText("Shops will show up here")).toBeInTheDocument();
  });
});
