"use client";

import { useState } from "react";
import { Banner, Button, Field, Modal, QtyStepper, Textarea } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useReview } from "./api";
import { buildChanges, MAX_QTY, reviseProblem, type LineEdit } from "./reviseChanges";
import { SubstitutePicker } from "./SubstitutePicker";
import type { ShopOrder } from "./types";

const problemText: Record<"none" | "all" | "qty", MessageKey> = {
  none: "shopConsole.reviseNone", all: "shopConsole.reviseAll", qty: "shopConsole.reviseQty",
};

/** Change the bill: a new quantity, remove a line, or send something else. The server prices it again. */
export function ReviseEditor({ shopId, order, open, onClose }: { shopId: string; order: ShopOrder; open: boolean; onClose: () => void }) {
  const review = useReview(shopId, order.id);
  const [edits, setEdits] = useState<Record<string, LineEdit>>({});
  const [reason, setReason] = useState("");
  const [picking, setPicking] = useState<string | null>(null);
  const lines = order.lines.filter((l) => l.state === "ACTIVE");
  const edit = (id: string, over: Partial<LineEdit>) => {
    const line = lines.find((l) => l.id === id)!;
    setEdits((all) => ({ ...all, [id]: { qty: line.qty, remove: false, substitute: null, ...all[id], ...over } }));
  };
  const changes = buildChanges(lines, edits);
  const problem = reviseProblem(lines, changes);
  const send = () =>
    review.mutate({ version: order.version, action: "REVISE", changes, ...(reason.trim() ? { reason: reason.trim() } : {}) }, { onSuccess: onClose });

  return (
    <Modal open={open} onClose={onClose} title={t("shopConsole.reviseTitle")}
      footer={<Button className="flex-1" loading={review.isPending} disabled={problem !== null} onClick={send}>{t("shopConsole.reviseSend")}</Button>}>
      <ul className="divide-y divide-line">
        {lines.map((l) => {
          const e = edits[l.id];
          return (
            <li key={l.id} className="flex flex-col gap-2 py-3">
              <div className="flex items-baseline justify-between gap-2">
                <span className={e?.remove ? "line-through text-muted" : ""}>{l.name}{e?.substitute && <> → <strong>{e.substitute.name}</strong></>}</span>
                {l.unitPriceCents !== null && <span className="shrink-0 text-sm text-muted">{formatLKR(l.unitPriceCents)}</span>}
              </div>
              {!e?.remove && (
                <QtyStepper value={e?.qty ?? l.qty} min={1} max={MAX_QTY} onChange={(qty) => edit(l.id, { qty })} />
              )}
              <div className="flex gap-2">
                <Button variant="secondary" aria-pressed={e?.remove ?? false} onClick={() => edit(l.id, { remove: !e?.remove, substitute: null })}>
                  {e?.remove ? t("shopConsole.undo") : t("shopConsole.remove")}
                </Button>
                {!e?.remove && <Button variant="secondary" onClick={() => setPicking(l.id)}>{t("shopConsole.substitute")}</Button>}
              </div>
            </li>
          );
        })}
      </ul>
      <Field className="mt-3" label={t("shopConsole.reason")} hint={t("shopConsole.reasonHint")}>
        {(p) => <Textarea {...p} maxLength={300} value={reason} onChange={(e) => setReason(e.target.value)} />}
      </Field>
      {problem && changes.length > 0 && <p role="alert" className="mt-2 text-sm text-danger">{t(problemText[problem])}</p>}
      {review.error && <Banner tone="danger" className="mt-2">{errorText(review.error)}</Banner>}
      <SubstitutePicker shopId={shopId} open={picking !== null} onClose={() => setPicking(null)}
        onPick={(p) => { if (picking) edit(picking, { substitute: p, remove: false }); setPicking(null); }} />
    </Modal>
  );
}
