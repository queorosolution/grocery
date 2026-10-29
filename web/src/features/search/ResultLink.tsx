import Link from "next/link";
import { Badge, Price, Thumb } from "@/components/ui";
import { t } from "@/lib/i18n";
import type { SearchProduct } from "./types";

/** A product found across shops. It opens the shop that sells it, where it can be added. */
export function ResultLink({ product: p, shopName }: { product: SearchProduct; shopName?: string }) {
  const body = (
    <>
      <Thumb src={p.imageUrl} className="w-full" />
      <div className="min-h-[3.5rem] min-w-0">
        <div className="line-clamp-2 text-base">{p.name}</div>
        <div className="truncate text-sm text-muted">{shopName ?? p.shopSlug}</div>
      </div>
      <Price cents={p.onlinePriceCents} mrpCents={p.mrpCents} />
      {!p.available && <Badge>{t("product.unavailable")}</Badge>}
    </>
  );
  const cls = "flex flex-col gap-2 rounded-card border border-line bg-bg p-3 md:hover:shadow-raise";
  return p.shopSlug ? <Link href={`/s/${p.shopSlug}`} className={cls}>{body}</Link> : <div className={cls}>{body}</div>;
}
