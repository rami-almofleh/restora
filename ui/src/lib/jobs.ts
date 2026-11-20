/** What every long-running action of Restora (backup, restore) reports. The values match status.phase in the cluster. */
export type JobPhase = "Pending" | "Running" | "Succeeded" | "Failed";

export interface JobStep {
  id: string;
  phase: JobPhase;
}

export function isActive(job: { phase: JobPhase }): boolean {
  return job.phase === "Pending" || job.phase === "Running";
}

/** How long a job took, or for a running job how long it has been going. */
export function jobDurationMs(job: { createdAt: string; finishedAt?: string }, now: number = Date.now()): number {
  const end = job.finishedAt ? Date.parse(job.finishedAt) : now;
  return Math.max(0, end - Date.parse(job.createdAt));
}
