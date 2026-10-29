// Links built from data our API sends: only digits and a leading plus go into tel:, only numbers into the map.

export const dialable = (phone: string) => phone.replace(/[^\d+]/g, "");

/** A Google Maps link for a pin, or null when the address has no pin. */
export function mapUrl(lat?: number, lng?: number): string | null {
  return Number.isFinite(lat) && Number.isFinite(lng) ? `https://www.google.com/maps/search/?api=1&query=${lat},${lng}` : null;
}
