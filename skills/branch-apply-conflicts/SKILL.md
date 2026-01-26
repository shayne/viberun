---
name: branch-apply-conflicts
description: Resolve merge conflicts when applying a viberun branch environment back to its base app. Use when `apply` or `branch apply` fails with conflicts.
---

# Branch Apply Conflict Resolution

Use this skill when `apply` or `branch apply` fails with merge conflicts.

## Quick workflow

1) Stay in the branch container (the one you were applying from). Conflicts are written into `/home/viberun/app`.
2) Find conflict markers:

```
rg -n "<<<<<<<|=======|>>>>>>>" /home/viberun/app
```

3) Resolve each file by choosing/combining changes. Do not edit `/home/viberun/data`.
4) Run the app’s tests or checks if they exist.
5) Re-run `apply` (or `vrctl host branch apply <branch>` if you are in the base container).
6) If apply still fails, repeat; `apply` rebuilds the merge from current `app/` contents each run.

## Notes

- `data/` changes are not promoted. Convert data changes to migrations inside `app/`.
- If you need context, compare files between the base and branch containers, but treat the branch `app/` as the source of truth for conflict edits.
