import { ProductsScreen } from "@/features/console-shop/ProductsScreen";

// One placeholder file; Cloudflare rewrites /console/shop/<any id>/products to it (docs/08 section 11.2).
export function generateStaticParams() {
  return [{ shopId: "_" }];
}

export default function Page() {
  return <ProductsScreen />;
}
