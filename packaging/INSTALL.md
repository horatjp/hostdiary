# hostdiary installation guide

hostdiary records CPU, memory, disk, network and other measurements for one Linux server every minute,
and writes browser-viewable pages and JSON. It has no persistent daemon, database or outbound communication.
See the included `README.md` for detailed specifications.

## 0. Archive contents

| File | Contents |
|---|---|
| `hostdiary` | Standalone executable (nothing else to install) |
| `INSTALL.md` | This installation guide (English, canonical) |
| `INSTALL.ja.md` | Japanese translation of this guide |
| `config.example.json` | Example configuration file |
| `nginx-hostdiary.conf.example` | Example nginx configuration for serving the viewer behind Basic authentication |
| `README.md`, `LICENSE` | Specifications (English) and license (MIT) |
| `SHA256SUMS` | SHA-256 checksums of the files above |

## 1. Check prerequisites

- OS: Debian 12 or later, or Ubuntu 22.04 or later
- CPU: must match `amd64` / `arm64` in the archive name
- systemd running as PID 1, and root access (sudo)

```sh
. /etc/os-release && echo "$PRETTY_NAME"
uname -m                    # x86_64: amd64 archive; aarch64: arm64 archive
ps -p 1 -o comm=            # Must print systemd
```

## 2. Extract and verify the contents

```sh
tar -xzf hostdiary-<version>-linux-<amd64|arm64>.tar.gz
cd hostdiary-<version>-linux-<amd64|arm64>
sha256sum -c SHA256SUMS     # All entries must report OK
./hostdiary --version       # Must print the version
```

If you received a SHA-256 checksum for the archive itself, compare it with `sha256sum <archive>` before extracting.

## 3. Configure (optional)

hostdiary works without a configuration file (devices are detected automatically, the system hostname is used, and applications are not recorded).
To also record process counts and memory for applications (such as nginx and php-fpm), put the configuration in place before installing.

```sh
sudo install -d -o root -g root -m 0755 /etc/hostdiary
sudo install -o root -g root -m 0644 config.example.json /etc/hostdiary/config.json
sudoedit /etc/hostdiary/config.json
```

- `apps`: a list of display names and process names from `/proc/<pid>/stat` (up to 15 bytes, exact match). A process is counted if any name matches
- `net`, `disk`, `diskio`: restrict recorded interfaces, mount points and block devices by name (automatic when empty)
- `dirs.enabled`: daily directory usage measurement. **Directory names (such as `/var/www/<site>` and `/home/<user>`) appear in the viewer**, so set this to `false` if you do not want to expose them
- Unknown fields, incorrect types and comments cause errors (JSON does not support comments)

## 4. Install

```sh
sudo ./hostdiary install
```

This installs `/usr/local/bin/hostdiary`, the four units `/etc/systemd/system/hostdiary-{collect,dirs}.{service,timer}`,
and `/etc/hostdiary/` (created if absent). Data accumulates in `/var/lib/hostdiary/`.

Installation runs one trial collection and enables the timers only if it succeeds. These two lines at the end indicate success:

```
timer: enabled=enabled, active=active
dirs timer: enabled=enabled, active=active
```

If it fails, check the displayed messages and `journalctl -u hostdiary-collect.service`.
You may delete the extracted directory after installation (the executable is installed in `/usr/local/bin`).

## 5. Verify operation

```sh
systemctl list-timers 'hostdiary-*.timer'     # collect: every minute; dirs: around 04:30 every morning
systemctl is-failed hostdiary-collect.service # inactive means it has not failed
ls /var/lib/hostdiary/public/                 # Contains index.html, index.json, data/, etc.
```

To measure directory usage immediately, run `sudo systemctl start hostdiary-dirs.service`.

## 6. View the pages

The viewer is `/var/lib/hostdiary/public/index.html`. It can open without communicating with the server, so use either of these methods:

- **Copy it locally** (no configuration changes): `rsync -a <server>:/var/lib/hostdiary/public/ ./hostdiary-view/` → open `index.html` in a browser
- **Serve it behind nginx authentication**: add `nginx-hostdiary.conf.example` to an existing HTTPS server, create a password file, then run `sudo nginx -t` → `sudo systemctl reload nginx`.
  If the server already uses Basic authentication, you may remove the two `auth_basic` lines and use that authentication

After making it accessible externally, always check the following:

- Without authentication, `/hostdiary/`, `/hostdiary/index.json`, `/hostdiary/index.js`, `/hostdiary/summary.json` and `/hostdiary/llms.txt` return 401 (`/hostdiary` redirects to `/hostdiary/` with 308)
- With authentication, the viewer opens
- The same files are not accessible through other ports, direct IP access or a server with a different Host name (for example, they return 404)
- Serve only `public/`, never its parent `/var/lib/hostdiary/` (which contains state.json and other files)

The viewer uses Japanese when the browser language is Japanese, and English otherwise.

To have an AI check the recorded state, provide the authenticated `/hostdiary/` URL and ask it to start with `llms.txt` (the reading guide for AI).
Daily summaries are in `summary.json` (numbers only, without judgments of normal or abnormal).

## 7. Update

Extract the new archive, verify it as in step 2, then run `sudo ./hostdiary install` again.
Data and configuration are preserved. The viewer files are replaced with the new version during the next collection (within one minute).

## 8. Uninstall

```sh
sudo hostdiary uninstall
```

This stops the timers and removes the executable and units. **Data (`/var/lib/hostdiary/`) and configuration (`/etc/hostdiary/`) are preserved.**
If you no longer need them, follow the displayed instructions to remove them manually. If you added nginx configuration, remove it too, then run `nginx -t` → reload.
