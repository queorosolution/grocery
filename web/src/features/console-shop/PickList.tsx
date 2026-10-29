"use client";

import { useState } from "react";
import { Card, Checkbox } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import type { SubstitutionPref } from "@/lib/enums.gen";
import type { ShopOrder } from "./types";

const prefText: Record<SubstitutionPref, MessageKey> = {
  REMOVE: "checkout.prefRemove", SIMILAR: "checkout.prefSimilar", CALL_ME: "checkout.prefCall",
};

/** A tick-list while picking. The ticks live in this screen only: they help the picker, nothing else reads them. */
export function PickList({ lines }: { lines: ShopOrder["lines"] }) {
  const [done, setDone] = useState<ReadonlySet<string>>(new Set());
  const active = lines.filter((l) => l.state === "ACTIVE" || l.state === "SUBSTITUTE");
  const toggle = (id: string) => setDone((s) => { const n = new Set(s); if (n.has(id)) n.delete(id); else n.add(id); return n; });
  return (
    <Card className="p-4">
      <h2 className="mb-2 text-lg font-semibold">{t("shopConsole.pickList", { done: done.size, total: active.length })}</h2>
      <ul className="divide-y divide-line">
        {active.map((l) => (
          <li key={l.id} className="py-2">
            <Checkbox label={`${l.qty} × ${l.name}`} checked={done.has(l.id)} onChange={() => toggle(l.id)} />
            <p className="ml-8 text-sm text-muted">{t("checkout.ifMissing")} {t(prefText[l.subPref])}</p>
          </li>
        ))}
      </ul>
    </Card>
  );
}
