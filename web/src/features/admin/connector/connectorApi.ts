import { ApiError } from "@/lib/api";
import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { Connector, Mapping, PreviewRow, SyncRun } from "../types";

const one = (shopId: string) => ["connector", shopId] as const;

/** The shop's connector, or null when it has none yet (the API answers 404 then). */
export function useConnector(shopId: string) {
  const q = useAdminQuery<Connector>(one(shopId), `/shops/${shopId}/connector`);
  const none = q.error instanceof ApiError && q.error.status === 404;
  return { ...q, data: none ? null : q.data, isError: q.isError && !none };
}

export const useCreateConnector = (shopId: string) =>
  useAdminMutation((connectionString: string) => adminCall<Connector>("POST", `/shops/${shopId}/connector`, { connectionString }), [one(shopId)]);

export const useReplaceSecret = (id: string) =>
  useAdminMutation((connectionString: string) => adminCall("PUT", `/connectors/${id}/secret`, { connectionString }), []);

export const useTestConnector = (id: string) =>
  useAdminMutation(() => adminCall<{ ok: boolean; readOnly: boolean; serverVersion: string; warnings: string[] }>("POST", `/connectors/${id}/test`), []);

export const useTables = (id: string, enabled: boolean) =>
  useAdminQuery<{ data: { schema: string; name: string; kind: string }[] }>(["connector-tables", id], `/connectors/${id}/tables`, { enabled });

export const useColumns = (id: string, table: string) =>
  useAdminQuery<{ data: { name: string; type: string }[] }>(["connector-columns", id], `/connectors/${id}/columns`, { query: { table }, enabled: table !== "" });

export const useSaveMapping = (shopId: string, id: string) =>
  useAdminMutation((m: Mapping) => adminCall("PUT", `/connectors/${id}/mapping`, m), [one(shopId)]);

export const usePreview = (id: string) =>
  useAdminMutation(() => adminCall<{ rows: PreviewRow[] }>("POST", `/connectors/${id}/preview`), []);

export const useSetLive = (shopId: string, id: string) =>
  useAdminMutation((live: boolean) => adminCall("POST", `/connectors/${id}/${live ? "activate" : "pause"}`), [one(shopId), ["runs", id]]);

export const useSyncNow = (id: string) =>
  useAdminMutation(() => adminCall("POST", `/connectors/${id}/sync`), [["runs", id]]);

/** The secret in the answer is shown once. Calling `reset()` on this mutation drops it from memory. */
export const useRotateTrigger = (id: string) =>
  useAdminMutation(() => adminCall<{ secret: string; example: string }>("POST", `/connectors/${id}/trigger-secret`), []);

export const useRuns = (id: string) => useAdminQuery<{ data: SyncRun[] }>(["runs", id], `/connectors/${id}/runs`);
