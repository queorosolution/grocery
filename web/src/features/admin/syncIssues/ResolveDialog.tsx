"use client";

import { useState } from "react";
import { Banner, Button, Field, Input, Modal } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { useDebounced } from "@/lib/useDebounced";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useProductSearch } from "../productsApi";
import type { SyncIssue } from "../types";
import { useResolve, type ResolveBody } from "./api";

/** What the admin can do with one issue. The kind decides: a row with no match, or a price held back. */
export function ResolveDialog({ issue, open, onClose }: { issue: SyncIssue; open: boolean; onClose: () => void }) {
  const resolve = useResolve(issue.id);
  const [term, setTerm] = useState("");
  const q = useDebounced(term, 300);
  const found = useProductSearch(q);
  const p = issue.payload;
  const run = (body: ResolveBody) => resolve.mutate(body, { onSuccess: onClose });
  // A price of 0 or one that could not be read is never approved (the API refuses too).
  const priceOk = p.priceCents !== null && p.priceCents > 0;

  return (
    <Modal open={open} onClose={() => { resolve.reset(); onClose(); }} title={p.name ?? issue.externalId}>
      <div className="flex flex-col gap-3">
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
          <dt className="text-muted">{t("admin.issues.externalId")}</dt><dd className="break-all">{issue.externalId}</dd>
          {p.barcode && <><dt className="text-muted">{t("admin.connector.col.barcode")}</dt><dd>{p.barcode}</dd></>}
          {p.priceCents !== null && <><dt className="text-muted">{t("admin.issues.price")}</dt><dd>{formatLKR(p.priceCents)}</dd></>}
          {p.currentPriceCents !== undefined && <><dt className="text-muted">{t("admin.issues.current")}</dt><dd>{formatLKR(p.currentPriceCents)}</dd></>}
        </dl>
        {issue.kind === "UNMATCHED" ? (
          <>
            <Field label={t("admin.issues.search")}>{(f) => <Input {...f} value={term} onChange={(e) => setTerm(e.target.value)} />}</Field>
            <ul className="flex flex-col gap-1">
              {found.data?.data.slice(0, 8).map((m) => (
                <li key={m.id}>
                  <Button variant="secondary" className="w-full justify-start text-left" loading={resolve.isPending} onClick={() => run({ action: "LINK", productId: m.id })}>
                    {m.name}{m.sizeLabel ? ` · ${m.sizeLabel}` : ""}{m.brand ? ` · ${m.brand}` : ""}
                  </Button>
                </li>
              ))}
            </ul>
            <Button loading={resolve.isPending} onClick={() => run({ action: "CREATE" })}>{t("admin.issues.create")}</Button>
          </>
        ) : (
          <div className="flex gap-2">
            <Button className="flex-1" loading={resolve.isPending} disabled={!priceOk} onClick={() => run({ action: "APPROVE_PRICE" })}>{t("admin.issues.approve")}</Button>
            <Button variant="secondary" className="flex-1" loading={resolve.isPending} onClick={() => run({ action: "REJECT_PRICE" })}>{t("admin.issues.reject")}</Button>
          </div>
        )}
        <Button variant="secondary" loading={resolve.isPending} onClick={() => run({ action: "IGNORE" })}>{t("admin.issues.ignore")}</Button>
        {resolve.error && <Banner tone="danger">{errorText(resolve.error)}</Banner>}
      </div>
    </Modal>
  );
}
