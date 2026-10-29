import { isAllowedPayHereAction } from "./safeUrl";

/**
 * Sends the person to PayHere with the fields our API signed (docs/08 section 8, XSS-05).
 * The form is built with createElement, never from a string, and may only post to one of the two
 * PayHere checkout addresses. Anything else throws before a form exists.
 */
export function submitPayHere(action: string, fields: Readonly<Record<string, string>>, doc: Document = document): void {
  if (!isAllowedPayHereAction(action)) throw new Error("payment form refused: unknown address");
  const form = doc.createElement("form");
  form.method = "post";
  form.action = action;
  form.style.display = "none";
  for (const [name, value] of Object.entries(fields)) {
    const input = doc.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = String(value);
    form.appendChild(input);
  }
  doc.body.appendChild(form);
  form.submit();
}
