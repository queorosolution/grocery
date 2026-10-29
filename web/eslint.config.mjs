import coreWebVitals from "eslint-config-next/core-web-vitals";
import typescript from "eslint-config-next/typescript";

// Private id pages only. The public shop page may use params at build time (metadata).
const noParams = {
  selector: "Identifier[name='params']",
  message:
    "Do not use `params` in pages: after a Cloudflare rewrite it is the placeholder `_`. Read ids with useRouteId() (docs/08 section 11.2).",
};

const config = [
  { ignores: [".next/**", "out/**", ".generated/**", "next-env.d.ts", "node_modules/**"] },
  ...coreWebVitals,
  ...typescript,
  {
    rules: {
      "react/no-danger": "error",
      "no-eval": "error",
      "no-implied-eval": "error",
      "no-new-func": "error",
      // One fetch wrapper only (docs/08 section 5 rule 1).
      "no-restricted-globals": ["error", { name: "fetch", message: "Use lib/api.ts (or lib/cartApi.ts)." }],
    },
  },
  { files: ["src/lib/api.ts", "src/lib/cartApi.ts", "src/**/*.test.{ts,tsx}", "scripts/**/*.mjs"], rules: { "no-restricted-globals": "off" } },
  { files: ["src/app/**/order/**/page.tsx", "src/app/console/**/page.tsx"], rules: { "no-restricted-syntax": ["error", noParams] } },
  {
    // The design system is pure: no data fetching, no features.
    files: ["src/components/ui/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        { patterns: [{ group: ["@/features/*", "@/lib/api", "@/lib/cartApi"], message: "components/ui must stay pure." }] },
      ],
    },
  },
  { files: ["scripts/**/*.mjs"], languageOptions: { globals: { process: "readonly", console: "readonly", URL: "readonly", fetch: "readonly", AbortSignal: "readonly" } } },
];

export default config;
