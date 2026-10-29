"use client";

import { useState } from "react";
import { Button, Input, Modal, Spinner } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useDebounced } from "@/lib/useDebounced";
import { useShopProducts } from "./productsApi";

interface Props {
  shopId: string;
  open: boolean;
  onPick: (p: { shopProductId: string; name: string }) => void;
  onClose: () => void;
}

/** Search the shop's own products for something to send instead. Only products that can be sold are offered. */
export function SubstitutePicker({ shopId, open, onPick, onClose }: Props) {
  const [text, setText] = useState("");
  const q = useDebounced(text, 300);
  const found = useShopProducts(shopId, q, true, open && q.trim().length >= 2);
  const rows = found.data?.pages.flatMap((p) => p.data) ?? [];
  return (
    <Modal open={open} onClose={onClose} title={t("shopConsole.substituteTitle")}>
      <Input aria-label={t("shopConsole.substituteSearch")} placeholder={t("shopConsole.substituteSearch")} value={text} onChange={(e) => setText(e.target.value)} />
      <div className="mt-3 flex flex-col gap-1" aria-live="polite">
        {found.isFetching && <Spinner />}
        {!found.isFetching && q.trim().length >= 2 && rows.length === 0 && <p className="text-muted">{t("search.noneTitle")}</p>}
        {rows.map((p) => (
          <Button key={p.id} variant="ghost" className="h-auto justify-between py-2 text-left" onClick={() => onPick({ shopProductId: p.id, name: p.name })}>
            <span className="min-w-0 truncate">{p.name}{p.sizeLabel ? ` · ${p.sizeLabel}` : ""}</span>
            <span className="shrink-0">{formatLKR(p.onlinePriceCents)}</span>
          </Button>
        ))}
      </div>
    </Modal>
  );
}
