// The map box of Sri Lanka. The API refuses a pin outside it (docs/06 section 5), so the form
// checks the same box before asking.

export interface LatLng {
  lat: number;
  lng: number;
}

export const SRI_LANKA = { latMin: 5.8, latMax: 10.0, lngMin: 79.4, lngMax: 82.0 } as const;

/** Where the map opens when nothing is chosen yet. */
export const COLOMBO: LatLng = { lat: 6.9271, lng: 79.8612 };

export function inSriLanka(p: LatLng): boolean {
  return p.lat >= SRI_LANKA.latMin && p.lat <= SRI_LANKA.latMax && p.lng >= SRI_LANKA.lngMin && p.lng <= SRI_LANKA.lngMax;
}
