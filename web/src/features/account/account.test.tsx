import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { AccountScreen } from "./AccountScreen";
import { AddressesScreen } from "./AddressesScreen";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("@/features/auth/AuthProvider", () => ({ useAuth: () => ({ openSignIn: vi.fn() }) }));
const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push: vi.fn() }) }));
vi.mock("@/features/auth/Turnstile", () => ({ Turnstile: () => null }));

const home = { id: "a1", label: "Home", line1: "12 Galle Road", landmark: "Near the temple", lat: 6.9, lng: 79.86, isDefault: true };
const work = { id: "a2", label: "Work", line1: "5 Union Place", landmark: null, lat: 6.92, lng: 79.85, isDefault: false };

beforeEach(() => replace.mockClear());

describe("AccountScreen", () => {
  it("shows the profile and renames the person", async () => {
    let who = me();
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(who),
      "PATCH /me": (b) => (who = { ...who, name: (b as { name: string }).name }),
    });
    renderScreen(<AccountScreen />);
    expect(await screen.findByText("Nimal Perera")).toBeInTheDocument();
    expect(screen.getByText("nimal@example.com")).toBeInTheDocument();
    expect(screen.getByText("077•••4567")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Edit" })[0]!);
    const box = screen.getByLabelText("Name");
    await userEvent.clear(box);
    await userEvent.type(box, "Nimal P");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Nimal P")).toBeInTheDocument();
    expect(calls(fake, "PATCH /me")[0]![1].body).toEqual({ name: "Nimal P" });
  });

  it("signs out on this device or everywhere", async () => {
    const fake = mockApi({ "GET /bootstrap": () => bootstrap(me()), "POST /auth/logout": () => undefined });
    renderScreen(<AccountScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/"));
    expect(calls(fake, "POST /auth/logout")[0]![1].body).toEqual({ all: false });
  });

  it("deletes the account only after the word is typed, then goes home", async () => {
    const fake = mockApi({ "GET /bootstrap": () => bootstrap(me()), "DELETE /me": () => undefined });
    renderScreen(<AccountScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Delete account" }));
    const confirm = screen.getByRole("button", { name: "Delete my account" });
    expect(confirm).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Type DELETE to confirm"), "delete");
    expect(confirm).toBeDisabled(); // the word is case-sensitive
    await userEvent.clear(screen.getByLabelText("Type DELETE to confirm"));
    await userEvent.type(screen.getByLabelText("Type DELETE to confirm"), "DELETE");
    await userEvent.click(confirm);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/"));
    expect(calls(fake, "DELETE /me")[0]![1].body).toEqual({ confirm: "DELETE" });
  });

  it("lists every reason when the API refuses, and stays signed in", async () => {
    mockApi({
      "GET /bootstrap": () => bootstrap(me()),
      "DELETE /me": () => new ApiError(409, "ACCOUNT_NOT_DELETABLE", "Finish your open orders.", { orders: "open", refunds: "pending" }),
    });
    renderScreen(<AccountScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Delete account" }));
    await userEvent.type(screen.getByLabelText("Type DELETE to confirm"), "DELETE");
    await userEvent.click(screen.getByRole("button", { name: "Delete my account" }));
    expect(await screen.findByText("You have an order in progress or a bill that is not paid yet.")).toBeInTheDocument();
    expect(screen.getByText("A refund is still being paid out to you.")).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });
});

describe("AddressesScreen", () => {
  it("lists addresses with the main one marked, and moves the main mark", async () => {
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(me()),
      "GET /me/addresses": () => ({ data: [home, work] }),
      "PATCH /me/addresses/a2": () => ({ address: { ...work, isDefault: true }, deliveringShops: [] }),
    });
    renderScreen(<AddressesScreen />);
    expect(await screen.findByText("12 Galle Road")).toBeInTheDocument();
    expect(screen.getByText("Main")).toBeInTheDocument();
    const card = screen.getByText("5 Union Place").closest("div[class*='rounded-card']") as HTMLElement;
    await userEvent.click(within(card).getByRole("button", { name: "Make main" }));
    await waitFor(() => expect(calls(fake, "PATCH /me/addresses/a2")).toHaveLength(1));
    expect(calls(fake, "PATCH /me/addresses/a2")[0]![1].body).toMatchObject({ isDefault: true, label: "Work", landmark: "" });
  });

  it("asks before removing an address", async () => {
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(me()),
      "GET /me/addresses": () => ({ data: [home, work] }),
      "DELETE /me/addresses/a2": () => undefined,
    });
    renderScreen(<AddressesScreen />);
    const card = (await screen.findByText("5 Union Place")).closest("div[class*='rounded-card']") as HTMLElement;
    await userEvent.click(within(card).getByRole("button", { name: "Remove" }));
    expect(calls(fake, "DELETE /me/addresses/a2")).toHaveLength(0);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("“Work” will be removed from your account.")).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(calls(fake, "DELETE /me/addresses/a2")).toHaveLength(1));
  });

  it("stops at five addresses", async () => {
    const five = Array.from({ length: 5 }, (_, i) => ({ ...work, id: `a${i}`, isDefault: i === 0 }));
    mockApi({ "GET /bootstrap": () => bootstrap(me()), "GET /me/addresses": () => ({ data: five }) });
    renderScreen(<AddressesScreen />);
    expect(await screen.findByRole("button", { name: "Add address" })).toBeDisabled();
    expect(screen.getByText("You can keep up to 5 addresses. Remove one to add another.")).toBeInTheDocument();
  });

  it("adds an address and then tells which shops deliver there", async () => {
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(me()),
      "GET /me/addresses": () => ({ data: [] }),
      "POST /me/addresses": () => ({ address: home, deliveringShops: [] }),
    });
    renderScreen(<AddressesScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Add address" }));
    await userEvent.type(screen.getByLabelText("Address"), "12 Galle Road");
    await userEvent.type(screen.getByLabelText("Latitude"), "6.9");
    await userEvent.type(screen.getByLabelText("Longitude"), "79.86");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));
    expect(await screen.findByText("No shop delivers here yet. You can still look around.")).toBeInTheDocument();
    expect(calls(fake, "POST /me/addresses")[0]![1].body).toMatchObject({ isDefault: true }); // the first one is the main one
  });

  it("shows the server's reason when it refuses", async () => {
    mockApi({
      "GET /bootstrap": () => bootstrap(me()),
      "GET /me/addresses": () => ({ data: [work] }),
      "POST /me/addresses": () => new ApiError(400, "VALIDATION_ERROR", "Check the highlighted fields.", { addresses: "max_5" }),
    });
    renderScreen(<AddressesScreen />);
    await userEvent.click(await screen.findByRole("button", { name: "Add address" }));
    await userEvent.type(screen.getByLabelText("Address"), "12 Galle Road");
    await userEvent.type(screen.getByLabelText("Latitude"), "6.9");
    await userEvent.type(screen.getByLabelText("Longitude"), "79.86");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));
    expect((await screen.findAllByText("You can keep up to 5 addresses. Remove one to add another.")).length).toBeGreaterThan(0);
  });
});
