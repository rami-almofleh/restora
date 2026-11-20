import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchActivities } from "./api";

afterEach(() => vi.unstubAllGlobals());

const counts = { total: 3, active: 1, succeeded: 1, failed: 1 };

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body)));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("fetchActivities", () => {
  it("sends only the filters that are set", async () => {
    const fetchMock = stubFetch({ items: [], total: 0, counts });

    await fetchActivities({ query: "demo", type: "restore", status: "failed" }, 1);

    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(url.pathname).toBe("/api/v1/activities");
    expect(Object.fromEntries(url.searchParams)).toEqual({ q: "demo", type: "restore", status: "failed", limit: "20" });
  });

  it("leaves out filters that select everything", async () => {
    const fetchMock = stubFetch({ items: [], total: 0, counts });

    await fetchActivities({ query: "", type: "all", status: "all" }, 1);

    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(Object.fromEntries(url.searchParams)).toEqual({ limit: "20" });
  });

  it("returns the counts next to the entries", async () => {
    stubFetch({ items: [], total: 0, counts });

    const result = await fetchActivities({ query: "", type: "all", status: "all" }, 1);

    expect(result.meta.counts).toEqual(counts);
  });
});
