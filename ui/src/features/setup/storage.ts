/** Choices of the person that the cluster cannot know. They live in this browser. */
export type SetupFlag = "seen" | "dismissed" | "restoreSkipped";

const PREFIX = "restora.setup.";

export function readFlag(flag: SetupFlag): boolean {
  try {
    return localStorage.getItem(PREFIX + flag) === "true";
  } catch {
    return false;
  }
}

export function writeFlag(flag: SetupFlag, value: boolean): void {
  try {
    if (value) {
      localStorage.setItem(PREFIX + flag, "true");
    } else {
      localStorage.removeItem(PREFIX + flag);
    }
  } catch {
    // Without storage the choice only lasts until the page is reloaded.
  }
}
