/**
 * Minimal updater shim — routes through Go Invoke (Pixel host).
 */
export async function check() {
  const { invoke } = await import("./tauri-core");
  return invoke("check_for_updates", {});
}

export async function downloadAndInstall() {
  const { invoke } = await import("./tauri-core");
  return invoke("download_update", {});
}

export async function onUpdaterEvent(
  handler: (event: { status: string; message?: string; progress?: number }) => void,
) {
  const { listen } = await import("./tauri-event");
  return listen("updater_status", (e) => {
    handler((e.payload ?? {}) as { status: string; message?: string; progress?: number });
  });
}
