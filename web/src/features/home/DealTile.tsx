import Link from "next/link";
import { Price, Thumb } from "@/components/ui";
import type { Deal } from "./types";

/** A product sold under its printed price. Opens the shop that sells it. */
export function DealTile({ deal }: { deal: Deal }) {
  return (
    <Link href={`/s/${deal.shopSlug}`}
      className="flex w-40 shrink-0 flex-col gap-2 rounded-card border border-line bg-bg p-3 md:hover:shadow-raise">
      <Thumb src={deal.imageUrl} className="w-full" />
      <div className="min-w-0">
        <div className="line-clamp-2 text-base">{deal.name}</div>
        <div className="truncate text-sm text-muted">{deal.shopName}</div>
      </div>
      <Price cents={deal.onlinePriceCents} mrpCents={deal.mrpCents} showSave />
    </Link>
  );
}
