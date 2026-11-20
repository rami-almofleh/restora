import { afterEach, describe, expect, it, vi } from "vitest";
import { createRestore, fetchRestores } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("createRestore", () => {
  it("posts to the snapshot and returns the new restore", async () => {
    const restore = { id: "r-1", phase: "Pending", steps: [], warnings: [] };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(restore), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    const created = await createRestore("s-1/odd");

    expect(created.id).toBe("r-1");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/snapshots/s-1%2Fodd/restores");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
  });

  it.each(["snapshot_not_found", "snapshot_not_ready", "restore_in_progress", "snapshot_in_progress"])(
    "surfaces the API error %s",
    async (code) => {
      const body = { error: { code, message: "No.", hint: "Wait." } };
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 409 })));

      await expect(createRestore("s-1")).rejects.toMatchObject({ code, hint: "Wait.", status: 409 });
    },
  );
});

describe("fetchRestores", () => {
  it("asks for the restores of the application", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [], total: 0 })));
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchRestores("my app", 1);

    expect(result).toEqual({ items: [], total: 0, hasMore: false, meta: {} });
    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(url.pathname).toBe("/api/v1/applications/my%20app/restores");
    expect(url.searchParams.get("limit")).toBe("5");
  });
});
