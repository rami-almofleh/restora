import { afterEach, describe, expect, it, vi } from "vitest";
import { createSnapshot, deleteSnapshot, fetchSnapshots, type Snapshot } from "./api";

function snapshot(id: string, phase: Snapshot["phase"] = "Succeeded"): Snapshot {
  return { id, application: "demo", phase, createdAt: "2026-11-19T10:00:00Z", steps: [], volumes: [], exports: [] };
}

afterEach(() => vi.unstubAllGlobals());

describe("fetchSnapshots", () => {
  it("follows the cursor and keeps the order", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [snapshot("b")], total: 2, nextCursor: "b" })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [snapshot("a")], total: 2 })));
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchSnapshots("demo", 3);

    expect(result.items.map((s) => s.id)).toEqual(["b", "a"]);
    expect(result.hasMore).toBe(false);
    const second = new URL(fetchMock.mock.calls[1][0] as string, "http://localhost");
    expect(second.pathname).toBe("/api/v1/applications/demo/snapshots");
    expect(second.searchParams.get("cursor")).toBe("b");
  });
});

describe("createSnapshot", () => {
  it("posts to the application and returns the new snapshot", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(snapshot("s-1", "Pending")), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    const created = await createSnapshot("my app");

    expect(created.id).toBe("s-1");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/applications/my%20app/snapshots");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
  });

  it("surfaces the API error code and hint", async () => {
    const body = { error: { code: "snapshot_in_progress", message: "Already running.", hint: "Wait." } };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 409 })));

    await expect(createSnapshot("demo")).rejects.toMatchObject({ code: "snapshot_in_progress", hint: "Wait.", status: 409 });
  });
});

describe("deleteSnapshot", () => {
  it("sends a DELETE for the snapshot and accepts the empty answer", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteSnapshot("s 1")).resolves.toBeUndefined();

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/snapshots/s%201");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "DELETE" });
  });

  it("surfaces why a backup cannot be deleted", async () => {
    const body = { error: { code: "snapshot_in_use", message: "In use.", hint: "Wait." } };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 409 })));

    await expect(deleteSnapshot("s-1")).rejects.toMatchObject({ code: "snapshot_in_use", status: 409 });
  });
});
