# Shell Setup Command Plan

## Goal
Provide an in-shell `setup` command that guides first-time users, runs bootstrap with TTY support (including SSH host key prompts), and returns them to a ready shell with clear next steps.

## Step 1: Detect setup-needed state and show banner
- Update the shell banner to show a setup message when no host is configured, or when a configured host is reachable but not bootstrapped.
- Stop auto-running bootstrap when a host is not bootstrapped.

Acceptance criteria:
- Starting the shell with no configured host shows a banner: "Welcome to viberun. Type setup to get started."
- Starting the shell with a configured but unbootstrapped host does not auto-run bootstrap and shows a setup hint.

## Step 2: Add `setup` to the shell command registry and help
- Add a global `setup` command with usage, description, and examples.
- `help setup` prints the setup overview and what information is needed.

Acceptance criteria:
- `help` lists `setup` in the global commands list.
- `help setup` describes the flow and host format (e.g., `user@host` or IP).

## Step 3: Prompt for host with a dedicated setup input
- When `setup` runs, print friendly guidance about needing a server (DigitalOcean, Hetzner, home lab, etc.).
- Use a Huh input prompt with a placeholder (e.g., `root@1.2.3.4`) and the existing host as a default.
- Save the host into config and move into bootstrap.

Acceptance criteria:
- `setup` prints the guidance text and then shows a Huh prompt for the host.
- If no default host exists, a blank entry is rejected with a clear error.
- If a default host exists, pressing Enter accepts it.

## Step 4: Run bootstrap with TTY support and return to shell
- Run the existing bootstrap steps from inside the shell (not by calling an external `viberun bootstrap` command).
- Ensure SSH prompts (host key verification, sudo password) work via the TTY.
- On success, sync the host/apps list and show a "create your first app" hint.

Acceptance criteria:
- The bootstrap flow runs with interactive SSH prompts when needed.
- On success, the shell returns and shows: "Create your first app: run <name>" (or similar).
- The shell updates host/app state without requiring a restart.
