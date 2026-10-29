import type { Metadata } from "next";
import { ShopScreen } from "@/features/shop/ShopScreen";
import { readShopList } from "@/lib/shopList";

// One file is built for every active shop, so a shared link shows the shop's name
// and logo. A shop that is newer than the build is shown by app/not-found.tsx.
// If the shop list is empty, "_" keeps `output: export` happy.
const PLACEHOLDER = "_";

export async function generateStaticParams() {
  const shops = await readShopList();
  return shops.length > 0 ? shops.map((s) => ({ slug: s.slug })) : [{ slug: PLACEHOLDER }];
}

// Build time only, where `params` is the real slug. The screen itself reads the URL.
export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params;
  const shop = (await readShopList()).find((s) => s.slug === slug);
  if (!shop) return {};
  return {
    title: shop.name,
    openGraph: { title: shop.name, ...(shop.logoUrl ? { images: [shop.logoUrl] } : {}) },
  };
}

export default function Page() {
  return <ShopScreen />;
}
