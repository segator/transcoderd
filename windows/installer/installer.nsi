Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef INPUT
  !error "INPUT is required"
!endif
!ifndef OUTPUT
  !error "OUTPUT is required"
!endif
Name "TranscoderD Worker"
OutFile "${OUTPUT}"
InstallDir "$PROGRAMFILES64\TranscoderD"
RequestExecutionLevel admin
SetCompressor /FINAL zlib
ShowInstDetails show
ShowUninstDetails show
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "TranscoderD necesita Windows de 64 bits."
    Abort
  ${EndIf}
FunctionEnd

Section "TranscoderD" Main
  SetShellVarContext current
  IfFileExists "$INSTDIR\transcoderd-desktop-service.exe" 0 fresh
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --prepare-upgrade'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "No se puede actualizar todavía. $1"
    Abort
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --uninstall'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "No se pudo retirar el servicio anterior: $1"
    Abort
  ${EndIf}
  IfFileExists "$INSTDIR\transcoderd-tray.exe" 0 fresh
  ExecWait '"$INSTDIR\transcoderd-tray.exe" --quit'
  Sleep 1000
fresh:
  SetOutPath "$INSTDIR"
  File /r "${INPUT}\*"
  ExecWait '"$INSTDIR\prerequisites\vc_redist.x64.exe" /install /quiet /norestart' $0
  ${If} $0 != 0
  ${AndIf} $0 != 3010
  ${AndIf} $0 != 1638
    MessageBox MB_ICONSTOP "No se pudo instalar Microsoft Visual C++ Runtime (código $0)."
    Abort
  ${EndIf}
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  SetRegView 32
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ExecWait '"$INSTDIR\prerequisites\MicrosoftEdgeWebview2Setup.exe" /silent /install' $0
    ${If} $0 != 0
    ${AndIf} $0 != 3010
      MessageBox MB_ICONSTOP "No se pudo instalar Microsoft Edge WebView2 Runtime (código $0)."
      Abort
    ${EndIf}
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --install'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "No se pudo instalar el servicio: $1"
    Abort
  ${EndIf}
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "DisplayName" "TranscoderD Worker"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "NoRepair" 1
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "TranscoderD" '"$INSTDIR\transcoderd-tray.exe" --background'
  CreateDirectory "$SMPROGRAMS\TranscoderD"
  CreateShortcut "$SMPROGRAMS\TranscoderD\TranscoderD.lnk" "$INSTDIR\transcoderd-tray.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --prepare-upgrade'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "No se puede desinstalar todavía. $1"
    Abort
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --uninstall'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "No se pudo retirar el servicio: $1"
    Abort
  ${EndIf}
  IfFileExists "$INSTDIR\transcoderd-tray.exe" 0 +3
  ExecWait '"$INSTDIR\transcoderd-tray.exe" --quit'
  Sleep 1000
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "TranscoderD"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD"
  Delete "$SMPROGRAMS\TranscoderD\TranscoderD.lnk"
  RMDir "$SMPROGRAMS\TranscoderD"
  RMDir /r "$INSTDIR"
SectionEnd
