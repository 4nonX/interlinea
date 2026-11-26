---
title: Install Interlinea
description: Download the desktop app for Windows, macOS or Linux, get past the first-start warning, and check the download.
---

Every release has the same files. The [download section](/#download) on
the front page always links to the newest one; all releases are on
[GitHub](https://github.com/4nonX/interlinea/releases).

## Windows

Download `Interlinea-<version>-windows-x64-setup.exe` and run it. It
installs for your user account only and needs no administrator rights.

The installer is not signed with a code-signing certificate yet, so
Windows may stop it with **Windows protected your PC**. Click
**More info**: the publisher shows as unknown, because the file is not
signed. Click **Run anyway**.

If you can't install programs on your computer, take
`Interlinea-<version>-windows-x64-portable.zip` instead: unpack it
anywhere and start `Interlinea.exe`.

## macOS

Download `Interlinea-<version>-macos-universal.dmg`. It runs on Macs with
Apple silicon and with Intel processors. Open it and drag Interlinea to
Applications.

The app is not notarized by Apple yet, so the first start is refused with
a message that Apple could not check it. To open it once:

1. Open **System Settings**, then **Privacy & Security**.
2. Scroll to **Security**. It names Interlinea with an **Open Anyway**
   button. Click it and confirm with your password.

On macOS 14 and earlier, a right click on the app in Applications and
**Open** does the same. After the first start it opens like any app.

## Linux

Download `Interlinea-<version>-linux-x86_64.AppImage`, make it executable
and start it:

```sh
chmod +x Interlinea-*-linux-x86_64.AppImage
./Interlinea-*-linux-x86_64.AppImage
```

If it does not start and the message mentions FUSE, install your
distribution's FUSE 2 package (`libfuse2` on Debian and Ubuntu).

## Check the download

`SHA256SUMS` in the release lists the checksum of every file. Compare it
with the checksum of the file you downloaded:

```sh
# Windows (PowerShell)
Get-FileHash .\Interlinea-0.4.0-windows-x64-setup.exe -Algorithm SHA256

# macOS
shasum -a 256 Interlinea-0.4.0-macos-universal.dmg

# Linux, with SHA256SUMS in the same folder
sha256sum --check --ignore-missing SHA256SUMS
```

## Without installing anything

The browser client runs in any current browser at
[interlinea.dev/app](/app/). Your work is kept in that browser on your
computer, not on a server. [Try it with a sample project](/app/?sample).

## Updating

Install the new version over the old one. Your projects, memories and
termbases are in a separate folder and stay where they are
([where your work is kept](/docs/help/your-data/)).
