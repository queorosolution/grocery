import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { Team } from "./types";

const base = (shopId: string) => `/console/shops/${encodeURIComponent(shopId)}/team`;

export function useTeam(shopId: string) {
  return useQuery({ queryKey: keys.shopConsole(shopId, "team"), queryFn: () => api<Team>(base(shopId)), staleTime: 15_000 });
}

function useTeamCall<V, R>(shopId: string, call: (v: V) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: call, onSuccess: () => client.invalidateQueries({ queryKey: keys.shopConsole(shopId, "team") }) });
}

export interface InviteBody {
  email: string;
  role: "SHOP_STAFF" | "SHOP_DRIVER";
  canEditProducts: boolean;
}

/** The token comes back once. Only its hash is stored, so a lost link means a new invite. */
export const useInvite = (shopId: string) =>
  useTeamCall(shopId, (body: InviteBody) => api<{ invite: { id: string }; token: string }>(`${base(shopId)}/invites`, { method: "POST", body }));

export const usePatchMember = (shopId: string) =>
  useTeamCall(shopId, (v: { id: string; role?: InviteBody["role"]; canEditProducts?: boolean }) =>
    api<void>(`${base(shopId)}/${encodeURIComponent(v.id)}`, { method: "PATCH", body: { role: v.role, canEditProducts: v.canEditProducts } }));

export const useRemoveMember = (shopId: string) =>
  useTeamCall(shopId, (id: string) => api<void>(`${base(shopId)}/${encodeURIComponent(id)}`, { method: "DELETE" }));
