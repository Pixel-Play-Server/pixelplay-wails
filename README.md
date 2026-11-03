# Pixel Play 7.0

Un launcher para tener tus perfiles, mods y amigos en un mismo lugar, con una interfaz de estilo Minecraft.

**[Descargar Pixel Play](https://launcher.pixelplay.gg)** · **[Código del frontend](https://github.com/Pixel-Play-Server/pixelplaylauncher)** · **[Backend Go/Wails](https://github.com/Pixel-Play-Server/pixelplay-wails)**

## Qué trae 7.0

- Navegación con secciones visibles, controles de cuenta compactos y la misma estética pixelada en todo el launcher.
- Perfiles y gestión de contenido desde Modrinth y CurseForge.
- Horizon 26.3 integrado en el flujo de perfiles de Pixel Play para Linux y Windows x86_64.
- Catálogo de [playhorizon.dev/mods](https://playhorizon.dev/mods): mods WebAssembly `.hmod`, consulta de proyectos nativos `.hnmod` y descarga verificada de resource packs.
- Cuentas más ordenadas y correcciones en mensajes, confirmaciones de envío y notificaciones de amigos.
- Capas oficiales de Minecraft de tu cuenta Microsoft; sin catálogo de capas personalizadas.
- Sin anuncios de hosting ni servidores patrocinados preconfigurados.

### Horizon: compatibilidad del contenido

Los mods `.hmod` se instalan y activan desde Pixel Play. Los resource packs se guardan en la carpeta del perfil y se seleccionan dentro del juego. Las descargas del catálogo verifican SHA-256 y tamaño y respetan las revocaciones del registro.

Los proyectos `.hnmod` contienen código Rust que requiere compilar una versión de Horizon con sus mixins. Se muestran con esta advertencia; **el compilador nativo todavía no está integrado**, por lo que Pixel Play no los presenta como instalados ni ejecutables. Un mod Java `.jar` no se convierte en un mod Horizon.

Horizon y Minecraft requieren sus propios recursos y condiciones de uso. Pixel Play no incluye archivos del juego ni concede acceso a servicios o servidores que requieran una cuenta autenticada.

## Desarrollo

La versión de escritorio 7.0 utiliza **Go + Wails + React/TypeScript**. El árbol `src-tauri` del repositorio frontend conserva la implementación anterior; no es el proceso de compilación de esta versión.

Consulta las instrucciones del [backend](https://github.com/Pixel-Play-Server/pixelplay-wails/tree/main/pixelplay-launcher). Las distribuciones de esta versión son Linux y Windows x86_64; no se anuncia un nuevo binario macOS sin una compilación verificada.

## Créditos y licencia

La interfaz deriva de [NoRiskClient Launcher](https://github.com/NoRiskClient/noriskclient-launcher) y [LiquidLauncher](https://github.com/liquidlauncher/liquidlauncher). Pixel Play mantiene su propia integración y diseño sobre esa base. El frontend se distribuye bajo **GNU GPL v3**; consulta su [LICENSE](https://github.com/Pixel-Play-Server/pixelplaylauncher/blob/main/LICENSE).

Horizon es un proyecto independiente de [horizon-rust](https://github.com/horizon-rust/horizon). Los mods, bibliotecas y demás proyectos conservan sus respectivas licencias. Las integraciones privadas de Lunar no forman parte de esta distribución pública.

No es un producto oficial de Minecraft, Mojang ni Microsoft.
