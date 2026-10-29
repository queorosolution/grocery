import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { OnboardingScreen } from "./OnboardingScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("@/features/auth/AuthProvider", () => ({ useAuth: () => ({ openSignIn: vi.fn() }) }));

const replace = vi.fn();
let search = "";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(search),
}));

// The real widget loads Cloudflare's script; this one passes the check when pressed.
vi.mock("@/features/auth/Turnstile", () => ({
  Turnstile: ({ onToken }: { onToken: (t: string | null) => void }) => <button onClick={() => onToken("ts-token")}>Pass bot check</button>,
}));

const address = { id: "a1", label: "Home", line1: "12 Galle Road", landmark: null, lat: 6.9, lng: 79.86, isDefault: true };

beforeEach(() => {
  replace.mockClear();
  search = "";
});

describe("OnboardingScreen", () => {
  it("walks a new customer through name, phone and address, then returns to where they were", async () => {
    search = "next=%2Fs%2Fkrishna%2F";
    let who = me({ phoneVerified: false, phoneMasked: null, needsOnboarding: true });
    let saved: unknown[] = [];
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(who),
      "GET /me/addresses": () => ({ data: saved }),
      "PATCH /me": (b) => (who = { ...who, name: (b as { name: string }).name }),
      "POST /me/phone/send": () => undefined,
      "POST /me/phone/verify": () => (who = { ...who, phoneVerified: true, needsOnboarding: false }),
      "POST /me/addresses": () => {
        saved = [address];
        return { address, deliveringShops: [{ slug: "krishna", name: "Krishna Super", distanceKm: 1.84 }] };
      },
    });
    renderScreen(<OnboardingScreen />);

    // 1. name
    expect(await screen.findByText("Step 1 of 3")).toBeInTheDocument();
    const name = screen.getByLabelText("Your name");
    await userEvent.clear(name);
    await userEvent.type(name, "Nimal P");
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(calls(fake, "PATCH /me")[0]![1].body).toEqual({ name: "Nimal P" }));

    // 2. phone: the text cannot be requested before the bot check passes
    expect(await screen.findByText("Step 2 of 3")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Mobile number"), "0771234567");
    expect(screen.getByRole("button", { name: "Send code" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Pass bot check" }));
    await userEvent.click(screen.getByRole("button", { name: "Send code" }));
    expect(await screen.findByText("We sent a code to 0771234567.")).toBeInTheDocument();
    expect(calls(fake, "POST /me/phone/send")[0]![1].body).toEqual({ phone: "0771234567", turnstileToken: "ts-token" });
    await userEvent.type(screen.getByLabelText("Code from the text"), "123456");
    await userEvent.click(screen.getByRole("button", { name: "Verify" }));

    // 3. address: the pin needs both numbers inside Sri Lanka
    expect(await screen.findByText("Step 3 of 3")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Address"), "12 Galle Road");
    await userEvent.type(screen.getByLabelText("Latitude"), "48.8");
    await userEvent.type(screen.getByLabelText("Longitude"), "2.3");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));
    expect(await screen.findByText("That pin is outside Sri Lanka.")).toBeInTheDocument();
    expect(calls(fake, "POST /me/addresses")).toHaveLength(0);
    await userEvent.clear(screen.getByLabelText("Latitude"));
    await userEvent.type(screen.getByLabelText("Latitude"), "6.9");
    await userEvent.clear(screen.getByLabelText("Longitude"));
    await userEvent.type(screen.getByLabelText("Longitude"), "79.86");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));
    expect(await screen.findByText("1 shop delivers here")).toBeInTheDocument();
    expect(calls(fake, "POST /me/addresses")[0]![1].body).toMatchObject({ label: "Home", line1: "12 Galle Road", lat: 6.9, lng: 79.86, isDefault: true });
    await userEvent.click(screen.getByRole("button", { name: "Start shopping" }));
    expect(replace).toHaveBeenCalledWith("/s/krishna/");
  });

  it("never returns to another site, whatever ?next= says", async () => {
    search = "next=https%3A%2F%2Fevil.example%2F";
    mockApi({ "GET /bootstrap": () => bootstrap(me()), "GET /me/addresses": () => ({ data: [address] }) });
    renderScreen(<OnboardingScreen />);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/"));
  });

  it("skips the steps that are already done", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(me()), "GET /me/addresses": () => ({ data: [] }) });
    renderScreen(<OnboardingScreen />);
    expect(await screen.findByText("Step 1 of 1")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Where should we deliver?" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add it later" }));
    expect(replace).toHaveBeenCalledWith("/");
  });

  it("shows the API's own words when the phone is refused", async () => {
    mockApi({
      "GET /bootstrap": () => bootstrap(me({ phoneVerified: false, needsOnboarding: true })),
      "GET /me/addresses": () => ({ data: [address] }),
      "PATCH /me": () => me({ phoneVerified: false }),
      "POST /me/phone/send": () => new ApiError(400, "VALIDATION_ERROR", "Check the highlighted fields.", { phone: "invalid" }),
    });
    renderScreen(<OnboardingScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Next" }));
    await userEvent.type(await screen.findByLabelText("Mobile number"), "12345678901");
    await userEvent.click(screen.getByRole("button", { name: "Pass bot check" }));
    await userEvent.click(screen.getByRole("button", { name: "Send code" }));
    expect(await screen.findByText("Enter a Sri Lankan mobile number, like 0771234567.")).toBeInTheDocument();
  });
});
