import type { Location, LocationInput, LocationType } from "./api";

/** The form works with strings the way inputs do. toInput turns it into what the API takes. */
export interface LocationForm {
  name: string;
  type: LocationType;
  endpoint: string;
  region: string;
  bucket: string;
  prefix: string;
  pathStyle: boolean;
  skipTLSVerify: boolean;
  /** Never filled from the server. Empty on an existing location means "keep the stored keys". */
  accessKey: string;
  secretKey: string;
  nfsServer: string;
  nfsPath: string;
}

export type FormProblem = "name" | "endpoint" | "bucket" | "accessKey" | "secretKey" | "server" | "path";

export function emptyForm(): LocationForm {
  return {
    name: "",
    type: "s3",
    endpoint: "",
    region: "",
    bucket: "",
    prefix: "",
    pathStyle: true,
    skipTLSVerify: false,
    accessKey: "",
    secretKey: "",
    nfsServer: "",
    nfsPath: "",
  };
}

export function formFromLocation(l: Location): LocationForm {
  return {
    ...emptyForm(),
    name: l.name,
    type: l.type,
    endpoint: l.s3?.endpoint ?? "",
    region: l.s3?.region ?? "",
    bucket: l.s3?.bucket ?? "",
    prefix: l.s3?.prefix ?? "",
    pathStyle: l.s3?.pathStyle ?? true,
    skipTLSVerify: l.s3?.skipTLSVerify ?? false,
    nfsServer: l.nfs?.server ?? "",
    nfsPath: l.nfs?.path ?? "",
  };
}

const BUCKET = /^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/;

/** Problems that can be found without asking the server. `editing` means keys may stay empty. */
export function findProblems(form: LocationForm, editing: boolean): FormProblem[] {
  const problems: FormProblem[] = [];
  if (form.name.trim() === "") problems.push("name");

  if (form.type === "s3") {
    if (form.endpoint.trim() === "") problems.push("endpoint");
    if (!BUCKET.test(form.bucket.trim())) problems.push("bucket");
    const keys = [form.accessKey, form.secretKey];
    const given = keys.filter((k) => k !== "").length;
    // New locations need both keys. Existing ones keep theirs unless both new ones are given.
    if (given === 0 && !editing) {
      problems.push("accessKey", "secretKey");
    } else if (given === 1) {
      problems.push(form.accessKey === "" ? "accessKey" : "secretKey");
    }
  } else {
    if (form.nfsServer.trim() === "") problems.push("server");
    if (!form.nfsPath.trim().startsWith("/")) problems.push("path");
  }
  return problems;
}

/** Builds the API input. Call it only when findProblems is empty. */
export function toInput(form: LocationForm): LocationInput {
  if (form.type === "nfs") {
    return { name: form.name.trim(), type: "nfs", nfs: { server: form.nfsServer.trim(), path: form.nfsPath.trim() } };
  }
  return {
    name: form.name.trim(),
    type: "s3",
    s3: {
      endpoint: form.endpoint.trim(),
      region: form.region.trim(),
      bucket: form.bucket.trim(),
      prefix: form.prefix.trim(),
      pathStyle: form.pathStyle,
      skipTLSVerify: form.skipTLSVerify,
      ...(form.accessKey !== "" ? { accessKey: form.accessKey, secretKey: form.secretKey } : {}),
    },
  };
}
