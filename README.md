# triangulet opener

**open capsules on [legacy triangulet](https://tri.pengpowers.xyz) around the clock, from the system tray, even before you sign in to windows.**

fast (around **12 packs per second, over 1 million packs a day** on the author's setup), quiet (lives in the tray), and smart (hunts the capsules with the best mystical odds). windows installer or a single portable `.exe`, plus a mac version.

> **unofficial.** this is a fan-made tool and is not affiliated with triangulet or its owner. it sends requests to the site as your account. opening very fast, or with many workers, may get an account limited or banned. you use it at your own risk.

---

## download

get these from the [latest release](../../releases/latest). the version is in every file name (for example `v36`).

| system | file | what it is |
|---|---|---|
| windows 10/11 | `trianguletopener-setup-vnn.exe` | **installer.** puts the app in your account, adds desktop and start menu shortcuts and an uninstall entry. no admin needed. |
| windows | `trianguletopener-vnn.exe` | portable: runs from any folder, no install. |
| mac | `trianguletopener-mac-vnn.zip` | both apple silicon and intel apps plus an install script. see the mac notes inside. |

windows smartscreen may say "unknown publisher" the first time, because the file is not code-signed. choose **more info**, then **run anyway**. on a mac, right-click the install script (or the app) and choose **open** the first time.

the mac version has no window of its own: it runs in the background and shows a dashboard in your browser with the same controls.

## quick start

1. run `trianguletopener.exe`.
2. type your triangulet username and password and press **save and log in**.
3. pick a mode (try **hunt**) and press **start opening**.
4. close the window. it hides in the system tray and keeps opening. left-click the tray icon to bring the window back; right-click for the quick menu.

---

## highlights

### fast
- **about 12 packs per second, over 1 million a day** in the author's testing. your speed depends on your connection, the site, and your settings.
- up to **16 workers**, each with several requests in flight at once ("at once"), sharing one warm connection pool.
- delay between pulls down to **0.000001 ms**.
- **no rate-limit waiting**: a "slow down" reply just means try again right away.
- **test limits** finds a good workers x at once setting for your connection by reading from the site. it spends no coins.
- stops **exactly** when your coins can't pay for another capsule (25 coins each).

### starts before windows does
normally windows only runs startup apps after you unlock your pc. this app can do better:
- **run at startup**: starts hidden in the tray when you sign in and begins opening right away.
- **before sign-in**: creates a scheduled task that opens capsules at boot, **before you unlock windows**. turn it on with one switch (windows asks for admin approval once). when you sign in, the normal app takes over smoothly. there is no tray icon or discord status until you sign in.
- if the login token expires or fails, it logs in again every 10 seconds until it works. it never gives up on its own (a wrong password is the one exception, and it tells you).

### discord presence
show your friends what you're doing: **"opening capsules"** appears as your discord activity while the opener runs.
- needs the discord desktop app open on the same pc.
- turn it on or off with `discordon` in `config.json`.
- to show the logo, upload `triangulet-logo.png` as an art asset named **`logo`** in your own discord application and put its application id in `discordid`. a default is built in.
- optional **webhook**: put a discord webhook url in `webhookurl` to get a message in a channel whenever you pull a chroma or a mystical.

### hunting
- **fixed**: every worker opens the capsule you pick.
- **next on mystical**: after a mystical, moves on to the next capsule in the list.
- **hunt**: aims workers at the capsule with the best mystical odds. *focus* puts every worker on the best one; *split 2/3/4* spreads them over the top capsules. *chase ones i don't own* goes after mysticals you are missing. *stop after n mysticals* ends the run for you.

### know what you have
- **trians**: your real inventory, an owned / missing switch, a collection progress bar, and search.
- **log**: every epic-or-better pull and every new trian, also saved to `pull-log.csv`.
- **history**: capsules per hour or day, mystical and chroma markers, and your coins over 24 h / 7 days / 30 days.
- **stats** and **hunt** tabs: live pulls-per-minute graph, rarity bars, packs left, packs since your last mystical, measured "1 in n".
- **auction**: list trians for sale.
- **alerts**: sound and tray notice for rare pulls, goal reached, low packs, out of coins, or a wrong login.

### settings (button at the bottom left)
- **hourly summary**: each hour, a notification with packs opened, mystical, chroma and your coin change (also to ntfy / discord webhook).
- **least and wish modes**: go for the trian you own the fewest of (random if tied), or only among the trians you starred in the **wishlist** tab.
- **luck tab**: how long your first full collection will take, how lucky you have been compared with the site's odds, and a **copy diagnostics** button for bug reports.
- **downtime hours**: type the hours the opener should stay paused (for example `3:20pm-4pm`); it resumes by itself.
- **keep pc awake**: while opening, your pc does not sleep but the screen can turn off, so you can leave it running with the monitor off.
- **theme**: dark, light or blue.
- **updates**: check now, turn the check off, or install updates by themselves while the window is hidden. after an update, a "what's new" box shows the release notes once.
- **password protection**: your saved password is encrypted for this pc (windows) or kept in the keychain (mac), not stored as plain text.
- **phone alerts**: rare pulls and warnings sent to your phone with the free [ntfy](https://ntfy.sh) app. make a random private topic, subscribe, press test.
- **backup**: export your settings, logins and history as one passphrase-encrypted file, and import it on another pc.

### more
- **multiple accounts**: save several logins and switch with `<` `>`. opening restarts on the new account if it was running.
- **updates**: checks this repo's latest release (windows and mac) and shows an **update** button when there is a newer version. it downloads only from github and checks the file against github's checksum before installing. you can turn the check off in settings.
- resizable 16:9 window that hides to the tray.

---

## files and privacy

everything is stored on your pc in `%appdata%\trianguletopener`: `config.json`, `pull-log.csv`, `history.json`, `opener.log`.

**`config.json` holds your username and password in plain text** so the app can log in again by itself. don't share it or upload it anywhere. the app only talks to the triangulet site, to github (to check for updates), and to discord if you turn that on.

## build from source

the code is in the [`src`](src) folder (windows app, mac dashboard, and the windows installer in `src/installer`).

needs [go](https://go.dev/dl/) 1.21 or newer and python with pillow (only to make the icon resource).

```sh
cd src
python tools/genres.py 36
set goos=windows
set goarch=amd64
go build -trimpath -ldflags="-s -w -h=windowsgui" -o trianguletopener.exe .
go test .
```

the app uses only the go standard library. the window is drawn by a small built-in renderer, with win32 calls for the window, tray and text boxes.

## known limits

- not code-signed (see above).
- speeds are not guaranteed: the site's own limits are the real ceiling, and very high settings can cause errors or an account ban.
- the window, tray and boot-time code is cross-compiled and the opener engine is covered by automated tests, but the windows parts have had little testing on real machines. please report anything odd by opening an issue.

## license

[mit](license).

## antivirus warning

the app is not code-signed, and the **before sign-in** option creates a hidden startup task. some antivirus programs (including windows defender) flag that as a trojan. it is a false alarm, but you can avoid it by using **run at startup** instead. if defender removes the file, restore it in windows security > protection history and send it to microsoft (security intelligence > submit a file > "i believe this file is clean").
