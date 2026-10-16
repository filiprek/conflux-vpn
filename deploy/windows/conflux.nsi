Unicode true
!include "LogicLib.nsh"
!include "FileFunc.nsh"
Name "Conflux"
OutFile "..\..\dist\Conflux-unsigned-setup.exe"
InstallDir "$PROGRAMFILES64\Conflux"
RequestExecutionLevel admin
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles
Section "Conflux"
  ${GetParameters} $R0
  ${GetOptions} $R0 "/OWNER_SID=" $R1
  ${GetOptions} $R0 "/SINGBOX=" $R2
  ${GetOptions} $R0 "/SINGBOX_SHA256=" $R3
  ${GetOptions} $R0 "/WINTUN=" $R4
  ${GetOptions} $R0 "/WINTUN_SHA256=" $R5
  ${If} $R1 == ""
  ${OrIf} $R2 == ""
  ${OrIf} $R3 == ""
  ${OrIf} $R4 == ""
  ${OrIf} $R5 == ""
    Abort "Provide /OWNER_SID= /SINGBOX= /SINGBOX_SHA256= /WINTUN= /WINTUN_SHA256=. See README."
  ${EndIf}
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\payload"
  File "..\..\dist\windows\conflux-service.exe"
  File "..\..\dist\windows\confluxctl.exe"
  File "..\..\dist\windows\Conflux.exe"
  File "install.ps1"
  File "uninstall.ps1"
  File "..\..\LICENSE"
  File "..\..\THIRD_PARTY_NOTICES.md"
  SetOutPath "$PLUGINSDIR\payload\licenses"
  File "..\..\docs\licenses\*"
  SetOutPath "$PLUGINSDIR\payload"
  nsExec::ExecToLog 'powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$PLUGINSDIR\payload\install.ps1" -OwnerSID "$R1" -SourceDirectory "$PLUGINSDIR\payload" -SingBox "$R2" -SingBoxSHA256 "$R3" -Wintun "$R4" -WintunSHA256 "$R5"'
  Pop $0
  ${If} $0 != 0
    Abort "Installation failed. Review the protected state before retrying."
  ${EndIf}
  SetOutPath "$PROGRAMFILES64\Conflux"
  WriteUninstaller "$PROGRAMFILES64\Conflux\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Conflux" "DisplayName" "Conflux"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Conflux" "UninstallString" '"$PROGRAMFILES64\Conflux\uninstall.exe"'
SectionEnd
Section "Uninstall"
  nsExec::ExecToLog 'powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$PROGRAMFILES64\Conflux\uninstall.ps1"'
  Pop $0
  ${If} $0 != 0
    Abort "Network recovery failed; files retained."
  ${EndIf}
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Conflux"
  Delete "$PROGRAMFILES64\Conflux\uninstall.ps1"
  Delete "$PROGRAMFILES64\Conflux\uninstall.exe"
  RMDir "$PROGRAMFILES64\Conflux"
SectionEnd
