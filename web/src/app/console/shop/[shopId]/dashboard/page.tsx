import { DashboardScreen } from "@/features/console-shop/DashboardScreen";

// One placeholder file; Cloudflare rewrites /console/shop/<any id>/dashboard to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ shopId: "_" }];
}

export default function Page() {
  return <DashboardScreen />;
}
