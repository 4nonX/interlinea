; The Windows installer of Interlinea, built with Inno Setup:
;
;   iscc /DAppVersion=0.6.0 /DSourceDir=<folder with Interlinea.exe and its files> /DOutputDir=<folder> interlinea.iss
;
; It installs for the person who runs it, without administrator rights.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "."
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif

[Setup]
AppId={{6A1B3F0C-4D0E-4C56-9E5E-6B0A5B7B7F31}
AppName=Interlinea
AppVersion={#AppVersion}
AppPublisher=Interlinea
DefaultDirName={autopf}\Interlinea
DefaultGroupName=Interlinea
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=Interlinea-{#AppVersion}-windows-x64-setup
SetupIconFile=interlinea.ico
UninstallDisplayIcon={app}\Interlinea.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "german"; MessagesFile: "compiler:Languages\German.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\Interlinea"; Filename: "{app}\Interlinea.exe"
Name: "{autodesktop}\Interlinea"; Filename: "{app}\Interlinea.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\Interlinea.exe"; Description: "{cm:LaunchProgram,Interlinea}"; Flags: nowait postinstall skipifsilent
