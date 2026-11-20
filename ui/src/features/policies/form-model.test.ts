import { describe, expect, it } from "vitest";
import type { Policy } from "./api";
import { emptyForm, findProblems, formFromPolicy, parseLabels, toInput, type FormProblem, type PolicyForm } from "./form-model";

function form(change: Partial<PolicyForm>): PolicyForm {
  return { ...emptyForm("Europe/Berlin"), name: "Nightly", applications: ["demo"], ...change };
}

describe("parseLabels", () => {
  it("reads one or several pairs", () => {
    expect(parseLabels("team=a")).toEqual({ team: "a" });
    expect(parseLabels(" team = a , tier=prod ")).toEqual({ team: "a", tier: "prod" });
    expect(parseLabels("example.com/team=")).toEqual({ "example.com/team": "" });
  });

  it.each(["", "  ", "team", "=a", "team=a, broken"])("rejects %j", (text) => {
    expect(parseLabels(text)).toBeUndefined();
  });
});

describe("findProblems", () => {
  it("accepts a complete form", () => {
    expect(findProblems(form({}))).toEqual([]);
  });

  const cases: { change: Partial<PolicyForm>; problem: FormProblem }[] = [
    { change: { name: "  " }, problem: "name" },
    { change: { applications: [] }, problem: "applications" },
    { change: { mode: "labels", labels: "oops" }, problem: "labels" },
    { change: { scheduleType: "hourly", minute: "75" }, problem: "minute" },
    { change: { scheduleType: "hourly", minute: "" }, problem: "minute" },
    { change: { scheduleType: "custom", cron: "* * *" }, problem: "cron" },
    { change: { keepAll: false }, problem: "retention" },
    { change: { keepAll: false, keep: { ...emptyForm("UTC").keep, daily: "-3" } }, problem: "retention" },
    { change: { keepAll: false, keep: { ...emptyForm("UTC").keep, daily: "5000" } }, problem: "retention" },
    { change: { keepAll: false, keep: { ...emptyForm("UTC").keep, daily: "abc" } }, problem: "retention" },
  ];
  it.each(cases)("finds $problem", ({ change, problem }) => {
    expect(findProblems(form(change))).toEqual([problem]);
  });

  it("accepts retention rules, where empty fields count as zero", () => {
    expect(findProblems(form({ keepAll: false, keep: { ...emptyForm("UTC").keep, last: "2", daily: " 7 " } }))).toEqual([]);
  });

  it("does not look at the retention numbers while everything is kept", () => {
    expect(findProblems(form({ keepAll: true, keep: { ...emptyForm("UTC").keep, daily: "abc" } }))).toEqual([]);
  });

  it("asks for a storage location only when told to", () => {
    expect(findProblems(form({ exportTo: [] }))).toEqual([]);
    expect(findProblems(form({ exportTo: [] }), { requireExport: true })).toEqual(["export"]);
    expect(findProblems(form({ exportTo: ["l-1"] }), { requireExport: true })).toEqual([]);
  });

  it("does not ask for applications when all are selected", () => {
    expect(findProblems(form({ mode: "all", applications: [] }))).toEqual([]);
  });
});

describe("toInput", () => {
  it("builds a daily schedule from the time", () => {
    expect(toInput(form({ scheduleType: "daily", time: "03:45" })).schedule).toEqual({
      type: "daily", minute: 45, hour: 3, weekday: 0, timezone: "Europe/Berlin",
    });
  });

  it("builds a weekly schedule", () => {
    expect(toInput(form({ scheduleType: "weekly", weekday: "5", time: "22:10" })).schedule).toMatchObject({
      type: "weekly", weekday: 5, hour: 22, minute: 10,
    });
  });

  it("takes the minute of an hourly schedule from its own field", () => {
    expect(toInput(form({ scheduleType: "hourly", minute: "20", time: "09:59" })).schedule).toMatchObject({
      type: "hourly", minute: 20, hour: 0,
    });
  });

  it("sends the cron expression only for custom", () => {
    expect(toInput(form({ scheduleType: "custom", cron: " * * * * * " })).schedule.cron).toBe("* * * * *");
    expect("cron" in toInput(form({ scheduleType: "hourly" })).schedule).toBe(false);
  });

  it("builds the selector for each mode", () => {
    expect(toInput(form({ mode: "applications", applications: ["a", "b"] })).selector).toEqual({ mode: "applications", applications: ["a", "b"] });
    expect(toInput(form({ mode: "labels", labels: "team=a" })).selector).toEqual({ mode: "labels", matchLabels: { team: "a" } });
    expect(toInput(form({ mode: "all" })).selector).toEqual({ mode: "all" });
  });

  it("sends zeros when everything is kept", () => {
    expect(toInput(form({ keepAll: true })).retention).toEqual({ last: 0, hourly: 0, daily: 0, weekly: 0, monthly: 0, yearly: 0 });
  });

  it("sends the typed retention numbers", () => {
    const keep = { ...emptyForm("UTC").keep, last: "2", weekly: "4", yearly: "1" };
    expect(toInput(form({ keepAll: false, keep })).retention).toEqual({ last: 2, hourly: 0, daily: 0, weekly: 4, monthly: 0, yearly: 1 });
  });

  it("sends exactly what the server accepts", () => {
    expect(Object.keys(toInput(form({}))).sort()).toEqual(["exportTo", "name", "retention", "schedule", "selector"]);
  });
});

describe("formFromPolicy", () => {
  const policy: Policy = {
    id: "p-1", name: "Weekly", cron: "30 4 * * 1", paused: true, applications: ["demo"], missing: [], createdAt: "2026-11-19T10:00:00Z",
    selector: { mode: "labels", matchLabels: { team: "a", tier: "prod" } },
    schedule: { type: "weekly", minute: 30, hour: 4, weekday: 1 },
    retention: { last: 0, hourly: 0, daily: 7, weekly: 0, monthly: 3, yearly: 0 },
    exportTo: ["l-1"],
  };

  it("fills the form and turns back into the same input", () => {
    const f = formFromPolicy(policy);
    expect(f).toMatchObject({ name: "Weekly", mode: "labels", time: "04:30", weekday: "1", timezone: "UTC" });
    expect(f.labels).toBe("team=a, tier=prod");
    const input = toInput(f);
    expect(input.selector).toEqual(policy.selector);
    expect(input.schedule).toMatchObject({ type: "weekly", minute: 30, hour: 4, weekday: 1 });
  });

  it("turns the retention of a policy into form fields and back", () => {
    const f = formFromPolicy(policy);
    expect(f.keepAll).toBe(false);
    expect(f.keep).toMatchObject({ daily: "7", monthly: "3", last: "" });
    expect(toInput(f).retention).toEqual(policy.retention);
  });

  it("carries the storage locations in both directions", () => {
    const f = formFromPolicy(policy);
    expect(f.exportTo).toEqual(["l-1"]);
    expect(toInput({ ...f, exportTo: ["l-1", "l-2"] }).exportTo).toEqual(["l-1", "l-2"]);
    expect(toInput(form({})).exportTo).toEqual([]);
  });

  it("shows a policy without rules as keeping everything", () => {
    const f = formFromPolicy({ ...policy, retention: { last: 0, hourly: 0, daily: 0, weekly: 0, monthly: 0, yearly: 0 } });
    expect(f.keepAll).toBe(true);
  });
});
