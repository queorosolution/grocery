"use client";

import { useRouter } from "next/navigation";
import { useRef, useState } from "react";
import { Banner, Button, Field, Skeleton, Textarea } from "@/components/ui";
import { useAddresses } from "@/features/address/api";
import { setMe } from "@/features/auth/api";
import { PhoneVerify } from "@/features/auth/PhoneVerify";
import { useCart } from "@/features/cart/cartContext";
import { useShop } from "@/features/shop/api";
import type { SubstitutionPref } from "@/lib/enums.gen";
import { errorText } from "@/lib/errorText";
import { newIdempotencyKey } from "@/lib/idempotencyKey";
import { t } from "@/lib/i18n";
import { useDebounced } from "@/lib/useDebounced";
import { useQueryClient } from "@tanstack/react-query";
import { billBody, useQuote, useSendBill, type BillRequest } from "./api";
import { problemsOf } from "./problems";
import { QuoteLines } from "./QuoteLines";
import { SlotPicker } from "./SlotPicker";
import type { Destination } from "./types";
import { WherePicker, type WhereChoice } from "./WherePicker";

const TYPING_MS = 600; // a pause after the last letter of "another place" before the bill is asked for again
const MAX_NOTE = 300;

export function CheckoutForm({ shopSlug }: { shopSlug: string }) {
  const router = useRouter();
  const client = useQueryClient();
  const cart = useCart();
  const ref = cart.ref!;
  const shop = useShop(shopSlug);
  const addresses = useAddresses();
  const [choice, setChoice] = useState<WhereChoice | null>(null);
  const [destination, setDestination] = useState<Destination | null>(null);
  const settled = useDebounced(destination, TYPING_MS);
  const [slotId, setSlotId] = useState<string | null>(null);
  const [prefs, setPrefs] = useState<Record<string, SubstitutionPref>>({});
  const [note, setNote] = useState("");
  const attempt = useRef<{ body: string; key: string } | null>(null);
  const send = useSendBill();

  // The main address unless the person chose something else. Without any address, "another place".
  const main = addresses.data?.find((a) => a.isDefault) ?? addresses.data?.[0];
  const picked: WhereChoice | null = choice ?? (main ? { kind: "saved", id: main.id } : addresses.isSuccess ? { kind: "other" } : null);
  const where = picked?.kind === "saved" ? { addressId: picked.id } : picked && destination && settled ? { destination: settled } : null;

  const itemIds = cart.lines.flatMap((l) => (l.itemId ? [l.itemId] : []));
  const needsSlot = shop.data?.deliveryType === "SLOT";
  const request: BillRequest | null = where && {
    shopId: ref.shopId, cartId: ref.cartId, where, slotId: needsSlot ? slotId : null, prefs,
    cartSignature: cart.lines.map((l) => `${l.itemId}:${l.quantity}`).join(","),
  };
  const quote = useQuote(request, itemIds);
  const problems = quote.data ? problemsOf(quote.data, needsSlot === true && !slotId) : [];

  const submit = () => {
    if (!request) return;
    // The same bill pressed twice reuses its key; a changed bill gets a new one (docs/06 section 1).
    const body = JSON.stringify(billBody(request, itemIds)) + note;
    if (attempt.current?.body !== body) attempt.current = { body, key: newIdempotencyKey() };
    send.mutate({ request, itemIds, note, key: attempt.current.key }, {
      onSuccess: (order) => {
        attempt.current = null;
        void cart.clear().then(() => router.push(`/order/${order.id}`));
      },
    });
  };

  const phoneMissing = quote.data?.checks.phoneVerified === false;
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6">
      <WherePicker addresses={addresses.data ?? []} choice={picked ?? { kind: "other" }} onChoice={(c) => { setChoice(c); if (c.kind === "saved") setDestination(null); }} onDestination={setDestination} />
      {quote.data && needsSlot && <SlotPicker slots={quote.data.slots} value={slotId} onChange={setSlotId} />}
      {quote.isError && <Banner tone="danger">{errorText(quote.error)}</Banner>}
      {quote.isFetching && !quote.data && <Skeleton className="h-48 w-full" />}
      {quote.data && <QuoteLines quote={quote.data} prefs={prefs} onPref={(id, p) => setPrefs((all) => ({ ...all, [id]: p }))} />}
      {problems.length > 0 && <Banner tone="warning"><ul>{problems.map((p) => <li key={p}>{t(p)}</li>)}</ul></Banner>}
      {phoneMissing && (
        <section aria-labelledby="phone" className="flex flex-col gap-2">
          <h2 id="phone" className="text-lg font-semibold">{t("checkout.phoneTitle")}</h2>
          <p className="text-muted">{t("checkout.phoneBody")}</p>
          <PhoneVerify onVerified={(me) => { setMe(client, me); void quote.refetch(); }} />
        </section>
      )}
      <Field label={t("checkout.note")} hint={t("checkout.noteHint")}>
        {(p) => <Textarea {...p} maxLength={MAX_NOTE} value={note} onChange={(e) => setNote(e.target.value)} />}
      </Field>
      {send.isError && <Banner tone="danger">{errorText(send.error)}</Banner>}
      <div className="sticky bottom-0 -mx-4 border-t border-line bg-bg p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        <Button size="lg" block loading={send.isPending} disabled={!quote.data || problems.length > 0 || phoneMissing} onClick={submit}>
          {t("checkout.send")}
        </Button>
        <p className="mt-1 text-center text-sm text-muted">{t("checkout.afterSend")}</p>
      </div>
    </div>
  );
}
