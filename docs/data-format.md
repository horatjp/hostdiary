# Public data format — schema 1

This document defines the public files produced by the current implementation. It covers one-minute values for the basic metrics, coarse values and monthly traffic estimates.
It also covers daily directory usage lists.
JSON strings and column descriptions are English. The viewer switches between Japanese and English according to the browser language (English for languages other than Japanese).

## Layout and versions

The public root is `/var/lib/hostdiary/public/`. Paths below are relative to this directory.

| Path | Contents |
|---|---|
| `index.json` | Information about the collection that last wrote the index, and lists of groups and daily files |
| `index.js` | The same index passed to `HD.index(...)` |
| `data/1m/YYYY-MM-DD/<group>.jsonl` | Headers and data rows per group and day |
| `data/1m/YYYY-MM-DD/<group>.js` | The same rows passed to `HD.l(...)` |
| `data/5m/YYYY-MM/<group>.jsonl` and `.js` | Five-minute values (monthly files, newest 3 months) |
| `data/1h/YYYY-MM/<group>.jsonl` and `.js` | Hourly values (monthly files, newest 25 months) |
| `data/1d/YYYY/<group>.jsonl` and `.js` | Daily values (yearly files, newest 3 years) |
| `data/dirs/YYYY-MM-DD.json` and `.js` | Top 20 directories by usage per filesystem (UTC date) |
| `summary.json` | Summary for each day with retained one-minute values (rows, gaps, and per-column minimum, maximum and mean). See "Summary" below |
| `llms.txt` | Reading guide for AI (fixed English Markdown). See "Reading guide for AI" below |
| `index.html`, `assets/` | Offline-capable viewer, bundled uPlot and LICENSE |
| `assets/VERSION` | Executable version + `+` + first 8 hexadecimal digits of the SHA-256 of all embedded viewer assets |

`schema: 1` is the public data format version, included in the index and daily file headers.
The index `version` is the executable version (currently `0.3.0`), distinct from the viewer asset hash.
Increment the schema when changing existing columns (meaning, units, removal or order), ID rules or file layout.
Adding columns does not increment the schema because headers identify them. Improvements to descriptions do not require a version increment either.
The viewer is rewritten when its version or hash changes, or an embedded public file is missing.
State, locks and temporary files live outside the public root. Public files use mode 0644 and public directories use 0755.

## Directory usage

`hostdiary dirs` measures once daily using the same filesystem selection and disk filters as collect. It does not invoke an external du.
`data/dirs/YYYY-MM-DD.json` uses the UTC date at execution start; another run on the same day replaces it. Example (fictional values):

```json
{
  "schema": 1, "kind": "dirs", "started_at": 1793500200, "finished_at": 1793500212,
  "depth": 2, "top": 20,
  "filesystems": [{
    "mount": "/", "total": 8192, "complete": true, "errors": 0,
    "entries": [{"path": "/var", "depth": 1, "bytes": 4096}]
  }]
}
```

- Sizes are `st_blocks × 512` bytes. Directories themselves are counted. Symlinks are not followed and traversal does not cross devices.
  Non-directory entries with nlink>1 are counted only once per inode. If linked from multiple paths, their usage is assigned to ancestors of the first path encountered during traversal.
- `entries` combines directories at depth 1 and 2 from the mount point, keeping at most 20 in descending size order. Ties use ascending public path order.
  Parent and child values overlap; do not add them. Usage at depth 3 and deeper is included in shallow ancestors.
- `total` is the traversal total for the whole mount. Files directly below the mount contribute only to total. It differs from the disk group's used
  (files still open after deletion, reserved blocks, files hidden by mounts, etc.).
- `errors` counts skipped entries. Child ENOENT and ENOTDIR alone leave complete=true. Other read failures set complete=false.
  Failure to Lstat the mount point sets complete=false and errors=1 even for a disappearance error. Partial results are saved, with exit code 1.
  When complete=false, total and entries are lower bounds; a filesystem whose mount point itself could not be read has total 0 and no entries.
- `path` and `mount` are absolute paths. Non-UTF-8 bytes, bytes of non-printable characters and backslashes are replaced with lowercase hexadecimal `\xHH`.
  JSON also escapes the backslash itself. Printable UTF-8 is preserved, making this representation reversible.
- A path absent from a day's list may be outside the top 20 or may not have existed; do not treat its absence as zero.

The `.js` file is one line: `HD.dirs("YYYY-MM-DD", <same JSON>);`. Temporary files are created in the private state directory,
then renamed in `.js` → `.json` order. If only JSON replacement fails during a same-day rerun, new JS and old JSON can coexist (accepted).
The viewer reads JS over both HTTP and file://, comparing the latest day with the same path 7 days earlier. If that day is absent, it uses the oldest day.
With only one day, it does not compare the day with itself: comparison date, value and difference are "—", with "no earlier day to compare" below the heading.
An absent comparison filesystem or path is "—". Partial status for both days is shown; differences are calculated only for display. No graphs are drawn.

Retention keeps the newest 31 JSON files with valid date names and removes older JSON/JS files. The day written in the current run is excluded from the deletion cutoff,
so many future dates can leave up to 32 files. There is no comparison with the clock. Invalid names and directories are untouched. Deletion order is JSON → JS.
With `dirs.enabled=false`, no measurement is made; the next dirs run removes all JSON/JS files with valid date names only.
If no filesystem is selected, it prints `no filesystem selected` to stderr and exits 0 without replacing existing files.

The index adds the following top-level `dirs` only when date-named JSON files exist. Collect builds it from filenames alone, without reading daily file contents.

```json
{"dirs":{"days":["2026-10-25","2026-11-01"],"path":"data/dirs/{day}.json","description":"..."}}
```

days is ascending. description explains the measurement method, parent/child overlap, depth origin, directly contained files, absence versus zero, total versus disk used, errors/complete and path escaping in English.
Dirs does not write the index, so additions or deletions due to disabling appear in the index at the next collect.
Until then, or in a race with deletion, failure to read a file referenced by the index is treated as missing data.

Known limits: `MemoryMax=256M`, `GOMEMLIMIT=200MiB` and a 30-minute deadline apply. More than about 2 million entries in one directory or
millions of hard links can stop dirs and leave that day missing (an earlier successful result for the same day is retained). Check the service journal for the cause.
An unresponsive network/FUSE mount below a selected filesystem can stall traversal until the 30-minute timeout, leaving that day missing.
Traversal is not a snapshot. Stderr details are limited to the first 100 errors per Run, followed by the total count. JSON errors are not truncated.

## Group IDs

The single global groups `cpu`, `load`, `mem`, `swap`, `procs`, `tcp` and `psi` use their kind as the ID.
Per-target `net`, `disk`, `diskio` and `app` use this format:

```text
<kind>-<slug>-<hash8>
```

The slug replaces each character of the original name other than ASCII lowercase a–z and digits 0–9 with `_`, then takes the first 32 characters.
Uppercase is not converted to lowercase. hash8 is the first 8 hexadecimal digits (lowercase) of the SHA-256 of the original name's UTF-8 bytes.
Example: the net group for `eth0` is `net-eth0-9c32a8c7`.
The original name is saved as `name` in headers and the index. A disk name is a mount point; an app name is its configured display name.

## Daily files

The date is `YYYY-MM-DD` obtained by converting the data row's `time` to **UTC**.
Neither the local nor browser time zone determines file placement.
Each collection appends one data array row to each target group's daily file. No fixed-interval empty or missing rows are inserted.

### JSONL

Example for `data/1m/2026-10-31/net-eth0-9c32a8c7.jsonl`:

```jsonl
{"schema":1,"group":"net-eth0-9c32a8c7","kind":"net","name":"eth0","step":60,"columns":["time","rx","tx"],"units":["s","bytes/s","bytes/s"]}
[1793404800,null,null]
[1793404860,10,20]
```

| Header field | Meaning |
|---|---|
| `schema` | Public format version; currently integer 1 |
| `group` | Group ID for this file |
| `kind` | Group kind |
| `name` | Original target name; kind name for global groups |
| `step` | Expected collection interval, 60 seconds; not a guarantee of actual row spacing |
| `columns` | Column names at each array position; starts with `time` |
| `units` | Units at the corresponding positions; starts with `s` |

A header is written at the beginning. On append, a new header is written if its contents differ from the preceding header.
Comparison includes schema, group, kind, name, step and units, as well as columns.
Object rows define the meaning of subsequent array rows; they are headers, not data rows.

Data rows have the same length as `columns`. `time` is wall-clock Unix seconds (integer) obtained at collection start.
Other elements are numbers or `null`. Floating-point rates, percentages and load values are rounded to at most 3 decimal places.
Integer bytes and counts are not converted to KiB/MiB or K/M for storage.

### JavaScript mirrors

The same rows in `data/1m/2026-10-31/net-eth0-9c32a8c7.js` take this form:

```javascript
HD.l("1m/2026-10-31/net-eth0-9c32a8c7",{"schema":1,"group":"net-eth0-9c32a8c7","kind":"net","name":"eth0","step":60,"columns":["time","rx","tx"],"units":["s","bytes/s","bytes/s"]});
HD.l("1m/2026-10-31/net-eth0-9c32a8c7",[1793404800,null,null]);
HD.l("1m/2026-10-31/net-eth0-9c32a8c7",[1793404860,10,20]);
```

The first argument is the path without `data/` or the extension; the second is the same object or array as in JSONL.
The viewer defines `HD.l` before loading script elements. HTTP and file URLs use the same path.

### Appending and recovering partial lines

Each line ends with a newline. One-minute rows are appended in one O_APPEND write per line. Coarse rows for one group, resolution and day are appended in one write per mirror. A collection lock limits writers to one.
Before appending, bytes after the final newline are truncated. A file with no newline is truncated to length 0.
During collection, retained older days and jsonl/js files for groups no longer being appended are repaired by the same rule.
On a detected append error, both mirrors are attempted to be restored to their repaired, pre-append lengths.
Even if appending one group fails, later groups, the index and state continue to be saved.

## Coarse values (5 minutes, 1 hour, 1 day)

After appending one-minute values, collect creates coarse values directly from one-minute values for finished UTC days, in 5m → 1h → 1d order, before deleting one-minute values.
Today and future days are excluded. Days whose one-minute values are already gone are not recovered. The 1h/1d values are not calculated from 5m.

Headers have the same shape as one-minute headers, with step 300, 3600 and 86400 respectively. Columns and units follow this order:

```text
columns: [time, samples, last_time, c1, c1_max, c2, c2_max, ...]
units:   [s,    count,   s,         u1, u1,     u2, u2,     ...]
```

- time is the **interval start**, aligned to step in UTC; it differs in meaning from a one-minute collection time.
- samples counts rows with distinct time values in the interval. A normally full interval has 5, 60 or 1440, but more are possible with sub-minute rows.
  Rows with all columns null count too; it is not the per-column non-null count.
- last_time is the actual last observation in the interval (maximum time), at least the interval start and less than start+step.
- c is the mean of non-null values (at most 3 decimal places), and c_max is their maximum. Both are null if all values are null. Intervals without rows are omitted.
  Maxima can come from different times per column; do not add CPU column maxima or compare used_max+avail_max.
- Columns come from the day's last valid header; each row is mapped using the column names in its own header. Absent columns are null.
  Later rows win for duplicate time values; rows outside the daily file's date range are discarded. A broken JSON line alone is discarded.
  Rows under unsupported headers are ignored; a day without readable rows succeeds as "nothing to create". I/O errors are failures.

Mirror call example: `HD.l("5m/2026-10/cpu", [1793404800,5,1793405040,12.5,20]);`.
The first argument is resolution/month-or-year/group, without `data/` or the extension.
Only intervals after the last data row for each resolution are appended, preventing duplicates on rerun.
Daily values mark completion and are written only after 5m/1h succeed. After a group fails, later days for that group are not written in that run.
If a daily directory cannot be read and its groups cannot be identified, processing stops from that day onward and returns an error.

## Groups, columns and units

When adding columns, do not use `samples`, `last_time` or the suffix `_max` for one-minute column names; they are reserved for coarse values.

Every group starts with `time`, in `s`. The table lists the remaining columns in storage order.

| kind | Target | Columns (excluding time) | Units (same order; grouped when shared) |
|---|---|---|---|
| `cpu` | All CPUs combined | user, nice, system, iowait, irq, softirq, steal, idle | All `%` |
| `load` | Whole host | load1, load5, load15, cpus | `(none)`, `(none)`, `(none)`, `count` |
| `mem` | Whole host | total, available, used, buffers, cached, oom_kills | First 5 `bytes`; oom_kills is `count` |
| `swap` | Whole host | total, used, in, out | `bytes`, `bytes`, `bytes/s`, `bytes/s` |
| `procs` | All read PIDs | total, running, sleeping, disk_sleep, stopped, zombie, other | All `count` |
| `tcp` | TCP v4+v6 combined | established, syn_sent, syn_recv, fin_wait1, fin_wait2, time_wait, close, close_wait, last_ack, listen, closing, other, out_segs, retrans_segs | First 12 `count`; last 2 `count/s` |
| `net` | Per NIC | rx, tx, rx_errors, rx_drops, tx_errors, tx_drops | First 2 `bytes/s`; last 4 `count/s` |
| `disk` | Per filesystem | used, avail, inodes_used, inodes_total | `bytes`, `bytes`, `count`, `count` |
| `diskio` | Per device | read, write, read_ops, write_ops, busy, read_time, write_time | `bytes/s`, `bytes/s`, `count/s`, `count/s`, `%`, `ms/s`, `ms/s` |
| `app` | Per configured display name | count, rss, started | `count`, `bytes`, `count` |
| `psi` | Whole host | cpu_some, memory_some, memory_full, io_some, io_full | All `%` |

The schema 1 unit vocabulary is `s`, `%`, `bytes`, `bytes/s`, `count`, `count/s`, `ms/s` and `(none)`.
Display-only formatting as B/KiB/MiB, or converting net to bit/s, does not affect storage.
The viewer omits unit suffixes for `(none)`, `count` and `count/s`; count axis labels follow the language.
Byte axes use powers-of-two increments.

- load.cpus counts numbered cpu lines in `/proc/stat`. mem converts kB in `/proc/meminfo` to bytes by multiplying by 1024.
  mem.used = total − available; swap.used = SwapTotal − SwapFree. Missing required values or negative differences yield null.
- procs maps R to running, S to sleeping, D to disk_sleep, T/t to stopped, Z to zombie and everything else to other.
  other includes the kernel idle state. Disappeared PIDs are excluded; other PID read failures make procs/app null instead of publishing partial totals.
- app matches comm exactly (aliases are OR). Zombies are excluded; rss sums stat field 24 × the runtime page size.
  Aliases are at most 15 bytes. If matching rules change, change the display name too, to avoid mixing different definitions under one ID.
- disk.used = (blocks − bfree) × frsize; avail = bavail × frsize.
  If frsize is 0, bsize is used. If the inode total is 0, inodes_used/inodes_total are both null.
- diskio.read/write multiply cumulative sector differences by 512 and convert to per-second rates.
  read_ops/write_ops are completion rates; busy is the I/O time difference (milliseconds) ÷ elapsed seconds ÷ 10.
- tcp maps state numbers 01–0B in order and counts others as other.
- psi divides differences of total (cumulative µs) in `/proc/pressure/{cpu,memory,io}` by `dt × 10⁶`, then multiplies by 100.
  some is time when at least one task waits for the resource; full is time when all non-idle tasks wait. CPU full and avg10/avg60/avg300 are not stored. Values above 100% are not clamped.
- diskio.read_time/write_time divide differences of cumulative diskstats read/write milliseconds by dt (ms/s). Concurrent I/O durations are added, so values can exceed 1000 ms/s.
- swap.in/out are pswpin/pswpout differences from `/proc/vmstat` ÷ dt × the runtime page size (bytes/s).
- mem.oom_kills is the difference from the previous vmstat oom_kill (count). It includes kills due to cgroup memory limits, not only host memory exhaustion. Compare mem.available too.
- app.started counts matching non-zombies with `round(previous uptime × 100) < starttime ≤ round(current uptime × 100)`.
  starttime is PID stat field 22. **USER_HZ=100 is assumed**. PID sets are not stored.
- tcp.out_segs/retrans_segs are OutSegs/RetransSegs rates from `/proc/net/snmp` (IPv4+IPv6). OutSegs includes pure ACKs and excludes retransmissions.
- net errors/drops are rates of rx/tx counters in `/proc/net/dev`. They depend on the driver and can always be 0 on virtual NICs.
  rx_drops also includes discarded frames without a protocol handler; it alone cannot identify causes or end-to-end packet loss.

Ratios are not stored; the viewer calculates them from materials in the same row. Mean completion time (ms) is read_time/read_ops or write_time/write_ops, not application response time.
Retransmission percentage is retrans_segs/out_segs×100, not a packet loss rate. It is drawn only when both operands are finite numbers and the denominator is positive; otherwise it is null.
Derived series (`read_completion`, `write_completion`, `retrans_ratio`) are viewer-only names, not added to stored or indexed columns.
Disk I/O uses separate graphs for transfer rates, operation rates, utilization and mean completion time; read_time/write_time are available as latest values in "Column descriptions".
TCP uses separate graphs for stacked connection states, sent/retransmitted segments and retransmission percentage.
Mean completion time and retransmission percentage panels appear only when at least one derived value is non-null (calculated 0 is included).

For coarse values, ratios divide means, never maxima. This approximates count weighting when collection intervals are nearly constant and both materials have identical null patterns.
It can differ when collection is skipped or missing patterns differ, such as the first half of a day when columns were added.
One-minute material rates and coarse means are rounded to 3 decimal places, making ratios imprecise for very small operands.
If either mean is 0 but its corresponding `_max` is positive, rounding erased the value and the ratio is not drawn.
The viewer shows oom_kills mean and maximum in a separate count panel without converting them to a total.
For AI readers, oom_kills/started mean×samples is an approximate interval total. It may not be exact because per-column non-null counts are not stored and means are rounded.
A positive OOM maximum means kills occurred even if the mean is 0. Do not treat an absent panel due to old columns or invalid units as proof of zero events.

Diskio excludes loop*, ram*, sr*, nbd* and zram* by default. Explicitly including these returns them to collection.
See [README](../README.md#configuration) for target include/exclude, safe filesystem selection and other default exclusions.

## Index

Example `index.json` (fictional illustrative data):

```json
{
  "schema": 1,
  "version": "0.1.0",
  "host": "fixture-host",
  "generated_at": 1793404860,
  "boots": [1793403800],
  "unavailable": [],
  "groups": [{
    "id": "net-eth0-9c32a8c7",
    "kind": "net",
    "name": "eth0",
    "columns": ["time", "rx", "tx"],
    "units": ["s", "bytes/s", "bytes/s"],
    "descriptions": ["collection time (Unix seconds)", "received bytes per second", "transmitted bytes per second"],
    "last_seen": 1793404860,
    "days": ["2026-10-31"]
  }]
}
```

| Index field | Meaning |
|---|---|
| `schema` | Public format version; 1 |
| `version` | Executable version, without the viewer asset hash |
| `host` | Configured hostname; OS hostname if the setting is empty |
| `generated_at` | time of the collection that wrote this index, i.e. last collection time; distinct from file mtime |
| `boots` | Estimated Unix seconds of the current boot and past boots within the last 1095 days (about 3 years, including the cutoff). The last entry is the last recorded boot |
| `unavailable` | Unavailable information for this run's read/calculation sources, not historical records |
| `groups` | Groups with retained one-minute or coarse values, ordered by ID; an empty array if none |
| `guide` | AI reading guide path, always `"llms.txt"` |
| `summary` | Summary path, always `"summary.json"` |

Group id/kind/name/columns/units correspond to daily file headers.
`descriptions` contains English explanations per column, including time.
`last_seen` is the largest time read from retained JSONL data rows, including all-null rows; it does not mean all metrics were available then.
`days` lists ascending UTC day directory names where index generation read data rows for the group.
Each index id and day constructs `data/1m/{day}/{id}.jsonl` (its mirror uses `.js` at the same path).
The index is rebuilt by scanning retained one-minute JSONL and adding coarse file listings (stat). Groups absent from the current observation remain while history exists.
The viewer shows `last_seen < generated_at` as stopped.

Boot estimates subtract the integer part of uptime from collection time. An estimate is added when boot_id changes or no boot history exists.
The current boot is kept even if older than 1095 days; the window `b >= at - 1095*86400` applies only to past boots.
Daily values use the newest 3 year files, not the same period as the last 1095 days (about 3 years) of boots.
With the same boot_id, the previously recorded current boot time is retained. The array is not sorted by timestamp, nor is its last entry replaced by the maximum.
If uptime cannot be read when boot_id changes, or boot_id cannot be read, the previous boot can remain last.
The last entry is the "Last recorded boot (estimated)", with no guarantee of matching the current boot. This past entry can also remain for up to 1095 days.
If boot information is unavailable and previous information is unusable, the array can be empty. Unobserved reboots are never reconstructed.
Missing, unreadable or corrupt state discards previous values and boot history, restarting from the current observation.
Absence from boots is not proof that no reboot occurred. Extending retention does not restore history already deleted.
If collection time is no later than the previous time, it exits 1 with clock_backward as before and does not advance history. Clock correction does not recalculate estimates for the same boot_id.

`index.js` is one line passing the index JSON unchanged as `HD.index(<JSON>);`.
Each index file is replaced by renaming a temporary file from a private location; the pair is not switched atomically.
Even an updated `generated_at` can accompany failed appends for some groups. Check each last_seen and the logs too.

### Coarse index fields

The following top-level fields are added (schema stays 1 because these are additions only).

```json
"resolutions": [
  {"name":"1m","step":60,"path":"data/1m/{day}/{id}.jsonl","keep":"at least 8 newest UTC days (more while rollup is failing)"},
  {"name":"5m","step":300,"path":"data/5m/{month}/{id}.jsonl","keep":"3 newest UTC months"},
  {"name":"1h","step":3600,"path":"data/1h/{month}/{id}.jsonl","keep":"25 newest UTC months"},
  {"name":"1d","step":86400,"path":"data/1d/{year}/{id}.jsonl","keep":"3 newest UTC years"}
],
"coarse_columns": "time is the UTC-aligned interval start; samples is the number of 1-minute rows in the interval (rows with all values null included); last_time is the latest 1-minute time in the interval; then for each 1m column c: c (average of non-null 1-minute values, rounded to 3 decimals) and c_max (maximum 1-minute value). Maxima of different columns may come from different minutes; do not add them. Ratios use means, never maxima; do not compute a ratio when either material mean is zero and its corresponding maximum is positive (rounding)."
```

Each group's coarse is, for example, `{"5m":["2026-09","2026-10"],"1h":["2026-10"],"1d":["2026"]}`.
Months/years with nonempty JSONL are listed using stat alone. Each array is ascending, or `[]` if absent.
Groups with only coarse values are also listed; days is `[]` and last_seen is last_time from the latest coarse file's final data row.
Columns/units are restored by removing samples, last_time and _max columns from the header. descriptions are inherited by column name from the previous index, or use the column name if absent.
A restoration read error produces exit code 1; a previous entry is carried over if present, with days set to `[]` and coarse set to this run's stat results.
Without a previous entry, the group is omitted. Entries whose restored ID differs from the filename ID are rejected.

### Monthly traffic (estimate)

Only net groups add traffic. Example (fictional data):

```json
"traffic": {
  "estimate": true,
  "description": "Traffic estimate: missing time is extrapolated from each day's mean; samples counts 1-minute rows, including null values; compare with the elapsed minutes.",
  "month": {"from":"2026-10-01","rx":123456789,"tx":9876543,"days":31,"days_missing":0,"samples":44100},
  "last30d": {"from":"2026-10-02","rx":123456789,"tx":9876543,"days":30,"days_missing":2,"samples":40210}
}
```

- month runs from the UTC month start through today; last30d covers 30 UTC days including today. Both end at generated_at.
- Past days use the daily mean rate per direction ×86400; today uses the non-null mean of one-minute values × elapsed seconds today. The sum is rounded to integer bytes.
  For today's duplicate timestamps, later rows win; rows after generated_at are excluded. Negative mean rates are treated as 0.
- This is an **estimate** extrapolating missing time from each day's mean. A day without daily/today data, or with either direction entirely null, adds 1 to days_missing.
  Even if one direction is null, the valid direction's amount is added. days is the number of target days; today's remaining time is excluded from the amount.
- samples sums daily samples for past days and distinct one-minute timestamps today. Nulls count too; it is neither covered time nor per-column valid counts.
  Sparse observations can be extrapolated to a whole day; compare with elapsed minutes.
- Only target daily year files (at most 2) and today's one-minute values are read. The viewer uses binary increments such as GiB, distinct from providers' decimal GB.

The viewer uses one-minute values for 1 hour/1 day/1 week, 5-minute values for 1 month (31 days), hourly values for 1 year (365 days) and daily values for 2 years (730 days).
Graphs primarily use stepped fills (align -1), split into panels by kind and unit (only load.cpus shares the load axis).
Stacked and line y axes start at 0 (CPU fixed at 0–100); panels showing receive/read above and transmit/write below use symmetric ranges around 0.
Memory stacks used and available; buffers, cached and total appear only in the numeric table.
A separate OOM kill count panel and numeric table follow. One-minute values count events since the previous collection; coarse values are means and maxima.
The panel is omitted if the index lacks the column or its unit is not count; old rows without it are null. 0 is a value; all-null means no observations and "—".
Coarse graphs draw interval means; only line panels overlay faint maximum lines. Stacked and symmetric panels do not overlay maxima.
samples/last_time are not plotted, and uPlot legends are hidden.
A table of minimum, mean, maximum, latest and cursor values is always shown below each graph; latest values also remain in the "Column descriptions" disclosure.
Statistics cover the selected period and do not follow zoom. Nulls are excluded; latest is the last non-null value. No values gives "—"; a single value is shown as itself.
Coarse means are weighted by samples; maxima use `_max` (the maximum ratio for derived series), while minima/latest use interval means.
Coarse rows with nonpositive, non-finite or null samples are excluded from plotting and statistics.
Net rx/tx also show selected-period totals from bit to Pbit (one-minute: bit/s × 60; coarse: bit/s × samples × 60).
This approximate total does not correct missing observations or collection intervals other than 60 seconds; it differs from the interpolated monthly/30-day traffic estimates above.
The viewer x is the interval end (time for one-minute values, time + step for coarse values). Window selection, point headings and cursor timestamps use the original time.
The viewer explains that today is excluded, and timestamps for points in the 2-year view use a UTC date, such as `2026-10-30 (UTC day)` (with a Japanese equivalent in Japanese).
Daily axes retain interval-end dates. Local times use a four-digit year and 24-hour format.
The top table of contents links to time-series groups in heading order, including stopped and all-missing groups. Dirs remains at the end.
The header's "Last recorded boot (estimated)" (localized in Japanese) displays the last boots entry in the browser time zone to seconds.
An absent or invalid last entry is unavailable. It is not rounded to a UTC day even in the 2-year view.
Dashed reboot lines on all graphs use actual boots timestamps without adding coarse step. Both selected-period endpoints are included; lines are drawn even during gaps or entirely missing data.
They follow zoom, reset and resize without changing y axes, measurement series or table statistics. Lines outside the period are omitted; no bands or normal/abnormal judgements are shown.
If an empty plot cannot be created, that panel falls back to the previous no-observations display and logs the reason to the console; other panels remain.

### unavailable

Each element is `{"group": ..., "column": ..., "reason": ...}`.

| Field | Meaning |
|---|---|
| `group` | Target group ID, or a kind name for a failure affecting an entire source |
| `column` | Target column name; null if not restricted to a column |
| `reason` | `unsupported` (absent in the environment), `not_found` (included target absent), `error` (read/parse failure) |

No fictional named ID is created for an entire source failure. For example, a `/proc/net/dev` failure uses group `net`.
A kind name applies to all groups of that kind, including groups not yet present in the index.
If uptime or boot_id cannot be read, one entry each is emitted for cpu, net and diskio. Identical group/column/reason entries are deduplicated.
Specific sources and detailed errors go to stderr; the index contains only these 3 fields.
Final output sorts by group, column and reason, with column=null first.
If a group has a column=null entry, its column-specific entries are omitted.

For example, a filesystem without inodes emits unsupported for inodes_used and inodes_total of its disk ID.
If only one of `/proc/net/tcp` and `/proc/net/tcp6` is absent, no report is emitted and the readable side's aggregate is used.
Only when both are absent is one tcp column=null unsupported entry emitted, with all tcp values null.
Read/parse failures are error, distinct from absence; failure of either side makes all tcp values null.
If TCP states are unavailable, all tcp columns including the 2 SNMP columns are null. If only SNMP is unavailable, only out_segs/retrans_segs are reported.
Vmstat is assessed per key; for example, missing oom_kill makes only mem.oom_kills unsupported.
PSI reports unsupported for the entire psi group if the pressure directory is absent, or for affected columns if an individual file is absent.
PSI EOPNOTSUPP/ENOTSUP read failures are also unsupported and do not cause a failing exit code. Other read/parse failures are error.
Not every reason for null is listed in unavailable, such as initial rates or invalid time intervals.
Write failures are also excluded from unavailable; details appear in logs and the exit code.

## null, time and rates

`null` means unavailable or incalculable; never replace it with 0. Numeric 0 is a measured/calculated zero.
**No row = no collection data for that timestamp**. This includes stopped/skipped collection and save failures; do not fill with zero or constant values.
Step is the expected interval, not proof of successful collection at an identical phase each minute.

Net/diskio, psi, swap.in/out and tcp segment rates use `dt`, the difference from the previous uptime saved in private state.
With the same boot_id, valid previous/current values and uptime, `0 < dt <= 180` and nondecreasing counters, the rate is `(current − previous) / dt`.
Otherwise it is null. Wall-clock differences are not the denominator.
Absent previous values differ from zero. On the first collection after an upgrade, only new cumulative-derived columns are null. Previous values in existing Net/DiskIO positions are retained.
Started can count immediately if old state has uptime for the same boot. Counters that lost previous values due to read failure remain null on the first recovery collection.

The count columns oom_kills/started do not impose the 180-second dt limit; with the same nonempty boot_id and previous values, they count across long intervals.
Oom_kills requires previous/current counters to exist and not decrease; it does not depend on valid uptime.
Started additionally requires valid previous/current uptime and an increase in rounded centiseconds.

CPU first checks previous values, boot_id and dt validity too. Then negative per-column differences are treated as 0,
and `column difference / sum of differences of 8 columns × 100` is calculated. If the sum is 0, all columns are null.
Guest is included in user, so it is not added separately. CPU percentages are not divided by uptime seconds.

If collection wall-clock time is no later than the last collection time in state, clock_backward is written to stderr and it exits 1.
No data row is appended; state time and the index do not advance.
See [README](../README.md#clock-recovery) for correcting the clock, resetting state and the warning about losing boots.

## Summary

`summary.json` is rewritten after the index at each collection (rename from a private temporary file). Example (fictional data):

```json
{
  "schema": 1, "version": "0.1.0", "host": "fixture-host", "generated_at": 1794096060,
  "guide": "llms.txt",
  "days": [{
    "day": "2026-11-08", "status": "partial", "expected_minutes": 2,
    "groups": [{
      "id": "mem", "kind": "mem", "name": "mem",
      "rows": 2, "first": 1794096000, "last": 1794096060,
      "missing_minutes": 0, "gaps": [], "gaps_truncated": false,
      "columns": [
        {"name": "available", "unit": "bytes", "count": 2, "min": 300, "min_time": 1794096060, "max": 500, "max_time": 1794096000, "mean": 400},
        {"name": "oom_kills", "unit": "count", "count": 2, "min": 0, "min_time": 1794096000, "max": 0, "max_time": 1794096000, "mean": 0, "sum": 0}
      ]
    }]
  }]
}
```

| Field | Meaning |
|---|---|
| `schema`, `version`, `host`, `generated_at` | Same as the index. `generated_at` is time of the collection that produced the summary |
| `days` | Existing date-named directories in `data/1m/`, oldest first |
| `status` | `complete` (before generated_at's UTC day), `partial` (same day), `future` (later; written while the clock was ahead) |
| `expected_minutes` | 1440 for complete, `min(1440, floor((generated_at − day start)/60) + 1)` for partial, null for future |
| `groups` | Groups with at least 1 readable data row that day, in ID order |
| `rows` | Number of distinct time values within the day; `first` and `last` are their minimum and maximum |
| `missing_minutes` | Number of minute slots from 0 to expected_minutes−1 (`floor((time − day start)/60)`) without rows; null for future |
| `gaps` | Up to 20 consecutive ranges of empty minute slots, oldest first. `from`/`to` are the starts of the first/last slots; `minutes` is the slot count. `gaps_truncated` is true if a 21st or later range exists. `[]` for future |
| `columns` | Columns from the day's last valid header, in order, excluding time |

Column fields: `unit` is unchanged from the header (per-second values are not converted to per-minute). `count` is the number of non-null values.
`min`/`max` are values; `min_time`/`max_time` are the first time each occurred. `mean` is the mean (rounded to 3 decimal places). All 5 are null when count is 0.
`sum` is added only to event-count columns `mem.oom_kills` and `app.started` (states such as procs, tcp states and inodes are not summed).
One-minute values are read as for coarse values (map each row by column names in the day's last valid header; absent columns are null, later duplicate time wins, and out-of-day rows are discarded).
No thresholds or normal/abnormal judgements are included.

Complete classifies a past UTC day; it does not guarantee no gaps. Check missing_minutes and each column's count/null too.
A day, group or column absent from the summary is not zero. Outside retention, read failure and no collection may be indistinguishable.

Completed days (complete) are cached outside the public root in `summary-cache.json` (0600), reused while daily file sizes and modification times are unchanged.
Today and future days are recalculated every time. A missing or broken cache causes full recalculation (same results).
A summary write or daily file read failure prints `summary:` to stderr and yields exit code 1. Unreadable days/files are skipped and the remainder is written.

## Reading guide for AI

`llms.txt` is English Markdown embedded in the executable, rewritten like viewer assets when the version/content changes (or it is missing).
It contains no environment-specific values (such as hostnames). Contents: reading order (index.json → summary.json → one-minute values → coarse values → dirs), JSONL shape,
common pitfalls (UTC, absent rows/null, `/s` means per second, state counts versus event counts, not adding maxima, oom_kills, rx_drops, boots, partial),
and a suggested inspection order. A "question → where to look" table directs reboot questions to boots, last-night questions to summaries and minute values after converting to UTC, comparisons with last week to matching windows,
and causes to external logs because reasons are not recorded. It explains that complete does not mean gap-free, absent days/groups/columns are not zero,
the 1095-day boot window and current/last-entry limits, and that old generated_at can also mean an old copy.

## Retention and reader rules

Valid YYYY-MM-DD date directories directly under `data/1m/` are sorted newest first; 8 are kept and the rest removed.
When rollup fails, only deletion of one-minute values is skipped; repair and coarse retention still run. More than 8 days can remain during failures.
Coarse retention keeps valid month/year names newest first: 3 for 5m, 25 for 1h and 3 for 1d.
There is no comparison with current time, and other names are untouched. Even after a long collection outage, old days remain if there are at most 8.
One future date can displace the oldest real-data day, but cannot alone delete all history.
Multiple future dates can displace multiple real dates; retaining 8 directories is not a strict guarantee of all history for the past 7 days.

- Discard invalid JSON lines and partial lines. Do not parse an entire JSONL file as one JSON value.
- Object rows are headers. Check supported schema and columns before interpreting subsequent array rows. Headers do not carry across files.
- Do not hard-code column order; use the columns/units applying to each row. Do not interpret unsupported schemas as known columns.
- Select daily file paths using UTC and convert to local time only for display.
- The viewer takes the later row for identical plotted timestamps and sorts by time. For coarse values, after shifting to interval ends, a gap exceeding the previous row's step×1.5 inserts null to break the line (one-minute values keep time unchanged).
  The selected period ends at generated_at. If the last collection time is old, check source freshness and whether the copy is old, as well as whether collection stopped.

## Known limitations

- Observations are instantaneous at one-minute intervals; processes that start and disappear between them cannot be captured. PID scanning is not a simultaneous snapshot of all metrics.
  Procs counts scanned PIDs, not all threads.
- RSS counts shared memory repeatedly per process. Apps configured with the same comm also each count the same PID.
  Do not treat summed RSS as exclusive host-wide memory usage.
- Mem buffers/cached are not independent components that sum with used. Do not add all columns and compare with total.
- Diskio.busy is approximate and may slightly exceed 100; it is not clamped to 100. It is not latency or queue length itself.
- Diskstats time counters can wrap at 32 bits; affected rates are null when a counter decreases.
- Started assumes uptime and PID starttime use the same boot clock. Environments treating clocks differently, such as during suspend, may miscount.
  USER_HZ=100 is assumed, and processes starting and ending between collections cannot be counted.
- The sum of disk.used and avail is not a substitute for total excluding reserved areas and similar space. No public column gives filesystem capacity in bytes.
- Missing, unreadable or unparseable state loses rate baselines and boots. Clock corrections affect row times and boot estimates.
  After boot information becomes unavailable, the last boots entry may not represent the current boot. Past boots expire after 1095 days; only the current boot is exempt from age-based expiry.
- Appending both mirrors or replacing both indexes is not one transaction. There is no automatic synchronization of complete-line differences left by forced termination. Coarse mirrors can differ by one day's rows.
- Index generation reads all retained one-minute JSONL every time, and net traffic reads target daily values (up to 2 years), so read volume grows with group count and history.
- Daily files or `data/dirs/` that cannot be opened/scanned during index creation are logged and skipped; the index is updated from other files. Collection returns exit code 1.
- JavaScript mirrors can deliver data through script loading from other sites. For HTTP serving, set CORP same-origin in addition to authentication ([README](../README.md#serve-behind-nginx-authentication)).
- Device names such as dm-N and md do not resolve logical-volume meaning. They can appear alongside underlying devices; do not simply sum diskio values.
- No thresholds, normal/abnormal judgements or notifications are output. Summaries cover only days with retained one-minute values, not coarse values or arbitrary periods.
- Summaries reread all of today's one-minute files every time, increasing collection read volume with the number of groups.
- If rollup keeps failing for more than 3 months and old one-minute values remain, it may repeatedly write and delete coarse values for months outside retention (wasted work, not history corruption).
- If a clock jump into the future rolls up an unfinished day or writes later intervals, remaining intervals cannot be filled after correcting the clock. This appears as low samples or missing data.
