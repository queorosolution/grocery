// The shapes of /me/addresses (docs/06 section 5).

export interface Address {
  id: string;
  label: string;
  line1: string;
  landmark: string | null;
  lat: number;
  lng: number;
  isDefault: boolean;
}

export interface AddressInput {
  label: string;
  line1: string;
  landmark: string;
  lat: number;
  lng: number;
  isDefault: boolean;
}

/** A shop that delivers to the pin, nearest first. */
export interface Reachable {
  slug: string;
  name: string;
  distanceKm: number;
}

export interface AddressResult {
  address: Address;
  deliveringShops: Reachable[];
}

/** The API refuses a sixth address (`fields.addresses = "max_5"`). */
export const MAX_ADDRESSES = 5;
