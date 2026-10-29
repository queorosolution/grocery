/** A fresh Idempotency-Key: 36 characters of 0-9 a-f and "-", inside the API's 16 to 64 (docs/06 section 1). */
export const newIdempotencyKey = (): string => crypto.randomUUID();
