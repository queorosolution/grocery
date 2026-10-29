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
