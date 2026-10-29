import { Banner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";

/** A whole number from a text box, or null when it is not one (empty, minus, letters, a decimal point). */
export function parseInt10(text: string): number | null {
  return /^\d{1,9}$/.test(text.trim()) ? Number(text.trim()) : null;
}

/** A number with at most 2 decimals ("1.5", "0.25") for km and kg. Never money: money goes through parseRupees. */
export function parseDec2(text: string): number | null {
  return /^\d{1,6}(\.\d{1,2})?$/.test(text.trim()) ? Number(text.trim()) : null;
}

const FIELD_TEXT: Readonly<Record<string, MessageKey>> = {
  taken: "admin.field.taken",
  unknown: "admin.field.unknown",
  missing: "admin.field.missing",
  not_allowed: "admin.field.notAllowed",
  overlaps: "admin.field.overlaps",
  used: "admin.field.used",
};

/** The sentence for one field that the API refused (`fields.slug = "taken"`), or undefined when it did not. */
export function fieldText(error: unknown, name: string): string | undefined {
  if (!(error instanceof ApiError)) return undefined;
  const code = error.fields[name];
  if (code === undefined) return undefined;
  return t(FIELD_TEXT[code] ?? "admin.field.invalid");
}

/** The words under a form after a save: why it failed, unless the fields already say it. */
export function SaveNotice({ error, saved }: { error: unknown; saved: boolean }) {
  if (error) {
    const named = error instanceof ApiError && Object.keys(error.fields).length > 0;
    return <Banner tone="danger">{named ? t("admin.checkFields") : errorText(error)}</Banner>;
  }
  return saved ? <p role="status" className="text-sm text-success">{t("admin.saved")}</p> : null;
}

/** A percent typed as text ("12.5") to basis points (1250), by splitting the text: no floating point. Null outside 0.01 to 100. */
export function parsePercentBp(text: string): number | null {
  const m = /^(\d{1,3})(?:\.(\d{1,2}))?$/.exec(text.trim());
  if (!m) return null;
  const bp = Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0"));
  return bp >= 1 && bp <= 10000 ? bp : null;
}

/** Basis points as percent text without floating point: 1250 -> "12.5", 1205 -> "12.05", 1200 -> "12". */
export function bpToText(bp: number): string {
  const rest = bp % 100;
  return `${Math.trunc(bp / 100)}${rest === 0 ? "" : `.${String(rest).padStart(2, "0").replace(/0$/, "")}`}`;
}
