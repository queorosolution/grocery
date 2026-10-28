import { existsSync } from "node:fs";

/**
 * Next.js reads .env files for itself but not for these scripts, so load them the same way.
 * Variables that are already set win, as in Next.js.
 */
export function loadEnvFiles() {
  for (const file of [".env.production.local", ".env.local", ".env.production", ".env"]) {
    if (existsSync(file)) process.loadEnvFile(file);
  }
}
