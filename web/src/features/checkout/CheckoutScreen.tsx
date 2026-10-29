"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { EmptyState, Spinner } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { RequireUser } from "@/features/auth/guards";
import { useCart } from "@/features/cart/cartContext";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { CheckoutForm } from "./CheckoutForm";

/** Who may be here: someone signed in, set up (name, phone step done), with something in the cart. */
function Gate() {
  const { me } = useMe();
  const { ref, lines } = useCart();
  const router = useRouter();
  const setupFirst = me?.needsOnboarding === true;
  useEffect(() => {
    if (setupFirst) router.replace("/onboarding?next=%2Fcheckout");
  }, [setupFirst, router]);

  if (setupFirst) return <div className="flex min-h-[40dvh] items-center justify-center"><Spinner /></div>;
  if (!ref || lines.length === 0) {
    return (
      <EmptyState title={t("cart.emptyTitle")} body={t("cart.emptyBody")}
        action={<Link href="/" className="font-semibold text-accent underline">{t("notFound.home")}</Link>} />
    );
  }
  return <CheckoutForm shopSlug={ref.shopSlug} />;
}

export function CheckoutScreen() {
  return (
    <Page>
      <h1 className="mb-4 text-xl font-semibold md:text-2xl">{t("checkout.title")}</h1>
      <RequireUser><Gate /></RequireUser>
    </Page>
  );
}
