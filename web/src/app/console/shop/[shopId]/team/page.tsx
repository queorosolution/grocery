import { TeamScreen } from "@/features/console-shop/TeamScreen";

// One placeholder file; Cloudflare rewrites /console/shop/<any id>/team to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ shopId: "_" }];
}

export default function Page() {
  return <TeamScreen />;
}
