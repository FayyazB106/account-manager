; Inno Setup script for Account Manager.
; Produces a per-user installer (no admin needed) that installs to a writable
; location, creates a Start-Menu shortcut that launches the app + opens the
; browser, offers optional desktop and "start at sign-in" shortcuts, shows the
; README, and registers an uninstaller.
;
; Built in CI:  ISCC /DMyAppVersion=X.Y.Z /DStageDir=stage installer.iss /O"output"
; The stage dir must contain: account-manager.exe, launch.vbs, README.txt

#define MyAppName "Account Manager"
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef StageDir
  #define StageDir "stage"
#endif

[Setup]
AppId={{9C7E1E2A-4B3D-4F7A-9E21-8B1F3C6D2A10}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=Fayyaz
DefaultDirName={localappdata}\Programs\Account Manager
DefaultGroupName=Account Manager
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputBaseFilename=account-manager-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Files]
Source: "{#StageDir}\account-manager.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StageDir}\launch.vbs";           DestDir: "{app}"; Flags: ignoreversion
Source: "{#StageDir}\README.txt";           DestDir: "{app}"; Flags: ignoreversion isreadme

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Shortcuts:"
Name: "startupicon"; Description: "Start Account Manager automatically when I sign in"; GroupDescription: "Startup:"; Flags: unchecked

[Icons]
Name: "{group}\Account Manager"; Filename: "{app}\launch.vbs"; WorkingDir: "{app}"; IconFilename: "{app}\account-manager.exe"
Name: "{group}\Uninstall Account Manager"; Filename: "{uninstallexe}"
Name: "{userdesktop}\Account Manager"; Filename: "{app}\launch.vbs"; WorkingDir: "{app}"; IconFilename: "{app}\account-manager.exe"; Tasks: desktopicon
Name: "{userstartup}\Account Manager"; Filename: "{app}\launch.vbs"; WorkingDir: "{app}"; IconFilename: "{app}\account-manager.exe"; Tasks: startupicon

; Relaunch after install. No "skipifsilent": a silent in-app update must still
; reopen the app (the updater runs this installer with /SILENT). On an interactive
; install this is the "Launch Account Manager now" checkbox on the final page.
[Run]
Filename: "{app}\launch.vbs"; Description: "Launch Account Manager now"; Flags: shellexec postinstall nowait

; Stop a running server before uninstalling, otherwise it keeps running (and the
; in-use exe can't be removed). taskkill exits non-zero if it isn't running,
; which Inno ignores here.
[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/IM account-manager.exe /F /T"; Flags: runhidden; RunOnceId: "StopAccountManager"

[Code]
// Also stop a running server before (re)installing, so an upgrade over a running
// copy doesn't fail with the exe locked/in-use.
//
// NOTE: no "/T" here. When the app updates itself it launches THIS installer as a
// child process, and "/T" (kill the whole process tree of account-manager.exe)
// would terminate the installer too — aborting the update. "/F" alone stops just
// the app, freeing the exe so it can be replaced.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM account-manager.exe /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := '';
end;
