---
name: cron-jobs
description: Help set up and manage cron-based jobs for recurring tasks.
metadata:
  short-description: Cron jobs
---

# cron-jobs

Purpose: help schedule recurring jobs inside the container using cron.

## Workflow
1) Ensure cron is installed and running under vrctl.
2) Choose either `/etc/cron.d/` (system-wide) or user crontab.
3) Add the schedule with explicit PATH and shell.
4) Verify execution via logs.
5) Mention that vrctl keeps cron running on container restarts.

## Install + start cron
```
apt-get update
apt-get install -y cron
vrctl service add cron --cmd "cron -f"
```

## /etc/cron.d/ example
```
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# m h dom mon dow user command
*/5 * * * * root /usr/bin/env bash -lc 'cd /workspace/<app> && ./scripts/job.sh' >> /var/log/<app>-cron.log 2>&1
```

## Verify
- `vrctl service status cron`
- `tail -n 200 /var/log/<app>-cron.log`
