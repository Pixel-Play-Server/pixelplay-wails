export async function readFile(_path: string): Promise<Uint8Array> {
  throw new Error("[wails-shim] fs.readFile not implemented");
}

export async function exists(_path: string): Promise<boolean> {
  return false;
}

export async function mkdir() {}
export async function writeFile() {}
export async function remove() {}
