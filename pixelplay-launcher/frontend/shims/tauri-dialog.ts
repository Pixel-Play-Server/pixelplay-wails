import { invoke } from "./tauri-core";

export type DialogFilter = { name: string; extensions: string[] };

type OpenOptions = {
  title?: string;
  multiple?: boolean;
  directory?: boolean;
  filters?: DialogFilter[];
};

type SaveOptions = {
  title?: string;
  defaultPath?: string;
  filters?: DialogFilter[];
};

function patternFromFilters(filters?: DialogFilter[]): string {
  if (!filters?.length) {
    return "*.ppack;*.pixelplaypack;*.mrpack;*.zip";
  }
  const ext = filters.flatMap((f) => f.extensions || []).map((e) => {
    const t = String(e || "").replace(/^\*\./, "").replace(/^\./, "");
    return t ? `*.${t}` : "";
  }).filter(Boolean);
  return ext.join(";") || "*.ppack;*.pixelplaypack;*.mrpack;*.zip";
}

export async function open(options?: OpenOptions): Promise<string | string[] | null> {
  const title = options?.title || "Seleccionar archivo";
  if (options?.directory) {
    const p = await invoke<string>("pick_directory", { title });
    return p ? p : null;
  }
  const filter = patternFromFilters(options?.filters);
  if (options?.multiple) {
    const p = await invoke<string[]>("pick_files", { title, filter });
    return p?.length ? p : null;
  }
  const p = await invoke<string>("pick_file", { title, filter });
  return p ? p : null;
}

export async function save(options?: SaveOptions): Promise<string | null> {
  const title = options?.title || "Guardar archivo";
  const filter = patternFromFilters(options?.filters);
  const p = await invoke<string>("save_file", {
    title,
    defaultPath: options?.defaultPath || "",
    filter,
  });
  return p ? p : null;
}

export async function message(opts?: { message?: string; title?: string }): Promise<void> {
  window.alert(String(opts?.message ?? opts?.title ?? ""));
}

export async function ask(opts?: { message?: string }): Promise<boolean> {
  return window.confirm(String(opts?.message ?? ""));
}

export async function confirm(opts?: { message?: string }): Promise<boolean> {
  return window.confirm(String(opts?.message ?? ""));
}
