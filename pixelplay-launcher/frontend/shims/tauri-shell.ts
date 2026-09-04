export async function open(_opts?: unknown): Promise<void> {
  console.warn("[wails-shim] shell.open stub");
}

export async function Command() {
  throw new Error("[wails-shim] shell.Command not implemented");
}
