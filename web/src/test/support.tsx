import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { vi, type Mock } from "vitest";
import { ToastProvider } from "@/components/ui";
import type { Bootstrap, Me } from "@/features/auth/types";
import { CartProvider } from "@/features/cart/CartProvider";
import { api } from "@/lib/api";

// Helpers for screen tests. Each test file mocks "@/lib/api" itself (vi.mock is hoisted per file):
//   vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));

export const me = (over: Partial<Me> = {}): Me => ({
  id: "u1", name: "Nimal Perera", email: "nimal@example.com", phoneMasked: "077•••4567", phoneVerified: true,
  needsOnboarding: false, roles: [], adminTotp: "NOT_REQUIRED", ...over,
});

export const bootstrap = (who: Me | null): Bootstrap => ({
  platform: { name: "NearShop", domain: "nearshop.test", supportPhone: "+94770000000", singleShopSlug: null },
  features: { whatsapp: false, manualPayment: true },
  me: who,
});

type Handler = (body: unknown) => unknown;

/** Answers api() calls by "METHOD /path". A call nobody planned for fails the test loudly. */
export function mockApi(handlers: Record<string, Handler>): Mock {
  const fake = api as unknown as Mock;
  fake.mockReset();
  fake.mockImplementation(async (path: string, options?: { method?: string; body?: unknown }) => {
    const key = `${options?.method ?? "GET"} ${path}`;
    const handler = handlers[key];
    if (!handler) throw new Error(`Unplanned API call: ${key}`);
    const out = handler(options?.body);
    if (out instanceof Error) throw out;
    return out;
  });
  return fake;
}

type Call = [string, { method?: string; body: Record<string, unknown>; query?: Record<string, unknown>; signal?: AbortSignal }];

/** The calls made to one "METHOD /path", with the body each one sent. */
export const calls = (fake: Mock, key: string): Call[] =>
  (fake.mock.calls as Call[]).filter(([path, o]) => `${o?.method ?? "GET"} ${path}` === key);

export function renderScreen(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { client, ...render(<QueryClientProvider client={client}><ToastProvider><CartProvider>{ui}</CartProvider></ToastProvider></QueryClientProvider>) };
}

export const noop = vi.fn();
