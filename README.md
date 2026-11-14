# hostdiary

日本語のインストール手順は `packaging/INSTALL.ja.md`(配布物では `INSTALL.ja.md`)にあります。

A resource diary for one Linux host: minute-by-minute measurements, daily summaries,
and browser graphs. A short-lived collector reads procfs every minute and appends
public time-series files. People browse HotSaNIC-style overview graphs; AI-first
means giving external AI tools JSON and a reading guide, without embedding AI.
No database, daemon, outbound API, CDN, or additional runtime is required.

The diary records measurements, OOM kills, observed application starts, missing
observations, directory changes (by comparing daily lists), and estimated boots.
It does not record reasons, service health or logs, and applies no thresholds or alerts.

The current version collects CPU, load, memory, swap, process states, TCP states,
network traffic per interface, filesystem capacity, block-device I/O, and process
count and RSS per configured application. Additional metrics include PSI pressure,
disk read/write time, swap in/out, OOM kills, application starts, TCP segments and
retransmissions, and NIC errors/drops. It keeps minute data and summaries for the
newest eight UTC day directories (more while rollup is failing), five-minute values
for 3 month files, hourly values for 25 month files, daily values for 3 year files,
31 daily directory lists, and past boots for 1095 days (about 3 years), plus the
current boot even when older. The 1095-day boot window and the three year files
of daily values do not cover the same period. Monthly traffic estimates are included. Target systems: Debian 12 and
Ubuntu 22.04 or later, on Linux amd64 or arm64.

## Build and install

Build with Go 1.26. There are no external Go modules. The HTML, JavaScript, CSS,
and uPlot distribution are embedded in the executable.

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o hostdiary ./cmd/hostdiary
# Use GOARCH=arm64 for an arm64 host.
```

To make distribution archives, run `scripts/package.sh`. It writes
`dist/hostdiary-<version>-linux-amd64.tar.gz` and `-linux-arm64.tar.gz`, each
containing the executable, English and Japanese installation guides (`INSTALL.md` and `INSTALL.ja.md`, from
`packaging/`), configuration and nginx examples, this README, the license and
`SHA256SUMS`, plus `dist/SHA256SUMS` for the archives.

Copy the binary to the target host. Review the configuration before installing;
the collector writes to `/var/lib/hostdiary/`. Installation requires root and
systemd running as PID 1, with `systemctl` available.

```sh
sudo ./hostdiary install
```

This installs the executable at `/usr/local/bin/hostdiary`, installs the embedded
four service/timer units (collect and dirs) at `/etc/systemd/system/`, and creates
`/etc/hostdiary/` if missing. Files are root:root, with mode 0755 for the executable
and 0644 for units. No configuration file is created or read by the installer.
Optionally create `/etc/hostdiary/config.json` using the example below before
installation. Keep it root-owned without group/other write access. Without it,
the collector uses automatic device discovery, the system hostname, and no apps.

After reloading systemd, installation runs a trial collection through the service,
under the same restrictions and 50-second timeout used by the timer. Only a
successful trial permits `systemctl enable --now hostdiary-collect.timer`.
Then installation enables `hostdiary-dirs.timer` with `enable --now` as well.
It does not run a trial directory scan; to measure now, use
`sudo systemctl start hostdiary-dirs.service` as shown in the summary.
Each systemctl child has a 90-second deadline; it is found at `/usr/bin/systemctl`
or `/bin/systemctl`, without using PATH.
Success requires both timers to report enabled and active. The summary shows
file changes, the installer version, `timer: enabled=…, active=…` and
`dirs timer: enabled=…, active=…` on failure too.

If the trial fails, inspect the displayed service status or
`journalctl -u hostdiary-collect.service`. A collect exit code of 1 can mean one
item failed even though data was written. The installer offers manual enabling
only when `systemctl show -p ExecMainStatus hostdiary-collect.service` confirms
exit 1: if acceptable, run `sudo systemctl enable --now hostdiary-collect.timer`. For
`clock_backward`, wait a little and retry (see Clock recovery for persistent cases).
Installation never stops an existing timer: on a failed update it can keep
running the installed executable. Fix the cause and retry, or stop it with
`sudo systemctl disable --now hostdiary-collect.timer`.
If the dirs timer remains active, the summary also gives
`sudo systemctl disable --now hostdiary-dirs.timer`.

Check the schedules with `systemctl list-timers 'hostdiary-*.timer'`. To serve
the history, see [Serve behind nginx authentication](#serve-behind-nginx-authentication).

### Update and uninstall

Build and copy the new executable, then run `sudo ./hostdiary install` again.
If the trial overlaps a timer run, it may report the old version's result or
`clock_backward`; for the latter, wait a little and retry.
Matching content, ownership and modes retain modification times. Incorrect
ownership or modes are repaired by replacing the inode through a temporary file,
isolating the installed file from old hardlinks and open descriptors. Installation
refuses symlinks, directories and special files at the five destination paths;
masked units stay masked and require an explicit `systemctl unmask` before retrying.
Differing unit content is replaced without a backup and reported.
Use `sudo systemctl edit hostdiary-collect.service` (or the timer) for local
overrides: drop-in directories are preserved. Data and configuration are kept.
The installed executable can also run `sudo hostdiary install` to restore its units.

```sh
sudo hostdiary uninstall
```

This disables and stops both timers, removes the executable and all four units,
and reloads systemd. An ongoing collection finishes normally (up to 50 seconds).
An ongoing directory scan can continue for up to 30 minutes. Uninstall reports
`dirs service: active=<value>` and, for `activating`, `active` or `deactivating`, prints
`systemctl stop hostdiary-dirs.service`; it does not stop that service automatically.
An executing oneshot service normally reports `activating`.
Its state is informational and does not change uninstall's exit code.
Symlinks (including masked units), directories and special files at the five
destination paths are left in place, and uninstall exits with code 1.
Data, configuration, drop-ins, and any leftover timer symlink are kept; the
command prints their paths and manual removal instructions. If disabling the
timer fails, removal still proceeds and the command exits with code 1. The final
summary queries both timers' active states (`timer: active=<value>` and
`dirs timer: active=<value>`) and gives a manual stop command for any timer
still active, which also exits 1. Failed timer state queries also exit 1;
inactive is expected after removal, including its normal exit code 3.
Review diagnostics and remaining links. There is no `--purge` option.

Install/uninstall accept no arguments (including `--root`). Exit codes are 0 for
success, 1 for an operational failure, and 2 for unmet prerequisites or invalid
arguments. The commands make no network requests and run only `systemctl`.

### Manual installation

Alternatively, place the binary and canonical units as root-owned files:

```sh
sudo install -o root -g root -m 0755 hostdiary /usr/local/bin/hostdiary
sudo install -d -o root -g root -m 0755 /etc/hostdiary
sudo install -o root -g root -m 0644 packaging/systemd/hostdiary-collect.service /etc/systemd/system/hostdiary-collect.service
sudo install -o root -g root -m 0644 packaging/systemd/hostdiary-collect.timer /etc/systemd/system/hostdiary-collect.timer
sudo install -o root -g root -m 0644 packaging/systemd/hostdiary-dirs.service /etc/systemd/system/hostdiary-dirs.service
sudo install -o root -g root -m 0644 packaging/systemd/hostdiary-dirs.timer /etc/systemd/system/hostdiary-dirs.timer
```

```sh
sudo systemctl daemon-reload
sudo systemctl start hostdiary-collect.service
# Continue only after the trial service succeeds.
sudo systemctl enable --now hostdiary-collect.timer
sudo systemctl enable --now hostdiary-dirs.timer
```

`hostdiary --version` prints the executable version. On a version change, the
next collection replaces the embedded viewer files. `public/assets/VERSION`
contains the executable version plus the first eight SHA-256 digits of the
embedded viewer assets. Matching version and fingerprint leave existing assets
untouched; missing asset files trigger a rewrite. Keep the binary and unit files
root-owned when updating them.

## Configuration

```json
{
  "hostname": "",
  "apps": {
    "nginx": ["nginx"],
    "php-fpm": ["php-fpm8.2", "php-fpm"]
  },
  "net": {"include": [], "exclude": []},
  "disk": {"include": [], "exclude": []},
  "diskio": {"include": [], "exclude": []},
  "dirs": {"enabled": true}
}
```

- `hostname` overrides the displayed hostname. Empty means the system hostname.
- Each app name is a display name; its aliases match `/proc/<pid>/stat` comm
  exactly, with OR semantics. Aliases must be nonempty and at most 15 **bytes**,
  because Linux truncates comm to 15 bytes. Zombies are excluded. RSS sums count
  shared memory more than once. Rename the app when changing its matching rules
  so that distinct definitions do not share one history.
- `net`, `disk`, and `diskio` use exact names: interface, mount point, and block
  device name, respectively. A nonempty `include` limits discovery to those
  names and overrides the default exclusions. Explicit `exclude` entries still
  win. A missing included target is reported as `not_found`.
- Default network exclusions: `lo`, `veth*`, `docker*`, `br-*`, and `virbr*`.
  Default block-device exclusions: `loop*`, `ram*`, `sr*`, `nbd*`, and `zram*`; partitions are
  never collected as separate I/O devices.
- Filesystem discovery only allows ext2/ext3/ext4/xfs/vfat, with a positive
  block major and mount root `/`. It uses the shortest mount point for each
  device. Includes do not bypass these safety constraints. Network, virtual,
  and bind filesystems are excluded before calling statfs.
- Unknown JSON keys, invalid types, empty app aliases, and overlong comm aliases
  are errors. Invalid configuration exits with code 2 without writing data.
  JSON comments are not supported. The public directory is fixed, not configurable.
- `dirs.enabled` defaults to true if omitted. Set false to disable directory
  scans and remove published daily lists on the next dirs invocation. See below
  for immediate removal and filesystem exclusions.

## Daily directory usage

`sudo hostdiary dirs` measures the same filesystems selected for the disk group,
including `disk.include`/`disk.exclude`. It walks one filesystem at a time,
does not follow symlinks, counts multiply linked non-directories once, and sums
allocated blocks (`st_blocks × 512`), including directories themselves.
No external `du` is used. Each filesystem lists the largest 20 directories at
depths 1 and 2 relative to its mount point. Parent/child values overlap; do not
add them. Files directly under the mount point contribute only to its total.
This total differs from filesystem used space, for example for deleted-but-open files.

The enabled-by-default `hostdiary-dirs.timer` runs at 04:30 in the host's local
time zone, with up to 10 minutes of random delay. A missed schedule triggers once
after startup. Files are named by the **execution start's UTC date**:
`data/dirs/YYYY-MM-DD.json` and `.js` (`HD.dirs(day, obj)`). A 04:30 run in Japan
uses the previous UTC date. Same-day runs replace that day's list.
Retention keeps the newest 31 valid JSON date names and always protects the day
just written; many future dates can therefore leave 32 dates. Other names are kept.

**Directory names are published**, including names such as `/home/<user>` or
`/var/www/<site>` at depth 2. Review this before enabling access to public data.
Two ways to limit disclosure:

- Set `"dirs":{"enabled":false}` to stop all scans. Existing daily JSON/JS
  lists are deleted on the next dirs execution, normally the next morning.
  To remove them immediately, run `sudo systemctl start hostdiary-dirs.service`
  after saving the configuration. The next collect removes their index entry.
- Use `disk.exclude` for selected mount points. This also removes those
  filesystems from capacity collection; it does not erase already published
  directory lists. For immediate removal of all old lists, use the first option.

The Directory usage table compares the latest day with exactly seven UTC days
earlier, or the oldest retained day if unavailable. Missing paths show `—`
with “not in top 20 that day”; absence is not zero.
If only one day exists, the comparison date, values and changes show `—`, with
“no earlier day to compare”; the latest day is never compared with itself.
Sizes use binary units and
changes may be negative. A partial measurement shows `partial (N entries skipped)`.
Unprintable path bytes and backslashes are represented with reversible `\xHH` escapes.

Exit codes are 0 for a complete run, disabled cleanup, or no selected filesystem;
1 for partial results or operational failure; 2 for invalid configuration;
75 if another dirs invocation holds the separate lock. No selected filesystem
logs `no filesystem selected` and preserves existing files. Disappearing children
(ENOENT/ENOTDIR) increase errors without making a run partial. Other unreadable
entries do. Diagnostics show at most 100 detailed walk errors, then the full count.

The dirs service has a 30-minute timeout, Nice=19, best-effort I/O priority 7,
MemoryMax=256M and GOMEMLIMIT=200MiB. It uses CAP_DAC_READ_SEARCH and omits
PrivateTmp and ProtectKernelModules so that the measured tree stays visible.
An extremely wide directory (roughly two million entries) or millions of hard
links may exceed its memory limit and leave a missing day. Check
`journalctl -u hostdiary-dirs.service`. Publication writes JS before JSON; on a
failed same-day replacement, the JS can be newer than the JSON. See the
[data format](docs/data-format.md) for retention and reading details.
An unresponsive network/FUSE mount beneath a selected filesystem can stall the
walk until its 30-minute timeout aborts the run, leaving that day's measurement missing.

## Read and view history

The collector writes `/var/lib/hostdiary/public/index.html`, `assets/`,
`index.json`, `index.js`, and `data/1m/<UTC-date>/<group>.jsonl` plus `.js` mirrors.
Completed UTC days are rolled up directly from minute values into
`data/5m/<UTC-month>/`, `data/1h/<UTC-month>/`, and `data/1d/<UTC-year>/`, each
with JSONL and JavaScript mirrors. Every coarse interval stores the mean and maximum of non-null values,
a sample count and the actual last observation time. All-null columns remain
null. Maxima from different columns may come from different minutes: do not add them.
The JSONL header defines columns and units; following arrays contain Unix time
and values. Missing values are `null`. JavaScript mirrors call `HD.l(path, line)`
and the index calls `HD.index(obj)` (schema 1). The public schema reference is
[docs/data-format.md](docs/data-format.md).

The index includes the last collection time, recorded boot times, unavailable
items, units and column descriptions, and the days and last observation for each
group. CPU and counter rates start as null; they also become null after a boot
change or an invalid uptime interval. The valid interval is greater than zero
and at most 180 seconds. Counters that decrease produce null; negative CPU
column differences are clamped to zero before calculating percentages.
The count columns `oom_kills` and `started` have no interval upper limit: they
require a previous baseline in the same boot, but still count over long gaps.
`oom_kills` also requires a nondecreasing counter; `started` requires increasing
valid uptime, assumes USER_HZ=100, and counts only matching processes still running.
It misses processes that start and exit between collections. OOM counts include
cgroup memory-limit kills; compare them with available host memory.
On upgrade, absent new counter baselines produce null once; existing net/diskio
columns continue using their saved positions. `started` can use old uptime state.

Each collection also writes `summary.json`: for every retained UTC day and
group, the row count, missing minute slots and gaps, and per column the count,
minimum and maximum with their first times, mean, and (for `mem.oom_kills` and
`app.started` only) the daily sum. Units stay as in the minute files, so `/s`
values remain per second; no thresholds or judgements are included. Completed
days are cached outside `public/` in `summary-cache.json` and reused while the
day file is unchanged. `llms.txt` is a fixed English guide for AI readers: the
reading order, the format and the rules that are easy to get wrong. The index
points to both with `"guide": "llms.txt"` and `"summary": "summary.json"`.
Give an AI client the `/hostdiary/` URL (with authentication) and ask it to
start from `llms.txt`.

Unsupported sources are reported in the index without failing collection. If PSI
is unavailable, check kernel support and the `psi=1` kernel boot parameter.

Copy the **entire** public directory to a trusted computer and open `index.html`.
Both HTTP and `file://` use the local `.js` files through script tags; no browser
fetch or server is required. The viewer automatically uses Japanese when the browser's preferred language is Japanese, and English otherwise, while JSON descriptions stay in English.
Each chart has a table of minimum, average, maximum, latest and cursor values for the selected period; zooming does not change these statistics, and double-click restores the selected period.
The contents links follow the overview's group headings. The header shows the
last recorded boot (estimated), using the last `boots` entry, not the largest
timestamp. After boot identity or uptime could not be read, it may differ from
the current boot. Missing entries do not prove that no reboot occurred.
Dashed vertical lines mark recorded estimated boots on every chart, including
gaps and empty history. Lines at either end of the selected period are included;
zooming and resizing move them with the time axis without changing observations
or table statistics. The lines do not indicate normal or abnormal conditions.
Choose 1 hour, 1 day (default), or 1 week using minute
values, or 1 month (31 days) using five-minute values, 1 year (365 days) using
hourly values, or 2 years (730 days) using daily values. Windows end at the
index's last collection time, but coarse values cover completed UTC days only;
choose 1 day for today. Coarse charts show interval means; only line panels also show faint maximum lines, and uPlot legends are hidden.
Coarse tables weight averages by samples and take maxima from minute maxima (derived series use the maximum ratio); minimum and latest values are interval means.
Times use four-digit years and a 24-hour clock in the browser's time zone except daily
observations in the 2-year table and cursor, which use UTC dates; axis ticks retain the interval-end date. Network
charts show bit/s; stored network data remains bytes/s. Nulls and collection
gaps break the line. Groups absent from the latest collection show
`stopped (last: …)` while their history remains available.
Values with units `(none)`, `count` or `count/s` omit the unit suffix; count axes use localized labels and byte axes use binary increments.

The PSI chart shows the percentage of interval time tasks stalled on CPU, memory
or I/O (some: at least one task; full: all non-idle tasks). Disk I/O is split into
throughput, operations, utilization, and average completion time. The last
chart divides `read_time / read_ops` and `write_time / write_ops` in milliseconds;
it is not application response time. The separate TCP retransmission ratio chart
divides `retrans_segs / out_segs × 100`; it is a rough indicator, not packet loss
rate. OutSegs includes pure ACKs and excludes retransmissions. NIC errors/drops
are driver counters, not end-to-end packet loss; virtual NICs may report zero.
Derived panels appear only when at least one derived value is non-null; a
calculated zero is a valid value.

Ratios are calculated from same-row materials, never stored or derived from
maxima. Missing/nonfinite materials or a nonpositive denominator leave a gap.
Ratios of coarse means approximate count-weighted values only when collection
intervals are nearly constant and both materials have matching null patterns.
Skipped collections or columns added mid-day can skew them. 1-minute rates and coarse means are
rounded to three decimals, so ratios with very small numerators or denominators are imprecise.
If a coarse material mean is zero but its maximum is positive, rounding erased the value and the
viewer leaves the ratio blank. Coarse `oom_kills`/`started` mean times samples is
an approximate interval total for AI readers, not a stored sum: samples is not
the per-column non-null count and means are rounded. The OOM panel in the memory
group shows per-collection counts for minute values and interval means with faint
maximum lines for coarse values; it does not display interval totals. A positive
maximum indicates kills even when the mean is zero. Collection gaps and intervals
are not corrected, and cgroup memory-limit kills are included.

Network groups also show traffic estimates for the current UTC month and the
last 30 UTC days including today, ending at the index generation time. Each
completed day contributes its mean rate times 86400; today contributes its mean
rate times elapsed seconds. Missing time is extrapolated from observed values.
A day with either direction missing counts once as missing; any valid direction
still contributes. Samples count minute rows, including nulls: compare with the
elapsed minutes. Totals use binary units such as GiB, rather than decimal GB.
The separate selected-period totals beneath network charts use bit through Pbit, summing observed bit/s × samples × 60 (samples = 1 for minute values) without filling gaps or correcting non-60-second intervals.

The last collection time stays at the top, including when all collection stops.
Reload the page to read a newer index. No threshold, health judgment, or alert is
generated. Public files can reveal host and application details: keep them
behind authentication and share copies only with trusted readers.

## Serve behind nginx authentication

Inside your existing HTTPS server, protect the entire prefix, including all
assets, JSON and JavaScript files. Create the password file separately using
your normal nginx authentication procedure; do not put it in the public tree.

```nginx
location = /hostdiary {
    return 308 /hostdiary/;
}

location ^~ /hostdiary/ {
    alias /var/lib/hostdiary/public/;
    auth_basic "Resource history";
    auth_basic_user_file /etc/nginx/hostdiary.htpasswd;
    add_header Cache-Control "no-cache" always;
    add_header Cross-Origin-Resource-Policy same-origin always;
    autoindex off;
}
```

The CORP header prevents other sites from loading the JavaScript mirrors as
scripts and reading private history through an authenticated browser.
Adding `add_header` in this location disables the usual inheritance of parent
`add_header` directives: repeat required server-level headers, such as HSTS,
here as well. Review this for every nested location that overrides headers.

Publish **only** `public/`, never its parent: `state.json`, locks, and temporary
files stay outside the published tree. Validate the nginx configuration using
your normal deployment procedure. Confirm that unauthenticated requests cannot
read `index.html`, `index.json`, `index.js`, `summary.json`, `llms.txt`, or any
data file. Check the intended
hostname, direct IP access, and alternate Host names/default virtual hosts, so
that another server block cannot expose the same files without authentication.
AI clients should connect by the intended hostname over HTTPS, with the same
authentication as people, rather than connecting by IP.

## Privileges and systemd limits

The dirs service differs from collect: it omits PrivateTmp and ProtectKernelModules
to keep the measured tree visible, uses only CAP_DAC_READ_SEARCH, and has a 30-minute timeout.

The collector runs once per minute as root and then exits. Root permits complete
process visibility, including hosts using hidepid, without a separate service
account or per-directory permission grants. It accepts no network connections.
Anyone who can write to `/usr/local/bin` (such as members of Debian's `staff`
group) can replace the executable that runs as root; installation warns about
group/other writable permissions on this directory.

The service uses `ProtectSystem=strict` and `StateDirectory=hostdiary`, leaving
the state directory writable. It denies IP traffic, restricts socket families to
AF_UNIX, prohibits privilege escalation, isolates temporary files, and protects
kernel modules, tunables and control groups. Its capability bound is
`CAP_SYS_PTRACE CAP_DAC_READ_SEARCH`; process access needs these capabilities.
It intentionally does not use `ProtectProc`, `PrivateNetwork`, or `ProtectHome`,
which could hide the host data being measured. Each run has a 50-second timeout.
The collector sets umask 022 itself, so public files/directories are 0644/0755
even when run manually. Private state is written with mode 0600.

Exit codes: 0 means success (including expected unavailable items), 1 means at
least one unexpected failure, 2 means invalid configuration, and 75 means
another collection holds the lock. systemd treats 75 as a successful skip.
Diagnostics and a summary go to standard error and the service journal.
A rollup failure also exits 1, preserves minute source directories, and still
attempts index/state updates and tail repair. The first collection after a
failure is resolved may take longer while it catches up. Retention uses the
newest valid directory names, not the current clock.

## Clock recovery

When wall time is not later than the previous collection, the collector logs
`clock_backward`, exits with code 1, and does not append or advance state. After
one accidental jump far into the future, this can persist until real time
catches up. Correct the host clock first. To restart the baseline earlier,
stop the timer and any active collection, then remove the private state:

```sh
sudo systemctl stop hostdiary-collect.timer hostdiary-collect.service
sudo rm /var/lib/hostdiary/state.json
sudo /usr/local/bin/hostdiary collect
sudo systemctl start hostdiary-collect.timer
```

Removing state resets rates to null for one collection and **also loses the boot
history**. Subsequent `boots` contains the current boot even when it is older
than 1095 days, plus past boots observed within the last 1095 days after the reset
(cutoff included). This window is independent of the three retained daily year files.
Boots already lost under a previous retention window are not recovered.
Boot information requires readable uptime and boot identity. Missing older boots are
not evidence that no reboot occurred. Existing public history is kept. Retention
keeps the newest eight date directories, rather than comparing them with the
clock; a future-dated directory can still displace the oldest retained day.

## Development and licenses

```sh
go test ./...
go vet ./...
node --test internal/web/app_test.cjs
```

Node is optional and used only to test the viewer's pure data-preparation
functions. It is not needed to build or run hostdiary.

hostdiary is released under the [MIT License](LICENSE).

The viewer bundles **uPlot 1.6.32**, copyright (c) 2022 Leon Sorokin, under the
MIT License. See [the bundled license](internal/web/assets/vendor/uplot-1.6.32/LICENSE),
also published as `assets/vendor/uplot-1.6.32/LICENSE` alongside the viewer.
