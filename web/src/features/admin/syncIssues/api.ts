import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { SyncIssue } from "../types";

export const useSyncIssues = (status: SyncIssue["status"], shopId: string | null) =>
  useAdminQuery<{ data: SyncIssue[] }>(["sync-issues"], "/sync-issues", { query: { status, shopId } });

export type ResolveBody =
  | { action: "LINK"; productId: string }
  | { action: "CREATE" | "APPROVE_PRICE" | "REJECT_PRICE" | "IGNORE" };

export const useResolve = (id: string) =>
  useAdminMutation((body: ResolveBody) => adminCall("POST", `/sync-issues/${id}/resolve`, body), [["sync-issues"], ["dashboard"]]);
