# Pixel Play — backend Go/Wails

La versión de escritorio de Pixel Play 7.0 usa este backend junto al frontend React del [repositorio de interfaz](https://github.com/Pixel-Play-Server/pixelplaylauncher). Consulta las funciones, descargas y límites de compatibilidad en el [README principal](../README.md).

## Compilar desde un checkout nuevo

Requiere Go, Node.js, Wails v2 y sus dependencias de plataforma. En Linux se necesitan GTK 3 y WebKitGTK 4.1.

Desde este directorio, prepara el frontend (sin reemplazar enlaces o carpetas que ya tengas):

```sh
git clone https://github.com/Pixel-Play-Server/pixelplaylauncher.git ../pixelplay-ui
ln -s ../../pixelplay-ui/src frontend/src
ln -s ../../pixelplay-ui/public frontend/public
npm ci --prefix frontend
npm run build --prefix frontend
go test -tags webkit2_41 ./...
wails build -tags webkit2_41 -s -skipbindings -o pixelplay-launcher
```

Windows usa WebView2. Con el toolchain adecuado, el instalador NSIS se genera con:

```sh
wails build -platform windows/amd64 -s -skipbindings -webview2 embed -nsis -installscope user
```

`-s` omite la compilación del frontend: ejecuta primero `npm run build --prefix frontend`. Los binarios aparecen en `build/bin`.

Las releases públicas no utilizan el tag privado `lunar_private` ni incluyen ese runtime.
