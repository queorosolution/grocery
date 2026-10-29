// A shop picks its colour, so it is checked before it goes into a style attribute.

const HEX = /^#[0-9a-fA-F]{6}$/;

/** The colour when it is a plain #rrggbb, else null. */
export function safeColor(value: string | null | undefined): string | null {
  return value && HEX.test(value) ? value : null;
}

/** White or near-black text, whichever reads better on `hex` (WCAG relative luminance). */
export function textOn(hex: string): "#ffffff" | "#222222" {
  const channel = (i: number) => {
    const c = parseInt(hex.slice(1 + i * 2, 3 + i * 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const luminance = 0.2126 * channel(0) + 0.7152 * channel(1) + 0.0722 * channel(2);
  return luminance > 0.4 ? "#222222" : "#ffffff";
}
