import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type RequestOptions } from "@/lib/api";
import { keys } from "@/lib/keys";

type Method = "POST" | "PUT" | "PATCH" | "DELETE";

/** Every admin call lives under /admin (docs/06 section 10). */
export const adminCall = <R = unknown>(method: Method, path: string, body?: unknown) =>
  api<R>(`/admin${path}`, { method, body });

interface QueryOptions {
  query?: RequestOptions["query"];
  enabled?: boolean;
}

/** A GET under /admin. `parts` names the data in the cache; the query text is added to the key. */
export function useAdminQuery<T>(parts: readonly unknown[], path: string, { query, enabled }: QueryOptions = {}) {
  return useQuery({
    queryKey: keys.admin(...parts, query ?? null),
    queryFn: ({ signal }) => api<T>(`/admin${path}`, { query, signal }),
    enabled,
    staleTime: 15_000,
  });
}

/**
 * A change under /admin. After it works, every list whose key starts with one of `refresh` is asked
 * for again, so a screen never shows what it just changed in its old form.
 */
export function useAdminMutation<V, R = unknown>(run: (v: V) => Promise<R>, refresh: readonly (readonly unknown[])[]) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => Promise.all(refresh.map((parts) => client.invalidateQueries({ queryKey: keys.admin(...parts) }))),
  });
}
