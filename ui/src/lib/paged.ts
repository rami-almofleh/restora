import { apiGet } from "./api";

/**
 * The loaded part of a list. `meta` holds what the server sends next to the items, for example counts;
 * it comes from the newest page that was loaded.
 */
export interface PagedList<T, M extends object = object> {
  items: T[];
  total: number;
  hasMore: boolean;
  meta: M;
}

type Page<T, M> = { items: T[]; total: number; nextCursor?: string } & M;

/**
 * Loads the first `pages` pages of a list endpoint, one request per page.
 * Reloading all pages at once keeps a list that grew with "load more" in step with the cluster.
 */
export async function fetchPaged<T, M extends object = object>(
  path: string,
  query: Record<string, string>,
  pageSize: number,
  pages: number,
  signal?: AbortSignal,
): Promise<PagedList<T, M>> {
  const items: T[] = [];
  let total = 0;
  let meta = {} as M;
  let cursor = "";

  for (let i = 0; i < pages; i++) {
    const params = new URLSearchParams({ ...query, limit: String(pageSize) });
    if (cursor) params.set("cursor", cursor);

    const { items: pageItems, total: pageTotal, nextCursor, ...rest } = await apiGet<Page<T, M>>(`${path}?${params}`, signal);
    items.push(...pageItems);
    total = pageTotal;
    meta = rest as unknown as M;
    if (!nextCursor) {
      return { items, total, hasMore: false, meta };
    }
    cursor = nextCursor;
  }
  return { items, total, hasMore: true, meta };
}
