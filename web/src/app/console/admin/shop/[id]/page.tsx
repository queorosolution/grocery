import { ShopPage } from "@/features/admin/shops/ShopPage";

// One placeholder file; Cloudflare rewrites /console/admin/shop/<any id> to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ id: "_" }];
}

export default function Page() {
  return <ShopPage />;
}
