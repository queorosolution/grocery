import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { Address, AddressInput, AddressResult } from "./types";

export function useAddresses(enabled = true) {
  return useQuery({ queryKey: keys.addresses(), queryFn: () => api<{ data: Address[] }>("/me/addresses").then((r) => r.data), enabled });
}

// A new default changes the other rows too, so these ask again instead of patching one row.
function useAddressMutation<V, R>(call: (v: V) => Promise<R>) {
  const client = useQueryClient();
  return useMutation({ mutationFn: call, onSuccess: () => client.invalidateQueries({ queryKey: keys.addresses() }) });
}

export const useAddAddress = () =>
  useAddressMutation((input: AddressInput) => api<AddressResult>("/me/addresses", { method: "POST", body: payload(input) }));

export const useUpdateAddress = () =>
  useAddressMutation((v: { id: string; input: AddressInput }) =>
    api<AddressResult>(`/me/addresses/${encodeURIComponent(v.id)}`, { method: "PATCH", body: payload(v.input) }));

export const useDeleteAddress = () =>
  useAddressMutation((id: string) => api<void>(`/me/addresses/${encodeURIComponent(id)}`, { method: "DELETE" }));

// An empty landmark is sent as "" so an edit can clear it; the API reads "" as "no landmark".
const payload = (i: AddressInput) => ({ ...i, landmark: i.landmark.trim(), label: i.label.trim(), line1: i.line1.trim() });
