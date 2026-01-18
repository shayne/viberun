# Future Plans

## Container hardening

- Add `--security-opt=no-new-privileges`.
- Add `--cap-drop=ALL` (then selectively add only if needed).
- Add resource caps: `--pids-limit`, `--memory`, `--cpus` (sane defaults).
- Add `--read-only` (with tmpfs for `/tmp`, `/run`, `/var/tmp`).
