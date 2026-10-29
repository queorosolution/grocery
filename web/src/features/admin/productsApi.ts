import { useAdminQuery } from "./api";
import type { MasterProduct } from "./types";

/** Master products whose name, brand or keywords start with a word. Off until there is something to search for. */
export const useProductSearch = (q: string) =>
  useAdminQuery<{ data: MasterProduct[]; nextCursor: string }>(["products", "search"], "/products", { query: { q }, enabled: q.trim().length >= 2 });
