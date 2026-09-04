export async function openUrl(url: string): Promise<void> {
  if (url.startsWith("http://") || url.startsWith("https://")) {
    window.open(url, "_blank");
    return;
  }
  return openPath(url);
}

export async function openPath(path: string): Promise<void> {
  const { invoke } = await import("./tauri-core");
  await invoke("open_file_directory", { filePath: path });
}

export async function revealItemInDir(path: string): Promise<void> {
  return openPath(path);
}
