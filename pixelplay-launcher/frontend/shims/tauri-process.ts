export async function exit(_code?: number): Promise<void> {
  window.runtime?.Quit?.();
}

export async function relaunch(): Promise<void> {
  window.location.reload();
}
