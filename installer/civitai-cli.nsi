; civitai-cli - NSIS installer for Windows
;
; Usage:
;   makensis /DAPP_VERSION=0.1.0 [/DAPP_ARCH=arm64] installer/civitai-cli.nsi
;
; Output:
;   dist/civitai-cli-windows-<arch>-setup.exe
;
; Design:
;   - Per-user install to %LOCALAPPDATA%\Programs\civitai-cli (no UAC,
;     no admin), matching install.ps1's default directory.
;   - Environment config (PATH) is delegated to civ.exe itself, which silently
;     registers its own directory into the user PATH on every startup
;     (env.EnsureUserPath, idempotent). The installer just drops the exe and
;     runs it once.
;   - The installer stub itself is x86; on ARM64 Windows it runs under the
;     built-in x86 emulation, while the exe it drops is the native
;     ${APP_ARCH} build.
;
; Path note: NSIS resolves File paths relative to this script's directory
; (installer/), so the built exe is referenced as "..\dist\<arch>\civ.exe".

Unicode true
ManifestDPIAware true

; Version injected from the command line: /DAPP_VERSION=x.y.z
!ifndef APP_VERSION
  !define APP_VERSION "0.0.0"
!endif

; Target CPU arch injected from the command line: /DAPP_ARCH=amd64|arm64
!ifndef APP_ARCH
  !define APP_ARCH "amd64"
!endif

Name "civitai-cli ${APP_VERSION}"
OutFile "..\dist\civitai-cli-windows-${APP_ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\civitai-cli"
; Reuse a previous install directory (own upgrade, or an install.ps1 install,
; which writes the same registry value)
InstallDirRegKey HKCU "Software\civitai-cli" "InstallDir"
RequestExecutionLevel user
ShowInstDetails show
ShowUnInstDetails show
BrandingText "civitai-cli ${APP_VERSION}"

; ---------- MUI2 (modern UI) ----------
!include "MUI2.nsh"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_UNFINISHPAGE_NOAUTOCLOSE

; Installer pages
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; Languages (English first = default)
!insertmacro MUI_LANGUAGE "English"

; ---------- Section: core files (required) ----------
Section "civitai-cli executable (required)" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "..\dist\${APP_ARCH}\civ.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Remember install location (reused on upgrade)
  WriteRegStr HKCU "Software\civitai-cli" "InstallDir" "$INSTDIR"

  ; Add/Remove Programs entry (per-user, HKCU)
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "DisplayName" "civitai-cli - Civitai CLI (search, inspect, vet models)"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "Publisher" "BaixuanZhu"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "URLInfoAbout" "https://github.com/BaixuanZhu/civitai-cli"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "DisplayIcon" '"$INSTDIR\civ.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli" "NoRepair" 1
SectionEnd

; ---------- Section: register the user PATH (recommended) ----------
Section "Register the user PATH (recommended)" SecConfig
  ; Run the exe once to trigger its built-in bootstrap: it appends $INSTDIR
  ; to the user PATH (registry HKCU\Environment) silently and idempotently,
  ; so users can run the bare command name "civ".
  ; nsExec avoids a flashing console window.
  nsExec::ExecToLog '"$INSTDIR\civ.exe" version'
SectionEnd

; ---------- Section: Start Menu shortcut to uninstaller ----------
Section "Start Menu uninstall shortcut" SecShortcut
  CreateDirectory "$SMPROGRAMS\civitai-cli"
  CreateShortcut "$SMPROGRAMS\civitai-cli\Uninstall civitai-cli.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

; ---------- Section descriptions (hover tooltips) ----------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore}     "civ.exe and its uninstaller (required)"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecConfig}   "Add the install directory to the user PATH so you can run the bare command name civ (recommended)"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecShortcut} "Create an uninstall shortcut under Start Menu > civitai-cli"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ---------- Uninstaller ----------
Section "Uninstall"
  ; Files (".bak" is left behind by a self-upgrade that could not delete the
  ; old exe while its process was still running)
  Delete "$INSTDIR\civ.exe"
  Delete "$INSTDIR\civ.exe.bak"
  Delete "$INSTDIR\uninstall.exe"

  ; Start Menu
  Delete "$SMPROGRAMS\civitai-cli\Uninstall civitai-cli.lnk"
  RMDir "$SMPROGRAMS\civitai-cli"

  ; Registry entries
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\civitai-cli"
  DeleteRegKey HKCU "Software\civitai-cli"

  ; Per-user cache (%LOCALAPPDATA%\civ: cached API bodies + hash cache; safe to
  ; delete) and config (%APPDATA%\civ: config.json holds the API token, so
  ; uninstalling also clears the sensitive credential). RMDir /r removes each
  ; tree wholesale; nothing else lives under those parents.
  RMDir /r "$LOCALAPPDATA\civ"
  RMDir /r "$APPDATA\civ"

  DetailPrint "Uninstall complete."

  ; Remove the (now empty) install directory
  RMDir "$INSTDIR"
SectionEnd