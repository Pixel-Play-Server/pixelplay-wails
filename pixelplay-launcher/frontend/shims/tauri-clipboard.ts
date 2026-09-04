export async function writeText(text: string): Promise<void> {
  await navigator.clipboard.writeText(text);
}

export async function writeImage(_image: unknown): Promise<void> {
  console.warn("[wails-shim] clipboard.writeImage not implemented");
}

export async function readText(): Promise<string> {
  return navigator.clipboard.readText();
}
