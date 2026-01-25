# Branch Environments Design

Date: 2026-01-25

## Goal

Add branch environments so users can develop features in an isolated clone of a base app, then apply only `app/` changes back to the base (pseudo-prod) app. Branch envs start as a full clone of the base app’s `/home/viberun` (both `app/` and `data/`), but **only code and migrations** in `app/` are applied back to the base app.

## Non-Goals (v1)

- Sync/rebase from base app into an existing branch env.
- Bringing `data/` mutations back into the base app.
- Custom domains for branch envs.

## User Experience

### Create/Enter a branch env

```
vibe <app> --branch <branch>
```

- If the branch env does not exist, it is created as a full clone of the base app’s home volume, then the session attaches to the branch env.
- If it already exists, it behaves like a normal app attach.
- `branch` must be a DNS slug (`a-z0-9-`); invalid values are rejected with guidance.

### Branch management (from app context)

```
app <app>
branch            # help/usage
branch list       # list branch envs for this base app
branch rm <name>  # delete a branch env
branch apply <name>
```

### Apply (from inside branch env)

```
apply
```

- Runs the apply flow inside the branch container.
- On conflicts, the agent resolves in the branch env and re-runs `apply`.

### URLs

If a proxy is configured and the server domain exists, branch envs use:

```
https://<app>--<branch>.<server-domain>
```

If no server domain is configured, `open` falls back to the forwarded localhost URL.

## Architecture Overview

Branch envs are first-class app variants. Internally, a branch env is a derived app name:

```
<app>--<branch>
```

That derived name is used for:
- Docker container name (`viberun-<app>--<branch>`)
- Port allocation/state
- Home volume path (`/var/lib/viberun/apps/<app>--<branch>/home.btrfs`)

Branch envs behave like normal apps, but have metadata that identifies the base app and branch name.

## Storage & Metadata

Each branch env stores metadata at:

```
/var/lib/viberun/apps/<app>--<branch>/branch.json
```

Fields:
- `base_app`
- `branch`
- `created_at`
- `base_snapshot_ref`
- `shadow_repo`

`branch list` scans for `branch.json` and groups by `base_app`.

## Home Volume Clone

Creation clones the base app’s home volume:

1) Create a readonly snapshot of base `@home` (e.g., `branch-base-<timestamp>`).
2) Create the branch env’s Btrfs file and seed it from that snapshot.
3) Start the branch container pointing at the new home volume.

This ensures the branch env has an exact copy of `app/` and `data/` at creation time.

## Shadow Git (viberun-managed)

To avoid interfering with user repos, viberun manages a shadow git repo per base app:

```
/var/lib/viberun/git/<app>.git
```

It is mounted into containers and used for apply operations via `GIT_DIR` and `GIT_WORK_TREE`, so it never touches a user’s `.git` under `app/`.

This repo provides merge/conflict semantics even when the app isn’t a git repo.

## Apply Workflow

Apply is agent-first and runs inside the branch env.

1) **Preflight**: ensure base + branch envs exist and match.
2) **Snapshot base**: always snapshot base app before applying.
3) **Shadow merge**:
   - 3-way merge shadow branch into shadow main using the recorded base snapshot ref.
   - If clean, continue.
   - If conflicts, keep user in branch env, provide conflict context/skill, and retry after fixes.
4) **Apply to base**:
   - Sync merged `app/` from shadow main into base app `/home/viberun/app`.
5) **Post-apply**: v1 no-op (future hooks can guide migrations).

**Data is never applied back.** Any schema changes must be represented in `app/` migrations.

## Agent Integration

- `apply` is designed to run inside the branch container so the agent can resolve conflicts with full context.
- `branch apply <name>` from base app context can attach/switch into the branch env and then run `apply`.
- Branch-only skill can warn about `data/` changes and encourage migrations.

## Error Handling

- Invalid branch slug: error with guidance.
- Base app missing: error.
- Branch env missing for apply: error with suggestion to create.
- Merge conflicts: stay in branch env with agent guidance; require re-run of `apply`.

## Testing

- Unit tests for branch name validation and derived app naming.
- Host-side tests for clone/snapshot metadata creation.
- Apply flow tests for clean merge + conflict path.
- Proxy URL generation tests for branch envs.

## Future Extensions

- `branch sync` to rebase/sync from base app (using stored `base_snapshot_ref`).
- Optional TTL cleanup.
- Per-branch custom domains (explicit set).
