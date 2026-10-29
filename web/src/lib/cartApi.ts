import { env } from "./env";

// The ONE place the browser calls the cart service (docs/08 section 6). It uses the shop's
// public key, which the cart service only accepts from our own origin. The platform key
// never reaches the browser. Prices here are an estimate: the bill uses our database.

export interface CartLine {
  id: string;
  itemId: string | null;
  name: string;
  unitPrice: number;
  quantity: number;
  lineTotal: number;
}

export interface Cart {
  id: string;
  status: "open" | "checked_out";
  lines: CartLine[];
  subtotal: number;
  itemCount: number;
}

export class CartError extends Error {
  constructor(readonly status: number, readonly code: string) {
    super(`cart service: ${code}`);
    this.name = "CartError";
  }
}

async function call<T>(publicKey: string, method: string, path: string, body?: unknown): Promise<T> {
  if (!env.cartApiBase) throw new CartError(0, "NOT_CONFIGURED");
  let res: Response;
  try {
    res = await fetch(env.cartApiBase + path, {
      method,
      headers: {
        Authorization: `Bearer ${publicKey}`,
        Accept: "application/json",
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new CartError(0, "NETWORK");
  }
  if (!res.ok) {
    const failure = (await res.json().catch(() => null)) as { error?: { code?: string } } | null;
    throw new CartError(res.status, failure?.error?.code ?? "UNKNOWN");
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

const cartPath = (cartId: string) => `/carts/${encodeURIComponent(cartId)}`;

export const cartApi = {
  create: (key: string) => call<Cart>(key, "POST", "/carts"),
  get: (key: string, cartId: string) => call<Cart>(key, "GET", cartPath(cartId)),
  addLine: (key: string, cartId: string, itemId: string, quantity: number) =>
    call<Cart>(key, "POST", `${cartPath(cartId)}/lines`, { itemId, quantity }),
  /** Quantity 0 removes the line. */
  setQuantity: (key: string, cartId: string, lineId: string, quantity: number) =>
    call<Cart>(key, "PATCH", `${cartPath(cartId)}/lines/${encodeURIComponent(lineId)}`, { quantity }),
  discard: (key: string, cartId: string) => call<void>(key, "DELETE", cartPath(cartId)),
};
