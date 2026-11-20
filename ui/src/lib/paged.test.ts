import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchPaged } from "./paged";

afterEach(() => vi.unstubAllGlobals());

function respond(...bodies: unknown[]) {
  const fetchMock = vi.fn();
  for (const body of bodies) {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(body)));
  }
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("fetchPaged", () => {
  it("stops when the server has no next page", async () => {
    const fetchMock = respond({ items: ["a"], total: 1 });

    const result = await fetchPaged<string>("/api/v1/things", { q: "x" }, 10, 3);

    expect(result).toEqual({ items: ["a"], total: 1, hasMore: false, meta: {} });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(url.pathname).toBe("/api/v1/things");
    expect(url.searchParams.get("q")).toBe("x");
    expect(url.searchParams.get("limit")).toBe("10");
    expect(url.searchParams.has("cursor")).toBe(false);
  });

  it("follows the cursor for each requested page and reports more", async () => {
    const fetchMock = respond(
      { items: ["a"], total: 3, nextCursor: "a" },
      { items: ["b"], total: 3, nextCursor: "b" },
    );

    const result = await fetchPaged<string>("/api/v1/things", {}, 1, 2);

    expect(result).toEqual({ items: ["a", "b"], total: 3, hasMore: true, meta: {} });
    const second = new URL(fetchMock.mock.calls[1][0] as string, "http://localhost");
    expect(second.searchParams.get("cursor")).toBe("a");
  });

  it("returns what the newest page sent besides the items", async () => {
    respond(
      { items: ["a"], total: 2, nextCursor: "a", counts: { active: 5 } },
      { items: ["b"], total: 2, counts: { active: 1 } },
    );

    const result = await fetchPaged<string, { counts: { active: number } }>("/api/v1/things", {}, 1, 2);

    expect(result.meta).toEqual({ counts: { active: 1 } });
  });
});
