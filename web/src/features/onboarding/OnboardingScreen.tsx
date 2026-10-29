"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { Spinner } from "@/components/ui";
import { useAddresses } from "@/features/address/api";
import { useMe } from "@/features/auth/api";
import { RequireUser } from "@/features/auth/guards";
import { PhoneVerify } from "@/features/auth/PhoneVerify";
import type { Me } from "@/features/auth/types";
import { Page } from "@/features/shell/Page";
import { t, type MessageKey } from "@/lib/i18n";
import { safeNext } from "@/lib/safeUrl";
import { AddressStep } from "./AddressStep";
import { NameStep } from "./NameStep";

type Step = "name" | "phone" | "address";

const titles: Record<Step, MessageKey> = { name: "onboarding.nameTitle", phone: "onboarding.phoneTitle", address: "onboarding.addressTitle" };

/** Only the steps this person still needs. */
function neededSteps(me: Me, addressCount: number): Step[] {
  const steps: Step[] = [];
  if (!me.phoneVerified || !me.name.trim()) steps.push("name");
  if (!me.phoneVerified) steps.push("phone");
  if (addressCount === 0) steps.push("address");
  return steps;
}

function Wizard({ initial, next }: { initial: Step[]; next: string }) {
  const router = useRouter();
  const { me } = useMe();
  // Fixed when the wizard opens: finishing the phone step must not remove the steps after it.
  const [steps] = useState(initial);
  const [index, setIndex] = useState(0);
  const step = steps[index];
  const advance = () => (index + 1 < steps.length ? setIndex(index + 1) : router.replace(next));

  useEffect(() => {
    if (steps.length === 0) router.replace(next); // already set up: nothing to do here
  }, [steps.length, next, router]);

  if (!step || !me) return <Spinner />;
  return (
    <Page className="max-w-md">
      <p className="text-sm text-muted">{t("onboarding.progress", { step: index + 1, total: steps.length })}</p>
      <h1 className="mt-1 mb-4 text-xl font-semibold md:text-2xl">{t(titles[step])}</h1>
      {step === "name" && <NameStep me={me} onDone={advance} />}
      {step === "phone" && <PhoneVerify onVerified={advance} />}
      {step === "address" && <AddressStep onDone={advance} />}
    </Page>
  );
}

function Steps() {
  const { me } = useMe();
  const addresses = useAddresses();
  const next = safeNext(useSearchParams().get("next"));
  if (!me || !addresses.data) return <div className="flex min-h-[50dvh] items-center justify-center"><Spinner /></div>;
  return <Wizard initial={neededSteps(me, addresses.data.length)} next={next} />;
}

export function OnboardingScreen() {
  return (
    <Suspense>
      <RequireUser>
        <Steps />
      </RequireUser>
    </Suspense>
  );
}
