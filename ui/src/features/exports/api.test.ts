import { afterEach, describe, expect, it, vi } from "vitest";
import { createRestoreFromExport } from "../restores/api";
import { fetchExportedApplications, fetchStoredBackups } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("exports api", () => {
  it("asks for the stored backups of one application", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [], total: 0 })));
    vi.stubGlobal("fetch", fetchMock);

    await fetchStoredBackups("my app", 1);

    const url = new URL(fetchMock.mock.calls[0][0] as string, "http://localhost");
    expect(url.pathname).toBe("/api/v1/exports");
    expect(url.searchParams.get("application")).toBe("my app");
    expect(url.searchParams.get("limit")).toBe("10");
  });

  it("lists the exported applications", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [{ application: "demo", exports: 3, latestAt: "2026-11-20T10:00:00Z" }] }))));
    expect(await fetchExportedApplications()).toEqual([{ application: "demo", exports: 3, latestAt: "2026-11-20T10:00:00Z" }]);
  });

  it("starts a restore from a stored backup", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "r-1" }), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    await createRestoreFromExport("x 1");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/exports/x%201/restores");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
  });
});
