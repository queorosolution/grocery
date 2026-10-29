import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { Bootstrap, Me, TotpSetup } from "./types";

const FIVE_MINUTES = 5 * 60_000;

/** One call per app load; `me` and the platform settings come from it (docs/08 section 5). */
export function useBootstrap() {
  return useQuery({ queryKey: keys.bootstrap(), queryFn: () => api<Bootstrap>("/bootstrap"), staleTime: FIVE_MINUTES });
}

/** The signed-in user, `null` when signed out. `loading` is true only until bootstrap first answers. */
export function useMe(): { me: Me | null; loading: boolean; failed: boolean; retry: () => void } {
  const q = useBootstrap();
  return { me: q.data?.me ?? null, loading: q.isPending, failed: q.isError, retry: () => void q.refetch() };
}

/** Writes a fresh `me` from a response into the cache instead of asking again (docs/08 section 5 rule 4). */
export function setMe(client: QueryClient, me: Me | null): void {
  client.setQueryData<Bootstrap>(keys.bootstrap(), (old) => (old ? { ...old, me } : old));
}

/** After sign-out or a lost session nothing of the last person may stay in the cache. */
export function forgetUser(client: QueryClient): void {
  setMe(client, null);
  client.removeQueries({ predicate: (q) => q.queryKey[0] !== keys.bootstrap()[0] });
}

export function useGoogleSignIn() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (v: { idToken: string; inviteToken?: string }) => api<Me>("/auth/google", { method: "POST", body: v }),
    onSuccess: (me) => setMe(client, me),
  });
}

export function useLogout() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (all: boolean) => api<void>("/auth/logout", { method: "POST", body: { all } }),
    onSettled: () => forgetUser(client),
  });
}

export function useTotpSetup() {
  return useMutation({ mutationFn: () => api<TotpSetup>("/auth/totp/setup", { method: "POST" }) });
}

export function useTotpVerify() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (code: string) => api<Me>("/auth/totp/verify", { method: "POST", body: { code } }),
    onSuccess: (me) => setMe(client, me),
  });
}

// The signed-in person's own details (docs/06 section 5). Each call answers with the new `me`.

export function useUpdateName() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api<Me>("/me", { method: "PATCH", body: { name } }),
    onSuccess: (me) => setMe(client, me),
  });
}

export function usePhoneSend() {
  return useMutation({
    mutationFn: (v: { phone: string; turnstileToken: string }) => api<void>("/me/phone/send", { method: "POST", body: v }),
  });
}

export function usePhoneVerify() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (code: string) => api<Me>("/me/phone/verify", { method: "POST", body: { code } }),
    onSuccess: (me) => setMe(client, me),
  });
}

/** Soft delete (DELETE /me). The API refuses with ACCOUNT_NOT_DELETABLE while anything is open. */
export function useDeleteAccount() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>("/me", { method: "DELETE", body: { confirm: "DELETE" } }),
    onSuccess: () => forgetUser(client),
  });
}
