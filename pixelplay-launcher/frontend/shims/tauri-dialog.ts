export async function open(_options?: unknown): Promise<string | string[] | null> {
  console.warn("[wails-shim] dialog.open not implemented yet");
  return null;
}

export async function save(_options?: unknown): Promise<string | null> {
  console.warn("[wails-shim] dialog.save not implemented yet");
  return null;
}

export async function message(_options?: unknown): Promise<void> {
  console.warn("[wails-shim] dialog.message not implemented yet");
}

export async function ask(_options?: unknown): Promise<boolean> {
  return window.confirm(String((_options as { message?: string })?.message ?? ""));
}

export async function confirm(_options?: unknown): Promise<boolean> {
  return window.confirm(String((_options as { message?: string })?.message ?? ""));
}

export type DialogFilter = { name: string; extensions: string[] };
