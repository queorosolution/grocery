"use client";

import { Banner, Modal } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t } from "@/lib/i18n";
import { useGoogleSignIn } from "./api";
import { GoogleButton } from "./GoogleButton";
import type { Me } from "./types";

interface SignInModalProps {
  open: boolean;
  /** A team invite from the link the owner passed on; it is sent with the Google token. */
  invite: string | null;
  onClose: () => void;
  onSignedIn: (me: Me) => void;
}

/** Sign-in is a modal, not a page (docs/08 section 4). Google's button gives us an ID token. */
export function SignInModal({ open, invite, onClose, onSignedIn }: SignInModalProps) {
  const signIn = useGoogleSignIn();
  const onToken = (idToken: string) => signIn.mutate({ idToken, inviteToken: invite ?? undefined }, { onSuccess: (me) => onSignedIn(me) });
  return (
    <Modal open={open} onClose={onClose} title={invite ? t("auth.inviteTitle") : t("auth.title")}>
      <div className="flex flex-col gap-3">
        <p className="text-muted">{invite ? t("auth.inviteBody") : t("auth.body")}</p>
        <GoogleButton onToken={onToken} busy={signIn.isPending} />
        {signIn.isPending && <p role="status" className="text-center text-sm text-muted">{t("auth.checking")}</p>}
        {signIn.error && <Banner tone="danger">{errorText(signIn.error)}</Banner>}
        <p className="text-center text-xs text-muted">{t("auth.privacy")}</p>
      </div>
    </Modal>
  );
}
