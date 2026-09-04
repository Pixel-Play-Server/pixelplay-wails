/** Event bus backed by Wails `window.runtime` (injected at runtime). */
type Handler = (event: { payload: unknown }) => void;
type Unlisten = () => void;

declare global {
  interface Window {
    runtime?: {
      EventsOn?: (event: string, callback: (...data: unknown[]) => void) => () => void;
      EventsEmit?: (event: string, ...data: unknown[]) => void;
    };
  }
}

export async function listen(event: string, handler: Handler): Promise<Unlisten> {
  const on = window.runtime?.EventsOn;
  if (typeof on === "function") {
    return on(event, (data: unknown) => {
      handler({ payload: data });
    });
  }
  return () => {};
}

export async function once(event: string, handler: Handler): Promise<Unlisten> {
  let done = false;
  const un = await listen(event, (e) => {
    if (done) return;
    done = true;
    un();
    handler(e);
  });
  return un;
}

export async function emit(event: string, payload?: unknown): Promise<void> {
  window.runtime?.EventsEmit?.(event, payload);
}

export async function emitTo(
  _target: string,
  event: string,
  payload?: unknown,
): Promise<void> {
  return emit(event, payload);
}
