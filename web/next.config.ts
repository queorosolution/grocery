import type { NextConfig } from "next";

// "output: export" is only for `next build`. In `next dev` dynamic routes work
// normally, so /order/<real id> can be opened locally (docs/08 section 11.3).
const config: NextConfig = {
  ...(process.env.NODE_ENV === "production" ? { output: "export" as const } : {}),
  trailingSlash: true,
  images: { unoptimized: true },
  reactStrictMode: true,
};

export default config;
