!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
!include "nsDialogs.nsh"
!include "x64.nsh"

!ifndef VERSION
!define VERSION "dev"
!endif

!ifndef SOURCE_DIR
!define SOURCE_DIR "dist/ntfy_windows_installer"
!endif

!ifndef OUT_FILE
!define OUT_FILE "dist/ntfy-${VERSION}-windows-amd64-setup.exe"
!endif

Name "ntfy"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\ntfy"
InstallDirRegKey HKCU "Software\ntfy" "InstallDir"
RequestExecutionLevel user

!define MUI_ICON "${SOURCE_DIR}\favicon.ico"
!define MUI_UNICON "${SOURCE_DIR}\favicon.ico"
!define MUI_ABORTWARNING

Var StartOnLogin

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
Page custom StartupPageCreate StartupPageLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function StartupPageCreate
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateCheckbox} 0 0 100% 12u "Start ntfy tray when I log in"
  Pop $StartOnLogin
  nsDialogs::Show
FunctionEnd

Function StartupPageLeave
  ${NSD_GetState} $StartOnLogin $0
  StrCpy $StartOnLogin $0
FunctionEnd

Section "ntfy" SecMain
  SetOutPath "$INSTDIR"
  File "${SOURCE_DIR}\ntfy.exe"
  File "${SOURCE_DIR}\ntfy-tray.exe"
  File "${SOURCE_DIR}\favicon.ico"
  File "${SOURCE_DIR}\LICENSE"
  File "${SOURCE_DIR}\README.md"
  File "${SOURCE_DIR}\set-shortcut-appid.ps1"

  SetOutPath "$INSTDIR\client"
  File "${SOURCE_DIR}\client.yml"

  CreateDirectory "$SMPROGRAMS\ntfy"
  CreateShortcut "$SMPROGRAMS\ntfy\ntfy tray.lnk" "$INSTDIR\ntfy-tray.exe" "" "$INSTDIR\favicon.ico"
  CreateShortcut "$SMPROGRAMS\ntfy\ntfy CLI.lnk" "$INSTDIR\ntfy.exe" "" "$INSTDIR\favicon.ico"
  CreateShortcut "$SMPROGRAMS\ntfy\Uninstall ntfy.lnk" "$INSTDIR\uninstall.exe"
  nsExec::ExecToLog 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\set-shortcut-appid.ps1" -ShortcutPath "$SMPROGRAMS\ntfy\ntfy tray.lnk" -AppUserModelID "ntfy"'
  Delete "$INSTDIR\set-shortcut-appid.ps1"

  WriteRegStr HKCU "Software\ntfy" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "DisplayName" "ntfy"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "Publisher" "ntfy"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "DisplayIcon" "$INSTDIR\favicon.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy" "NoRepair" 1
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ${If} $StartOnLogin == ${BST_CHECKED}
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "ntfy-tray" '"$INSTDIR\ntfy-tray.exe"'
  ${EndIf}

  IfFileExists "$APPDATA\ntfy\client.yml" doneUserConfig
    CreateDirectory "$APPDATA\ntfy"
    CopyFiles /SILENT "$INSTDIR\client\client.yml" "$APPDATA\ntfy\client.yml"
  doneUserConfig:
SectionEnd

Section "Uninstall"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "ntfy-tray"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ntfy"
  DeleteRegKey HKCU "Software\ntfy"

  Delete "$SMPROGRAMS\ntfy\ntfy tray.lnk"
  Delete "$SMPROGRAMS\ntfy\ntfy CLI.lnk"
  Delete "$SMPROGRAMS\ntfy\Uninstall ntfy.lnk"
  RMDir "$SMPROGRAMS\ntfy"

  Delete "$INSTDIR\client\client.yml"
  RMDir "$INSTDIR\client"
  Delete "$INSTDIR\ntfy.exe"
  Delete "$INSTDIR\ntfy-tray.exe"
  Delete "$INSTDIR\favicon.ico"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\set-shortcut-appid.ps1"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
