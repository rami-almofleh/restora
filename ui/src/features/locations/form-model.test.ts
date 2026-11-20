import { describe, expect, it } from "vitest";
import type { Location } from "./api";
import { emptyForm, findProblems, formFromLocation, toInput, type FormProblem, type LocationForm } from "./form-model";

function s3Form(change: Partial<LocationForm> = {}): LocationForm {
  return {
    ...emptyForm(),
    name: "MinIO",
    endpoint: "http://minio.minio:9000",
    bucket: "restora",
    accessKey: "AKIA",
    secretKey: "secret",
    ...change,
  };
}

function nfsForm(change: Partial<LocationForm> = {}): LocationForm {
  return { ...emptyForm(), name: "Share", type: "nfs", nfsServer: "nfs.example.com", nfsPath: "/exports/restora", ...change };
}

describe("findProblems", () => {
  it("accepts complete forms", () => {
    expect(findProblems(s3Form(), false)).toEqual([]);
    expect(findProblems(nfsForm(), false)).toEqual([]);
  });

  const cases: { form: LocationForm; editing: boolean; problems: FormProblem[] }[] = [
    { form: s3Form({ name: " " }), editing: false, problems: ["name"] },
    { form: s3Form({ endpoint: "" }), editing: false, problems: ["endpoint"] },
    { form: s3Form({ bucket: "AB" }), editing: false, problems: ["bucket"] },
    { form: s3Form({ bucket: "Has-Capitals" }), editing: false, problems: ["bucket"] },
    { form: s3Form({ accessKey: "", secretKey: "" }), editing: false, problems: ["accessKey", "secretKey"] },
    { form: s3Form({ secretKey: "" }), editing: false, problems: ["secretKey"] },
    { form: s3Form({ accessKey: "" }), editing: true, problems: ["accessKey"] },
    { form: nfsForm({ nfsServer: "" }), editing: false, problems: ["server"] },
    { form: nfsForm({ nfsPath: "exports" }), editing: false, problems: ["path"] },
  ];
  it.each(cases)("finds $problems", ({ form, editing, problems }) => {
    expect(findProblems(form, editing)).toEqual(problems);
  });

  it("lets keys stay empty while editing", () => {
    expect(findProblems(s3Form({ accessKey: "", secretKey: "" }), true)).toEqual([]);
  });
});

describe("toInput", () => {
  it("builds the S3 input with trimmed values and the keys", () => {
    const input = toInput(s3Form({ name: " MinIO ", prefix: " backups ", region: "eu-central-1" }));
    expect(input).toEqual({
      name: "MinIO",
      type: "s3",
      s3: {
        endpoint: "http://minio.minio:9000", region: "eu-central-1", bucket: "restora", prefix: "backups",
        pathStyle: true, skipTLSVerify: false, accessKey: "AKIA", secretKey: "secret",
      },
    });
  });

  it("sends no keys when they were left empty, so the stored ones stay", () => {
    const input = toInput(s3Form({ accessKey: "", secretKey: "" }));
    expect(input.s3 && "accessKey" in input.s3).toBe(false);
    expect(input.s3 && "secretKey" in input.s3).toBe(false);
  });

  it("builds the NFS input without S3 settings", () => {
    expect(toInput(nfsForm({ nfsServer: " nfs.example.com " }))).toEqual({
      name: "Share", type: "nfs", nfs: { server: "nfs.example.com", path: "/exports/restora" },
    });
  });
});

describe("formFromLocation", () => {
  const location: Location = {
    id: "l-1", name: "MinIO", type: "s3", createdAt: "2026-11-20T10:00:00Z",
    s3: { endpoint: "http://minio.minio:9000", bucket: "restora", pathStyle: false, skipTLSVerify: true, credentialsSet: true },
    test: { state: "succeeded", checks: [] }, exports: 0,
  };

  it("fills the settings but never the keys", () => {
    const form = formFromLocation(location);
    expect(form).toMatchObject({ name: "MinIO", type: "s3", bucket: "restora", pathStyle: false, skipTLSVerify: true });
    expect(form.accessKey).toBe("");
    expect(form.secretKey).toBe("");
  });

  it("fills an NFS location", () => {
    const form = formFromLocation({ ...location, type: "nfs", s3: undefined, nfs: { server: "nfs", path: "/x" } });
    expect(form).toMatchObject({ type: "nfs", nfsServer: "nfs", nfsPath: "/x" });
  });
});
