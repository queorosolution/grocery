import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { consoleLinks, hasRole } from "./roles";
import { UserMenu } from "./UserMenu";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
const openSignIn = vi.fn();
vi.mock("./AuthProvider", () => ({ useAuth: () => ({ openSignIn }) }));
const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push: vi.fn() }) }));

describe("UserMenu", () => {
  it("offers sign-in to a visitor", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(null) });
    renderScreen(<UserMenu />);
    await userEvent.click(await screen.findByRole("button", { name: "Sign in" }));
    expect(openSignIn).toHaveBeenCalled();
  });

  it("opens a menu for a customer: account and sign out, no console link", async () => {
    const fake = mockApi({ "GET /bootstrap": () => bootstrap(me()), "POST /auth/logout": () => undefined });
    renderScreen(<UserMenu />);
    await userEvent.click(await screen.findByRole("button", { name: "Account menu" }));
    expect(screen.getByRole("menuitem", { name: "My account" })).toHaveAttribute("href", "/account");
    expect(screen.queryByRole("menuitem", { name: "Console" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/"));
    expect(calls(fake, "POST /auth/logout")).toHaveLength(1);
  });

  it("closes on Escape and shows the console link to a team member", async () => {
    const staff = me({ roles: [{ role: "SHOP_OWNER", shopId: "s1", shopSlug: "k", shopName: "Krishna" }] });
    mockApi({ "GET /bootstrap": () => bootstrap(staff) });
    renderScreen(<UserMenu />);
    await userEvent.click(await screen.findByRole("button", { name: "Account menu" }));
    expect(screen.getByRole("menuitem", { name: "Console" })).toHaveAttribute("href", "/console");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });
});

describe("roles", () => {
  const roleOf = (role: "SUPER_ADMIN" | "SHOP_OWNER" | "SHOP_STAFF" | "SHOP_DRIVER" | "PLATFORM_DRIVER", shopId: string | null = null) =>
    ({ role, shopId, shopSlug: null, shopName: shopId ? "Krishna" : null });

  it("hasRole is false for a visitor and a customer", () => {
    expect(hasRole(null, "SHOP_OWNER")).toBe(false);
    expect(hasRole(me(), "SHOP_OWNER")).toBe(false);
  });

  it("lists one place per role, admin first", () => {
    const who = me({ roles: [roleOf("SHOP_OWNER", "s1"), roleOf("SUPER_ADMIN"), roleOf("SHOP_DRIVER", "s2")] });
    expect(consoleLinks(who).map((l) => l.href)).toEqual(["/console/admin", "/console/shop/s1/orders", "/console/driver"]);
  });

  it("gives a customer nothing", () => {
    expect(consoleLinks(me())).toEqual([]);
  });
});
