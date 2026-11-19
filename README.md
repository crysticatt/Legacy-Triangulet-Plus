# Triangulet Opener

**Open capsules on [legacy Triangulet](https://tri.pengpowers.xyz) around the clock, from the system tray, even before you sign in to Windows.**

Fast (around **12 packs per second, over 1 million packs a day** on the author's setup), quiet (lives in the tray), and smart (hunts the capsules with the best Mystical odds). One small `.exe`, no installer.

> **Unofficial.** This is a fan-made tool and is not affiliated with Triangulet or its owner. It sends requests to the site as your account. Opening very fast, or with many workers, may get an account limited or banned. You use it at your own risk.

---

## Download

Get **`TrianguletOpener.exe`** from the [latest release](../../releases/latest). Put it in a folder and run it. Windows 10 or 11, 64-bit.

Windows SmartScreen may say "unknown publisher" the first time, because the file is not code-signed. Choose **More info**, then **Run anyway**.

## Quick start

1. Run `TrianguletOpener.exe`.
2. Type your Triangulet username and password and press **Save and log in**.
3. Pick a mode (try **Hunt**) and press **Start opening**.
4. Close the window. It hides in the system tray and keeps opening. Left-click the tray icon to bring the window back; right-click for the quick menu.

---

## Highlights

### Fast
- **About 12 packs per second, over 1 million a day** in the author's testing. Your speed depends on your connection, the site, and your settings.
- Up to **16 workers**, each with several requests in flight at once ("At once"), sharing one warm connection pool.
- Delay between pulls down to **0.000001 ms**.
- **No rate-limit waiting**: a "slow down" reply just means try again right away.
- **Test limits** finds a good Workers x At once setting for your connection by reading from the site. It spends no coins.
- Stops **exactly** when your coins can't pay for another capsule (25 coins each).

### Starts before Windows does
Normally Windows only runs startup apps after you unlock your PC. This app can do better:
- **Run at startup**: starts hidden in the tray when you sign in and begins opening right away.
- **Before sign-in**: creates a scheduled task that opens capsules at boot, **before you unlock Windows**. Turn it on with one switch (Windows asks for admin approval once). When you sign in, the normal app takes over smoothly. There is no tray icon or Discord status until you sign in.
- If the login token expires or fails, it logs in again every 10 seconds until it works. It never gives up on its own (a wrong password is the one exception, and it tells you).

### Discord presence
Show your friends what you're doing: **"Opening Capsules"** appears as your Discord activity while the opener runs.
- Needs the Discord desktop app open on the same PC.
- Turn it on or off with `discordOn` in `config.json`.
- To show the logo, upload `triangulet-logo.png` as an Art Asset named **`logo`** in your own Discord application and put its Application ID in `discordId`. A default is built in.
- Optional **webhook**: put a Discord webhook URL in `webhookUrl` to get a message in a channel whenever you pull a Chroma or a Mystical.

### Hunting
- **Fixed**: every worker opens the capsule you pick.
- **Next on Mystical**: after a Mystical, moves on to the next capsule in the list.
- **Hunt**: aims workers at the capsule with the best Mystical odds. *Focus* puts every worker on the best one; *Split 2/3/4* spreads them over the top capsules. *Chase ones I don't own* goes after Mysticals you are missing. *Stop after N Mysticals* ends the run for you.

### Know what you have
- **Trians**: your real inventory, an Owned / Missing switch, a collection progress bar, and search.
- **Log**: every Epic-or-better pull and every new trian, also saved to `pull-log.csv`.
- **History**: capsules per hour or day, Mystical and Chroma markers, and your coins over 24 h / 7 days / 30 days.
- **Stats** and **Hunt** tabs: live pulls-per-minute graph, rarity bars, packs left, packs since your last Mystical, measured "1 in N".
- **Auction**: list trians for sale.
- **Alerts**: sound and tray notice for rare pulls, goal reached, low packs, out of coins, or a wrong login.

### More
- **Multiple accounts**: save several logins and switch with `<` `>`. Opening restarts on the new account if it was running.
- **Updates**: checks this repo's latest release and shows an **Update** button when there is a newer version. It downloads only from GitHub and checks the file against GitHub's checksum before installing. Turn the check off with `"noUpdateCheck": true` in `config.json`.
- Resizable 16:9 window that hides to the tray.

---

## Files and privacy

Everything is stored on your PC in `%APPDATA%\TrianguletOpener`: `config.json`, `pull-log.csv`, `history.json`, `opener.log`.

**`config.json` holds your username and password in plain text** so the app can log in again by itself. Don't share it or upload it anywhere. The app only talks to the Triangulet site, to GitHub (to check for updates), and to Discord if you turn that on.

## Build from source

Needs [Go](https://go.dev/dl/) 1.21 or newer and Python with Pillow (only to make the icon resource).

```sh
python tools/genres.py 35
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w -H=windowsgui" -o TrianguletOpener.exe .
go test .
```

The app uses only the Go standard library. The window is drawn by a small built-in renderer, with Win32 calls for the window, tray and text boxes.

## Known limits

- Not code-signed (see above).
- Speeds are not guaranteed: the site's own limits are the real ceiling, and very high settings can cause errors or an account ban.
- The window, tray and boot-time code is cross-compiled and the opener engine is covered by automated tests, but the Windows parts have had little testing on real machines. Please report anything odd by opening an issue.

## License

[MIT](LICENSE).
