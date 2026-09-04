/**
 * Tauri window API shim → Wails runtime window controls.
 */
declare global {
  interface Window {
    runtime?: {
      WindowMinimise?: () => void;
      WindowMaximise?: () => void;
      WindowUnmaximise?: () => void;
      WindowToggleMaximise?: () => void;
      WindowIsMaximised?: () => Promise<boolean> | boolean;
      Quit?: () => void;
      EventsOn?: (event: string, callback: (...data: unknown[]) => void) => () => void;
      EventsEmit?: (event: string, ...data: unknown[]) => void;
      OnFileDrop?: (callback: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean) => void;
      OnFileDropOff?: () => void;
    };
  }
}

function win() {
  return window.runtime ?? {};
}

export function getCurrentWindow() {
  const api = {
    minimize: async () => {
      win().WindowMinimise?.();
    },
    maximize: async () => {
      win().WindowMaximise?.();
    },
    unmaximize: async () => {
      win().WindowUnmaximise?.();
    },
    toggleMaximize: async () => {
      win().WindowToggleMaximise?.();
    },
    close: async () => {
      win().Quit?.();
    },
    hide: async () => {},
    show: async () => {},
    setTitle: async (title: string) => {
      document.title = title;
    },
    isMaximized: async () => {
      const v = win().WindowIsMaximised?.();
      return !!(await Promise.resolve(v));
    },
    startDragging: async () => {},
    onFocusChanged: async () => () => {},
    onResized: async () => () => {},
    listen: async () => () => {},
    onDragDropEvent: async (cb: (event: { payload: unknown }) => void | Promise<void>) => {
      const rt = win();
      if (typeof rt.OnFileDrop !== "function") {
        return () => {};
      }
      rt.OnFileDrop((x, y, paths) => {
        void cb({
          payload: {
            type: "drop",
            paths: paths ?? [],
            position: { x, y },
          },
        });
      }, false);
      return () => {
        rt.OnFileDropOff?.();
      };
    },
  };
  return api;
}

export function getCurrentWebviewWindow() {
  return getCurrentWindow();
}

export class Window {
  constructor(_label?: string, _options?: unknown) {}
  static getCurrent() {
    return getCurrentWindow();
  }
  static getByLabel(_label: string) {
    return getCurrentWindow();
  }
  async show() {}
  async close() {
    win().Quit?.();
  }
}

export default { getCurrentWindow, Window };
