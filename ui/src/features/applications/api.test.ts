import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchApplications } from "./api";

function summary(name: string) {
  return { name, health: "running", engines: [], workloads: 1, volumes: 0, configMaps: 0, secrets: 0, createdAt: "" };
}

function respondWith(...bodies: unknown[]) {
  const fetchMock = vi.fn();
  for (const body of bodies) {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(body), { status: 200 }));
  }
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("fetchApplications", () => {
  it("sends the filter and stops when there is no next page", async () => {
    const fetchMock = respondWith({ items: [summary("demo")], total: 1 });

    const result = await fetchApplications({ query: "demo", volumes: "with" }, 3);

    expect(result).toEqual({ items: [summary("demo")], total: 1, hasMore: false, meta: {} });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(url.searchParams.get("q")).toBe("demo");
    expect(url.searchParams.get("volumes")).toBe("with");
  });

  it("follows the cursor for each requested page and reports more", async () => {
    const fetchMock = respondWith(
      { items: [summary("a")], total: 3, nextCursor: "a" },
      { items: [summary("b")], total: 3, nextCursor: "b" },
    );

    const result = await fetchApplications({ query: "", volumes: "all" }, 2);

    expect(result.items.map((i) => i.name)).toEqual(["a", "b"]);
    expect(result.hasMore).toBe(true);
    const second = new URL(fetchMock.mock.calls[1][0] as string, "http://localhost");
    expect(second.searchParams.get("cursor")).toBe("a");
    expect(second.searchParams.has("volumes")).toBe(false);
  });
});
