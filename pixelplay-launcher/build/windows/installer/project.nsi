Unicode true

####
## Pixel Play Launcher — NSIS installer (Wails)
## Build: wails build -platform windows/amd64 -nsis -installscope user -s -skipbindings
####

## Per-user install comes from `wails build -installscope user` (defines REQUEST_EXECUTION_LEVEL).
!define INFO_COMPANYNAME "Pixel Play"
!define INFO_PRODUCTNAME "Pixel Play"
!define PRODUCT_EXECUTABLE "pixelplay-launcher.exe"
!define UNINST_KEY_NAME "PixelPlayLauncher"
# REQUEST_EXECUTION_LEVEL comes from `wails build -installscope user`

!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir Pixel Play"
!define MUI_FINISHPAGE_RUN_CHECKED
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchPixelPlay

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"

# Stable path used by soporte / scripts / updater (not Program Files).
InstallDir "$LOCALAPPDATA\Programs\PixelPlay"
InstallDirRegKey HKCU "Software\${UNINST_KEY_NAME}" "InstallLocation"
ShowInstDetails show

Function LaunchPixelPlay
    # Explicit launch so Finish always opens the drop-in exe path.
    Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    # Visual C++ 2015-2022 x64 — required by LWJGL (Minecraft). Missing → exit 3221225781.
    DetailPrint "Instalando Visual C++ Redistributable (x64)…"
    InitPluginsDir
    CreateDirectory "$PLUGINSDIR\vcredist"
    SetOutPath "$PLUGINSDIR\vcredist"
    File "tmp\vc_redist.x64.exe"
    ExecWait '"$PLUGINSDIR\vcredist\vc_redist.x64.exe" /install /passive /norestart' $0
    DetailPrint "Visual C++ setup exit: $0"

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # App data skeleton (same layout as runtime paths.DataDir on Windows)
    CreateDirectory "$APPDATA\pixelplay"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3\meta"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3\profiles"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3\ppacks"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3\updates"
    CreateDirectory "$APPDATA\pixelplay\pixelplayClientV3\logs"

    # Fresh-install defaults (avoids Failed to load profiles)
    IfFileExists "$APPDATA\pixelplay\pixelplayClientV3\profiles.json" skip_profiles
      FileOpen $0 "$APPDATA\pixelplay\pixelplayClientV3\profiles.json" w
      FileWrite $0 "[]$\r$\n"
      FileClose $0
    skip_profiles:
    IfFileExists "$APPDATA\pixelplay\pixelplayClientV3\accounts.json" skip_accounts
      FileOpen $0 "$APPDATA\pixelplay\pixelplayClientV3\accounts.json" w
      FileWrite $0 "{$\r$\n  $\"accounts$\": [],$\r$\n  $\"token$\": null$\r$\n}$\r$\n"
      FileClose $0
    skip_accounts:
    IfFileExists "$APPDATA\pixelplay\pixelplayClientV3\launcher_config.json" skip_config
      FileOpen $0 "$APPDATA\pixelplay\pixelplayClientV3\launcher_config.json" w
      FileWrite $0 "{}$\r$\n"
      FileClose $0
    skip_config:

    # Start Menu + Desktop
    CreateDirectory "$SMPROGRAMS\Pixel Play"
    CreateShortcut "$SMPROGRAMS\Pixel Play\Pixel Play.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$SMPROGRAMS\Pixel Play\Desinstalar Pixel Play.lnk" "$INSTDIR\uninstall.exe"
    CreateShortcut "$DESKTOP\Pixel Play.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    WriteRegStr HKCU "Software\${UNINST_KEY_NAME}" "InstallLocation" "$INSTDIR"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$LOCALAPPDATA\${PRODUCT_EXECUTABLE}"

    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\uninstall.exe"
    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\Pixel Play\Pixel Play.lnk"
    Delete "$SMPROGRAMS\Pixel Play\Desinstalar Pixel Play.lnk"
    RMDir "$SMPROGRAMS\Pixel Play"
    Delete "$DESKTOP\Pixel Play.lnk"

    DeleteRegKey HKCU "Software\${UNINST_KEY_NAME}"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
