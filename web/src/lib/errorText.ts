import { ApiError } from "./api";
import { t } from "./i18n";

/** The sentence to show for a failed call: the API's own message for known codes. */
export function errorText(err: unknown): string {
  return err instanceof ApiError ? err.message : t("common.error");
}

/** Field errors from a 400 (`fields.phone = "invalid"`), as `{ phone: "..." }` text for the form. */
export function fieldErrors(err: unknown, texts: Readonly<Record<string, string>>): Record<string, string> {
  if (!(err instanceof ApiError)) return {};
  return Object.fromEntries(Object.entries(err.fields).map(([name, code]) => [name, texts[`${name}.${code}`] ?? texts[name] ?? err.message]));
}
