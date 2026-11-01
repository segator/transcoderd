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
!define MUI_FINISHPAGE_TEXT "Setup is complete. Open TranscoderD from the Start menu to connect your worker."
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  StrCpy $INSTDIR "$PROGRAMFILES64\TranscoderD"
  ${IfNot} ${RunningX64}
    Push "TranscoderD requires 64-bit Windows."
    Call FailInstall
  ${EndIf}
FunctionEnd

Function FailInstall
  Pop $2
  DetailPrint "$2"
  IfSilent silent_failure
  MessageBox MB_ICONSTOP "$2"
silent_failure:
  SetErrorLevel 1
  Abort
FunctionEnd

Section "TranscoderD" Main
  SetShellVarContext current
  StrCpy $R9 "0"
  StrCpy $R8 "0"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=service-helper.exe "${INPUT}/transcoderd-desktop-service.exe"
  nsExec::ExecToStack '"$PLUGINSDIR\service-helper.exe" --record-installation "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    Push "Could not preserve the installation owner: $1"
    Call FailInstall
  ${EndIf}
  IfFileExists "$INSTDIR\transcoderd-desktop-service.exe" 0 fresh
  StrCpy $R9 "1"
  DetailPrint "Waiting for all accepted CPU/GPU work to finish before upgrading..."
  nsExec::ExecToLog '"$PLUGINSDIR\service-helper.exe" --prepare-upgrade-wait'
  Pop $0
  ${If} $0 != 0
    Push "Could not prepare a safe upgrade. See the details above. The service and accepted work have not been force-stopped."
    Call FailInstall
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --uninstall'
  Pop $0
  Pop $1
  ${If} $0 != 0
    Push "Could not remove the previous service: $1"
    Call FailInstall
  ${EndIf}
  IfFileExists "$INSTDIR\transcoderd-tray.exe" 0 after_tray
  ExecWait '"$INSTDIR\transcoderd-tray.exe" --quit'
  Sleep 1000
after_tray:
  nsExec::ExecToStack '"$PLUGINSDIR\service-helper.exe" --close-trays'
  Pop $0
  Pop $1
  ${If} $0 != 0
    Push "Could not close the installed dashboard: $1"
    Call FailInstall
  ${EndIf}
  ClearErrors
  RMDir /r "$INSTDIR"
  ${If} ${Errors}
    Push "Could not replace the previous installation files."
    Call FailInstall
  ${EndIf}
fresh:
  SetOutPath "$INSTDIR"
  ClearErrors
  File /r "${INPUT}\*"
!ifdef FIXTURE
  ReadEnvStr $R7 "ProgramData"
  IfFileExists "$R7\TranscoderD\updates\fixture-fail-install" 0 fixture_install_ok
  Delete "$INSTDIR\transcoderd-worker.exe"
  SetErrorLevel 42
  Abort
fixture_install_ok:
!endif
  ${If} ${Errors}
    Push "Could not write the installation files."
    Call FailInstall
  ${EndIf}
  ExecWait '"$INSTDIR\prerequisites\vc_redist.x64.exe" /install /quiet /norestart' $0
  ${If} $0 == 3010
    StrCpy $R8 "1"
  ${EndIf}
  ${If} $0 != 0
  ${AndIf} $0 != 3010
  ${AndIf} $0 != 1638
    Push "Could not install Microsoft Visual C++ Runtime (code $0)."
    Call FailInstall
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
    ${If} $0 == 3010
      StrCpy $R8 "1"
    ${EndIf}
    ${If} $0 != 0
    ${AndIf} $0 != 3010
      Push "Could not install Microsoft Edge WebView2 Runtime (code $0)."
      Call FailInstall
    ${EndIf}
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --install'
  Pop $0
  Pop $1
  ${If} $0 != 0
    Push "Could not install the service: $1"
    Call FailInstall
  ${EndIf}
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "DisplayName" "TranscoderD Worker"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\TranscoderD" "NoRepair" 1
  ${If} $R9 == "0"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "TranscoderD" '"$INSTDIR\transcoderd-tray.exe" --background'
    CreateDirectory "$SMPROGRAMS\TranscoderD"
    CreateShortcut "$SMPROGRAMS\TranscoderD\TranscoderD.lnk" "$INSTDIR\transcoderd-tray.exe"
  ${EndIf}
  ${If} $R8 == "1"
    SetRebootFlag true
    SetErrorLevel 3010
  ${EndIf}
!ifdef FIXTURE
  ReadEnvStr $R7 "ProgramData"
  IfFileExists "$R7\TranscoderD\updates\fixture-restart-required" 0 fixture_no_restart
  SetErrorLevel 3010
fixture_no_restart:
!endif
SectionEnd

Section "Uninstall"
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --prepare-upgrade'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "Cannot uninstall yet. $1"
    Abort
  ${EndIf}
  nsExec::ExecToStack '"$INSTDIR\transcoderd-desktop-service.exe" --uninstall'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Could not remove the service: $1"
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
