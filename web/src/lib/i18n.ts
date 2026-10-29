import en from "@/i18n/en.json";

// All user text lives in i18n/en.json. A Sinhala or Tamil file can be added later
// without touching components (docs/08 section 10).

type Messages = typeof en;
type Flatten<T, P extends string = ""> = {
  [K in keyof T & string]: T[K] extends string ? `${P}${K}` : Flatten<T[K], `${P}${K}.`>;
}[keyof T & string];

export type MessageKey = Flatten<Messages>;

function lookup(key: string): string | undefined {
  let node: unknown = en;
  for (const part of key.split(".")) {
    if (typeof node !== "object" || node === null) return undefined;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === "string" ? node : undefined;
}

/** t("bill.sendToShop") or t("cart.items", { count: 3 }) for "{count} items". */
export function t(key: MessageKey, params: Record<string, string | number> = {}): string {
  const text = lookup(key) ?? key;
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => (name in params ? String(params[name]) : whole));
}
