import { describe, expect, it } from "vitest";
import type { ActivityEntry } from "../activity/api";
import type { Location } from "../locations/api";
import type { Policy } from "../policies/api";
import { computeProgress, type SetupInput } from "./progress";

function location(state: Location["test"]["state"], id = "l-1"): Location {
  return { id, name: id, type: "nfs", test: { state, checks: [] }, exports: 0, createdAt: "2026-11-20T10:00:00Z" };
}

const policy = { id: "p-1", name: "Nightly", applications: ["demo"], exportTo: ["l-1"] } as unknown as Policy;

function backup(change: Partial<ActivityEntry> = {}): ActivityEntry {
  return {
    type: "backup", id: "s-1", application: "demo", phase: "Succeeded", createdAt: "2026-11-20T10:00:00Z", steps: [],
    policy: { id: "p-1", name: "Nightly" }, exports: [{ location: "l-1", phase: "Succeeded" }], ...change,
  };
}

function restore(change: Partial<ActivityEntry> = {}): ActivityEntry {
  return {
    type: "restore", id: "r-1", application: "demo", phase: "Succeeded", createdAt: "2026-11-20T10:00:00Z", steps: [], exports: [],
    restore: { snapshot: "", exportLocation: "Storage", created: 1, updated: 2, warnings: [] }, ...change,
  };
}

const empty: SetupInput = { locations: [], policies: [], backups: [], restores: [], restoreSkipped: false };

describe("computeProgress", () => {
  it("starts at the storage location on a fresh installation", () => {
    const p = computeProgress(empty);
    expect(p.current).toBe("location");
    expect(p.index).toBe(0);
    expect(Object.values(p.done)).toEqual([false, false, false, false]);
  });

  it("does not count a location whose test has not succeeded", () => {
    expect(computeProgress({ ...empty, locations: [location("failed"), location("pending", "l-2")] }).current).toBe("location");
  });

  it("moves on to the policy once a location works, and remembers which one", () => {
    const p = computeProgress({ ...empty, locations: [location("failed", "l-2"), location("succeeded")] });
    expect(p.current).toBe("policy");
    expect(p.location?.id).toBe("l-1");
  });

  it("asks for the test run after the policy", () => {
    const p = computeProgress({ ...empty, locations: [location("succeeded")], policies: [policy] });
    expect(p.current).toBe("run");
    expect(p.policy?.id).toBe("p-1");
  });

  it("counts only a backup of a policy whose copy worked as the test run", () => {
    const base = { ...empty, locations: [location("succeeded")], policies: [policy] };
    expect(computeProgress({ ...base, backups: [backup({ policy: undefined })] }).current).toBe("run");
    expect(computeProgress({ ...base, backups: [backup({ exports: [] })] }).current).toBe("run");
    expect(computeProgress({ ...base, backups: [backup({ exports: [{ location: "l-1", phase: "Failed" }] })] }).current).toBe("run");
    expect(computeProgress({ ...base, backups: [backup({ phase: "Failed" })] }).current).toBe("run");
    expect(computeProgress({ ...base, backups: [backup()] }).current).toBe("restore");
  });

  it("counts a restore from a storage location as the test restore, not a local one", () => {
    const base = { ...empty, locations: [location("succeeded")], policies: [policy], backups: [backup()] };
    expect(computeProgress({ ...base, restores: [restore({ restore: { snapshot: "s-1", created: 0, updated: 0, warnings: [] } })] }).current).toBe("restore");
    expect(computeProgress({ ...base, restores: [restore({ phase: "Failed" })] }).current).toBe("restore");
    expect(computeProgress({ ...base, restores: [restore()] })).toMatchObject({ current: "done", index: 4 });
  });

  it("lets the person skip the test restore", () => {
    const p = computeProgress({ ...empty, locations: [location("succeeded")], policies: [policy], backups: [backup()], restoreSkipped: true });
    expect(p.current).toBe("done");
  });

  it("goes back when something that was done is gone", () => {
    const all = { ...empty, locations: [location("succeeded")], policies: [policy], backups: [backup()], restores: [restore()] };
    expect(computeProgress(all).current).toBe("done");
    expect(computeProgress({ ...all, locations: [] }).current).toBe("location");
  });
});
