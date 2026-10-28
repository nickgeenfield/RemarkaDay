# reCalendar

<img src="assets/icon.png" alt="reCalendar icon: a month grid with a red header, blue days and one highlighted day" width="128" height="128">

reCalendar is a Windows installer, and a small set of improvements, for [reMarkable Calendar](https://github.com/nguyer/remarkable-calendar) by Nicko Guyer (nguyer): an [AppLoad](https://github.com/asivery/rm-appload) app for the reMarkable Paper Pro that syncs with Google Calendar. You get week and day views, two-way Google Calendar sync with an offline queue, and the ability to create, edit and delete events. reCalendar adds a close button in the week toolbar, which returns to AppLoad, and a long-press on an event in the week or day view, which opens the delete confirmation. Upstream publishes no licence file, so this repository does not contain upstream source or binaries. It links to that project, credits it below, and carries the installer plus the reCalendar patch.

## Features

| | Feature | What you get |
| --- | --- | --- |
| ☑ | Week and day views | A seven-day week, and a day timeline with an all-day area. Swipe, or use the arrows and Today. |
| ☑ | Two-way Google Calendar sync | Download calendars and send changes back. Edits made offline wait in a queue and go to Google on the next sync. |
| ☑ | Create, edit and delete | Manage events with the on-screen keyboard, including an individual occurrence of a recurring event. |
| ☑ | Long-press delete | Press and hold an event in the week or day view (700 milliseconds) to open the delete confirmation. |
| ☑ | Close button | In the week toolbar, **Close reCalendar** returns you to AppLoad. |

The rest of the app is upstream's: several Google calendars and their colours, local-only events, an offline date window, and display settings (12- or 24-hour time, fixed or scrolling day). Defaults there are 12-hour time, a fixed 7am–9pm day, and about one month back and six months ahead cached. See the [upstream README](https://github.com/nguyer/remarkable-calendar) for that full list. Editing a whole recurring series, managing guests, and a continuous sync timer are not implemented.

## Requirements

- A reMarkable Paper Pro, with developer mode on. The AppLoad manifest uses the Paper Pro Move aspect ratio (`move`). Upstream designed the app for the Move and also reports that it runs on the full-size Paper Pro.
- [XOVI](https://github.com/asivery/rm-xovi-extensions) and [AppLoad](https://github.com/asivery/rm-appload), already installed and working.
- A Windows 10 or Windows 11 PC on the same Wi-Fi network as the tablet.
- For Google sync: a Google account, and a free [Google Cloud](https://console.cloud.google.com/) project of your own. Local-only events do not need Google.

## Installation

1. Download `reCalendarInstaller.exe` from the [Releases](https://github.com/nickgeenfield/reCalendar/releases) page.
2. Windows SmartScreen may warn that it does not recognise the app. Choose **More info**, then **Run anyway**.
3. On the tablet, switch on Wi-Fi and unlock the device. Close reCalendar if it is already open.
4. Find the tablet's IP address under **Settings > General > Help > About**.
5. The SSH user is `root`. The password is under **Settings > General > Help > About > Copyrights and licences** (scroll down). Developer mode is required. The password is case-sensitive and can change after a software update.
6. Run the installer. Enter the IP address, the user `root`, and the password. The password is not shown as you type.
7. When the menu appears, choose:
   - **1** — Install/Update
   - **2** — Google sync setup
   - **3** — Uninstall
   - **4** — Quit
8. After install or update, on the tablet open AppLoad and tap **Reload**. If reCalendar still does not appear, restart the UI with mods, for example via reManager.
9. For Google sync, keep reCalendar closed and continue with [Google sync setup](#google-sync-setup). You can do that straight away (option 2) or run the installer again later.

Install and update replace the AppLoad files under `/home/root/xovi/exthome/appload/remarkable-calendar`. They leave the event database and any existing Google credentials alone.

**Uninstall** (option 3) asks before it removes the app. A second question asks whether to delete Google credentials (`/home/root/.config/remarkable-calendar`) and the event database (`/home/root/.local/share/remarkable-calendar`). Reload AppLoad afterwards, or restart the UI with mods, so the icon disappears.

Option 4, or answering no to the uninstall question, changes nothing.

## Google sync setup

Google sync uses an OAuth client in a Cloud project you control. This is once per account. Afterwards the tablet syncs on its own. Keep reCalendar closed until the initial sync has finished, so the app does not save UTC before you have chosen a timezone.

1. Open [Google Cloud Console](https://console.cloud.google.com/) and create a project (or select one).
2. In the API Library, enable **Google Calendar API**.
3. Open the OAuth consent screen. Choose **External**, and leave the app in **Testing**.
4. **Add yourself as a test user.** If you skip this, Google refuses sign-in with **Access blocked: … has not completed the Google verification process**.
5. Create an OAuth client of type **Desktop app**. Download the JSON and keep it on the PC. The installer asks for this file as `client.json`. It is not uploaded to the tablet.
6. Run the installer and choose **2** (Google sync setup). Give it the path to `client.json`.
7. A browser opens for Google sign-in. Grant both Calendar permissions (read, and edit events). If Google says **Google hasn't verified this app**, choose **Continue**. That message is normal for a personal Cloud project in Testing; it is not the same failure as forgetting the test user.
8. The installer uploads credentials to the tablet and can run the first sync. When it asks for a timezone, use a full IANA name such as `Europe/London`. **`London` on its own fails with `ParseError`.** Other examples: `Europe/Paris`, `America/New_York`.

Then, on the tablet: AppLoad → **Reload** → reCalendar → the gear icon. Check the display timezone, refresh the calendar list, tick the calendars you want, and choose **Save & sync**.

`client.json` (the Desktop client you downloaded) and `google.json` (the refresh token on the tablet) are different files. See [Security](#security).

## Usage tips

| To… | Do this |
| --- | --- |
| Move between weeks or days | Swipe left or right, or tap the toolbar arrows. |
| Return to today | Tap **Today**. |
| Open a day | Tap a date, **More events…**, or the day-view control. |
| Read an event | Tap it. Tap **X** to go back. |
| Create an event | Tap **+**, choose a calendar, fill in the form, and save. |
| Keep an event only on the tablet | Choose **Local events** in the editor. |
| Edit an event | Open it and tap the pencil. |
| Delete an event | Open it and tap the bin, or press and hold the event in the week or day view until the delete confirmation appears. |
| Leave the app | In the week toolbar, tap **Close reCalendar**. |
| Sync | Tap **Sync** while the tablet is on Wi-Fi. Offline edits stay queued until then. |
| See queued changes | Open **Settings**. |

The editor wants dates as `YYYY-MM-DD` and times as `HH:MM` in 24-hour form, even if the week is shown in 12-hour time. A long-press delete is ignored while a sync is running, and it does not offer delete for a calendar you cannot write to. Saving on the tablet does not mean Google has accepted the change yet. Settings shows sync errors; discarding a queued change does not undo something Google has already accepted.

## Troubleshooting

| Problem | What to check |
| --- | --- |
| SmartScreen blocks the installer | **More info**, then **Run anyway**. |
| Cannot connect | Same Wi-Fi, tablet on and unlocked, developer mode on. IP from **Settings > General > Help > About**. User `root`. Password from **Copyrights and licences**, typed with the original case. Over USB the address is usually `10.11.99.1`. |
| "XOVI isn't installed" or AppLoad is missing | Install XOVI and the AppLoad extension first. The installer does not change the tablet in that case. |
| reCalendar does not appear, or the UI looks old | Close the app, AppLoad → **Reload**, then open it again. If it is still missing, restart the UI with mods (for example via reManager). |
| **Access blocked: … has not completed the Google verification process** | OAuth consent screen is **External** and in **Testing**, and the Google account is added as a test user. Then sign in again. |
| **Google hasn't verified this app** | Choose **Continue**. Expected for your own Testing client. |
| Timezone `ParseError` | Use a full IANA name (`Europe/London`). A city alone (`London`) is rejected. |
| Times are wrong | Settings → display timezone, again as a full IANA name. If Settings was saved before the first sync, it may have stored UTC. Set the timezone in the app and sync again. |
| Calendars or events missing | Wi-Fi, option 2 completed, Settings → refresh the list, calendar ticks, and the offline date window. |
| Cannot change a Google event | Grant both permissions at sign-in (read and edit), and use a calendar you can write to. |
| Sign-in times out | The installer waits five minutes. Run option 2 again. If the browser does not open, paste the link it prints. |

## Building from source

The release executable is the supported way to install. To build it yourself, produce `resources.rcc` and `backend/entry` first. Those steps, including upstream commit `5ee822fffa0d2b7fe462cb73281fa29df3aaaeae` and `patches/recalendar-changes.patch`, are in [installer/payload/README.md](installer/payload/README.md). `manifest.json` and `icon.png` are already in `installer/payload/`.

`go.mod` is inside `installer/`. From the repository root, after the payload is in place, build the Windows installer with:

```sh
cd installer
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags '-s -w' -o reCalendarInstaller.exe .
```

From a module rooted at the repository, the same build is `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags '-s -w' -o reCalendarInstaller.exe ./installer`. In this tree `go.mod` is already inside `installer/`, so the package path is `.`. `CGO_ENABLED=0` keeps the executable free of a C toolchain. The result is `installer/reCalendarInstaller.exe`. Executables and zip archives are gitignored.

Check the installer package with:

```sh
cd installer
go vet ./...
go test ./...
```

## Roadmap

Handwriting pad: long-press an empty slot, write with the pen, recognise the handwriting to text with [MyScript](https://www.myscript.com/), confirm the text, and create the event.

## Credits

- **[nguyer/remarkable-calendar](https://github.com/nguyer/remarkable-calendar)** by Nicko Guyer. reCalendar is that app plus the patch in `patches/recalendar-changes.patch` (close button, long-press delete, and the reCalendar name) and the icon in `assets/icon.png`. The calendar itself — sync, storage, views and the QML shell — is his work. Thank you.
- **[AppLoad](https://github.com/asivery/rm-appload)** by asivery, which loads the app on the tablet.
- **[XOVI](https://github.com/asivery/rm-xovi-extensions)** by asivery, which AppLoad runs on.

## Licence

The installer in `installer/` is by Nick Greenfield.

The upstream app is nguyer's and is published without a licence file. This repository does not copy that source or those binaries. Building a payload means checking out upstream yourself, at `5ee822fffa0d2b7fe462cb73281fa29df3aaaeae`, and applying `patches/recalendar-changes.patch`. Comply with whatever terms apply to that tree before you redistribute a build that contains it.

## Security

The installer never prints secrets. The tablet password is read without echoing. Google client secrets, refresh tokens, access tokens and the PKCE verifier are not written to the console; if a sync message looks as though it contains one, that output is omitted. The browser link used for sign-in is printed so you can open it yourself. It carries the OAuth client id and a one-time challenge, not the refresh token.

Credentials are stored on the tablet at `/home/root/.config/remarkable-calendar/google.json`, with private permissions: the file is mode `600` and the directory is mode `700`. The installer does not write `google.json` onto the PC. `client.json` stays on the PC; do not commit it, and do not send it to the tablet. The SSH connection does not pin the tablet host key, so run the installer on a network you trust.
