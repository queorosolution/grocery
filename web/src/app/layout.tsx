import type { CSSProperties } from "react";
import type { Metadata, Viewport } from "next";
import { env } from "@/lib/env";
import { t } from "@/lib/i18n";
import { Providers } from "./providers";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: env.platformName, template: `%s · ${env.platformName}` },
  description: t("app.tagline"),
  applicationName: env.platformName,
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: env.accent,
};

// NEXT_PUBLIC_ACCENT overrides the platform accent without a code change (docs/13).
const accent = { "--c-accent": env.accent } as CSSProperties;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body style={accent} className="min-h-dvh bg-bg text-ink antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
