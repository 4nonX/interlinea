# Signing the releases

The release workflow (`.github/workflows/release.yml`) signs the Windows
programs and notarizes the macOS app as soon as the certificates are
there, and builds them unsigned until then, as it does today. Nothing
else changes: the same jobs build, start and publish the files either
way, and the release notes tell people about the first-start warning
only for the system whose files are not signed.

| System | Without the setup below | With it |
|---|---|---|
| Windows | `Interlinea.exe` and the installer unsigned: SmartScreen may stop the first start | `Interlinea.exe` and the installer signed through SignPath |
| macOS | the app signed for the computer only: macOS refuses the first start until Open Anyway | the app signed with a Developer ID, the disk image notarized and stapled |
| Linux | an AppImage, which has no signature to check | the same |

## Windows, through SignPath

Code signing certificates for Windows are kept on hardware now, so a
certificate file in a secret is no longer how a project signs. The
workflow sends the files to [SignPath](https://about.signpath.io), which
signs them with a certificate it keeps, after checking that they were
built by this repository's workflow on GitHub's own machines.

SignPath Foundation gives open-source projects such a certificate free of
charge; it is issued to SignPath Foundation, which is then named as the
publisher. Its [conditions](https://signpath.org/terms) are what the
project has to meet, among them:

- an OSI-approved licence for everything (AGPL-3.0 is one);
- multi-factor authentication on GitHub and SignPath for everyone in the
  team, and named authors, reviewers and approvers;
- a page titled "Code signing policy" on the project's website, linked from
  the home page and the download section, with the team roles, the
  attribution "Free code signing provided by SignPath.io, certificate by
  SignPath Foundation" and the privacy statement;
- the product name and version in the signed files' metadata, which
  `desktop/packaging/interlinea.rc.in` and the installer script set;
- a person approving every signing request.

Once SignPath has accepted the project:

1. In SignPath, make a project for this repository with GitHub.com as its
   trusted build system, an artifact configuration that signs every
   `.exe` in the zip it receives, and a signing policy (usually
   `release-signing`).

   ```xml
   <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
     <zip-file>
       <pe-file path="*.exe" max-matches="unbounded">
         <authenticode-sign />
       </pe-file>
     </zip-file>
   </artifact-configuration>
   ```

2. In the repository's settings, Secrets and variables, Actions:
   - secret `SIGNPATH_API_TOKEN`: the API token of a SignPath user who may
     submit signing requests;
   - variable `SIGNPATH_ORGANIZATION_ID`: the organization's ID, in the
     address of the SignPath web page;
   - variable `SIGNPATH_PROJECT_SLUG`: the project's slug;
   - variable `SIGNPATH_SIGNING_POLICY_SLUG`, if the policy is not called
     `release-signing`.

The next release then submits `Interlinea.exe` and `interlinea-core.exe`
before the installer is made, and the installer after it, waits for the
approval, puts the signed files in place and checks their signatures
before anything is published.

## macOS, with a Developer ID

Notarization needs a membership of the Apple Developer Program.

1. In the Apple Developer account, make a **Developer ID Application**
   certificate, and export it with its private key from Keychain Access as
   a `.p12` file with a password.
2. At account.apple.com, make an app-specific password for the
   notarization.
3. In the repository's settings, Secrets and variables, Actions, add the
   secrets:
   - `MACOS_CERTIFICATE`: the `.p12` file in base64
     (`base64 -i certificate.p12 | pbcopy`);
   - `MACOS_CERTIFICATE_PASSWORD`: its password;
   - `APPLE_ID`: the Apple Account of the membership;
   - `APPLE_TEAM_ID`: the team's ID, in the membership details;
   - `APPLE_APP_PASSWORD`: the app-specific password.

The next release then signs every library, the core and the app with the
hardened runtime and a timestamp, signs the disk image, has Apple notarize
it, staples the ticket to it and checks it as macOS will.

## Trying it without a release

The release workflow started by hand (Actions, release, Run workflow, a
version such as `0.0.0-test`) builds and signs everything without
publishing; the signed files are attached to the run.
