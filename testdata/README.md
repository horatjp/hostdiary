# Synthetic Linux fixtures

These files are hand-written in Linux procfs format and contain no captured host data.
The filesystem mounts are excluded by the fixture configuration so the collection
test works on macOS too. Linux statfs is exercised separately on a temporary directory.
