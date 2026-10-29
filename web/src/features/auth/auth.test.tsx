import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { AdminGate, ConsoleGate, RequireUser } from "./guards";
import { SignInModal } from "./SignInModal";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));
vi.mock("@/lib/env", async (orig) => ({ env: { ...(await orig<typeof import("@/lib/env")>()).env, googleClientId: "client-id" } }));

// Google's script is replaced by a function that hands us the "token callback" to call.
let giveToken: (idToken: string) => void = () => {};
vi.mock("./google", () => ({
  renderGoogleButton: async (el: HTMLElement, onToken: (t: string) => void) => {
    giveToken = onToken;
    el.textContent = "Google button";
  },
}));

const openSignIn = vi.fn();
vi.mock("./AuthProvider", () => ({ useAuth: () => ({ openSignIn }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }));

beforeEach(() => openSignIn.mockClear());

describe("SignInModal", () => {
  it("sends the Google token to the API and hands the new me back", async () => {
    const who = me();
    const fake = mockApi({ "POST /auth/google": () => who });
    const done = vi.fn();
    renderScreen(<SignInModal open invite={null} onClose={vi.fn()} onSignedIn={done} />);
    await screen.findByText("Google button");
    giveToken("id-token");
    await waitFor(() => expect(done).toHaveBeenCalledWith(who));
    expect(calls(fake, "POST /auth/google")[0]![1].body).toEqual({ idToken: "id-token", inviteToken: undefined });
  });

  it("sends the team invite with the token", async () => {
    const fake = mockApi({ "POST /auth/google": () => me() });
    renderScreen(<SignInModal open invite={"i".repeat(43)} onClose={vi.fn()} onSignedIn={vi.fn()} />);
    expect(screen.getByText("Join the team")).toBeInTheDocument();
    await screen.findByText("Google button");
    giveToken("id-token");
    await waitFor(() => expect(fake).toHaveBeenCalled());
    expect(calls(fake, "POST /auth/google")[0]![1].body.inviteToken).toBe("i".repeat(43));
  });

  it("shows the API's message when sign-in is refused, and signs nobody in", async () => {
    mockApi({ "POST /auth/google": () => new ApiError(403, "FORBIDDEN", "You can't do that.") });
    const done = vi.fn();
    renderScreen(<SignInModal open invite={null} onClose={vi.fn()} onSignedIn={done} />);
    await screen.findByText("Google button");
    giveToken("bad");
    expect(await screen.findByRole("alert")).toHaveTextContent("You can't do that.");
    expect(done).not.toHaveBeenCalled();
  });
});

describe("RequireUser", () => {
  it("asks a signed-out visitor to sign in and shows nothing of the page", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(null) });
    renderScreen(<RequireUser><p>secret page</p></RequireUser>);
    expect(await screen.findByText("Sign in to continue")).toBeInTheDocument();
    expect(screen.queryByText("secret page")).not.toBeInTheDocument();
    await waitFor(() => expect(openSignIn).toHaveBeenCalledTimes(1));
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(openSignIn).toHaveBeenCalledTimes(2);
  });

  it("shows the page to a signed-in person", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(me()) });
    renderScreen(<RequireUser><p>my page</p></RequireUser>);
    expect(await screen.findByText("my page")).toBeInTheDocument();
    expect(openSignIn).not.toHaveBeenCalled();
  });

  it("offers a retry when the account cannot be loaded", async () => {
    mockApi({ "GET /bootstrap": () => new ApiError(500, "UNKNOWN", "x") });
    renderScreen(<RequireUser><p>my page</p></RequireUser>);
    expect(await screen.findByText("Couldn't load your account")).toBeInTheDocument();
    expect(openSignIn).not.toHaveBeenCalled();
  });
});

describe("ConsoleGate", () => {
  it("keeps a plain customer out of the console", async () => {
    mockApi({ "GET /bootstrap": () => bootstrap(me({ roles: [] })) });
    renderScreen(<ConsoleGate><p>orders queue</p></ConsoleGate>);
    expect(await screen.findByText("This page isn't for your account")).toBeInTheDocument();
    expect(screen.queryByText("orders queue")).not.toBeInTheDocument();
  });

  it("lets a shop member in", async () => {
    const staff = me({ roles: [{ role: "SHOP_STAFF", shopId: "s1", shopSlug: "k", shopName: "K" }] });
    mockApi({ "GET /bootstrap": () => bootstrap(staff) });
    renderScreen(<ConsoleGate><p>orders queue</p></ConsoleGate>);
    expect(await screen.findByText("orders queue")).toBeInTheDocument();
  });
});

describe("AdminGate", () => {
  const admin = (adminTotp: "SETUP_NEEDED" | "CODE_NEEDED" | "OK") =>
    me({ roles: [{ role: "SUPER_ADMIN", shopId: null, shopSlug: null, shopName: null }], adminTotp });

  it("does not open admin screens to a shop owner", async () => {
    const owner = me({ roles: [{ role: "SHOP_OWNER", shopId: "s1", shopSlug: "k", shopName: "K" }] });
    mockApi({ "GET /bootstrap": () => bootstrap(owner) });
    renderScreen(<RequireUser><AdminGate><p>admin dashboard</p></AdminGate></RequireUser>);
    expect(await screen.findByText("This page isn't for your account")).toBeInTheDocument();
  });

  it("asks for the authenticator code before anything else, then opens", async () => {
    let current = admin("CODE_NEEDED");
    const fake = mockApi({
      "GET /bootstrap": () => bootstrap(current),
      "POST /auth/totp/verify": () => (current = admin("OK")),
    });
    renderScreen(<RequireUser><AdminGate><p>admin dashboard</p></AdminGate></RequireUser>);
    expect(await screen.findByText("Enter your code")).toBeInTheDocument();
    expect(screen.queryByText("admin dashboard")).not.toBeInTheDocument();
    const button = screen.getByRole("button", { name: "Continue" });
    expect(button).toBeDisabled(); // needs 6 digits
    await userEvent.type(screen.getByLabelText("6-digit code"), "12a3456");
    await userEvent.click(button);
    expect(await screen.findByText("admin dashboard")).toBeInTheDocument();
    expect(calls(fake, "POST /auth/totp/verify")[0]![1].body).toEqual({ code: "123456" });
  });

  it("shows a wrong code's message next to the box", async () => {
    mockApi({
      "GET /bootstrap": () => bootstrap(admin("CODE_NEEDED")),
      "POST /auth/totp/verify": () => new ApiError(401, "TOTP_INVALID", "That code didn't work."),
    });
    renderScreen(<RequireUser><AdminGate><p>admin dashboard</p></AdminGate></RequireUser>);
    await userEvent.type(await screen.findByLabelText("6-digit code"), "000000");
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByText("That code didn't work.")).toBeInTheDocument();
    expect(screen.queryByText("admin dashboard")).not.toBeInTheDocument();
  });

  it("shows the QR code first when TOTP is not set up, and only trusts an authenticator link", async () => {
    let current = admin("SETUP_NEEDED");
    mockApi({
      "GET /bootstrap": () => bootstrap(current),
      "POST /auth/totp/setup": () => ({ otpauthUri: "otpauth://totp/NearShop:a%40b.lk?secret=ABCDEFGH&issuer=NearShop", secretMasked: "ABCD••••" }),
      "POST /auth/totp/verify": () => (current = admin("OK")),
    });
    renderScreen(<RequireUser><AdminGate><p>admin dashboard</p></AdminGate></RequireUser>);
    await userEvent.click(await screen.findByRole("button", { name: "Show my QR code" }));
    expect(await screen.findByRole("img", { name: "QR code for your authenticator app" })).toBeInTheDocument();
    expect(screen.getByText("Secret: ABCD••••")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open in authenticator app" }).getAttribute("href")).toMatch(/^otpauth:\/\/totp\//);
    await userEvent.type(screen.getByLabelText("6-digit code"), "654321");
    await userEvent.click(screen.getByRole("button", { name: "Turn on" }));
    expect(await screen.findByText("admin dashboard")).toBeInTheDocument();
  });

  it("refuses to draw anything that is not an authenticator link", async () => {
    mockApi({
      "GET /bootstrap": () => bootstrap(admin("SETUP_NEEDED")),
      "POST /auth/totp/setup": () => ({ otpauthUri: "https://evil.example/qr", secretMasked: "x" }),
    });
    renderScreen(<RequireUser><AdminGate><p>admin dashboard</p></AdminGate></RequireUser>);
    await userEvent.click(await screen.findByRole("button", { name: "Show my QR code" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Show my QR code" })).toBeEnabled());
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});
