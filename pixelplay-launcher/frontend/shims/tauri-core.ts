/**
 * Tauri API shim for Wails — keeps the React UI calling invoke() unchanged.
 */
declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          Invoke: (cmd: string, argsJSON: string) => Promise<unknown>;
        };
      };
    };
  }
}

function getInvokeBridge() {
  const fn = window.go?.main?.App?.Invoke;
  if (!fn) {
    throw new Error(
      "Wails bridge no disponible (window.go.main.App.Invoke). ¿Corrés con wails dev/build?",
    );
  }
  return fn;
}

export async function invoke<T = unknown>(
  cmd: string,
  args?: Record<string, unknown>,
): Promise<T> {
  const bridge = getInvokeBridge();
  const result = await bridge(cmd, JSON.stringify(args ?? {}));
  return result as T;
}

export async function transformCallback() {
  throw new Error("transformCallback no soportado en Wails shim");
}

export function convertFileSrc(filePath: string, _protocol?: string): string {
  // Wails serves local assets differently; absolute file URLs work for many cases.
  if (filePath.startsWith("http") || filePath.startsWith("data:")) {
    return filePath;
  }
  if (filePath.startsWith("file://")) {
    return filePath;
  }
  return `file://${filePath}`;
}

export default { invoke, convertFileSrc };
