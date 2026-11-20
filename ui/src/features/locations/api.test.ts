import { afterEach, describe, expect, it, vi } from "vitest";
import { createLocation, deleteLocation, requestLocationTest, updateLocation } from "./api";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(body === null ? null : JSON.stringify(body), { status }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("locations api", () => {
  it("sends the input as JSON when creating", async () => {
    const fetchMock = stubFetch(201, { id: "l-1" });
    await createLocation({ name: "Share", type: "nfs", nfs: { server: "nfs", path: "/x" } });

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/locations");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({ name: "Share", type: "nfs", nfs: { server: "nfs", path: "/x" } });
  });

  it("puts a change to the location", async () => {
    const fetchMock = stubFetch(200, { id: "l 1" });
    await updateLocation("l 1", { name: "Share", type: "nfs", nfs: { server: "nfs", path: "/x" } });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/locations/l%201");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "PUT" });
  });

  it("asks for a test with a POST and no body", async () => {
    const fetchMock = stubFetch(202, { id: "l-1" });
    await requestLocationTest("l-1");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/locations/l-1/test");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "POST" });
    expect(fetchMock.mock.calls[0][1].body).toBeUndefined();
  });

  it("deletes and accepts the empty answer", async () => {
    const fetchMock = stubFetch(204, null);
    await expect(deleteLocation("l-1")).resolves.toBeUndefined();
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: "DELETE" });
  });

  it("surfaces the error code of a refused input", async () => {
    stubFetch(409, { error: { code: "location_name_taken", message: "Taken.", hint: "Choose another." } });
    await expect(createLocation({ name: "x", type: "nfs" })).rejects.toMatchObject({ code: "location_name_taken", status: 409 });
  });
});
