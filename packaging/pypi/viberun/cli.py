import os
import platform
import signal
import subprocess
import sys
from pathlib import Path


def _target_triple() -> str:
    system = sys.platform
    machine = platform.machine().lower()

    if system.startswith("linux"):
        if machine in {"x86_64", "amd64"}:
            return "x86_64-unknown-linux-musl"
        if machine in {"aarch64", "arm64"}:
            return "aarch64-unknown-linux-musl"
    if system == "darwin":
        if machine in {"x86_64", "amd64"}:
            return "x86_64-apple-darwin"
        if machine in {"aarch64", "arm64"}:
            return "aarch64-apple-darwin"
    if system in {"win32", "cygwin", "msys"}:
        if machine in {"x86_64", "amd64"}:
            return "x86_64-pc-windows-msvc"
        if machine in {"aarch64", "arm64"}:
            return "aarch64-pc-windows-msvc"

    raise RuntimeError(f"Unsupported platform: {system} ({machine})")


def _binary_path() -> Path:
    target = _target_triple()
    root = Path(__file__).resolve().parent
    binary_name = "viberun.exe" if os.name == "nt" else "viberun"
    return root / "vendor" / target / "viberun" / binary_name


def _extend_path(env: dict, vendor_root: Path) -> dict:
    path_dir = vendor_root / "path"
    if not path_dir.exists():
        return env
    path_sep = ";" if os.name == "nt" else ":"
    existing = env.get("PATH", "")
    env["PATH"] = f"{path_dir}{path_sep}{existing}" if existing else str(path_dir)
    return env


def main() -> int:
    binary = _binary_path()
    if not binary.exists():
        raise RuntimeError(f"Missing viberun binary at {binary}")

    env = _extend_path(os.environ.copy(), binary.parent.parent)
    env["VIBERUN_MANAGED_BY_PIP"] = "1"

    proc = subprocess.Popen([str(binary), *sys.argv[1:]], env=env)

    def forward(sig, _frame=None):
        try:
            proc.send_signal(sig)
        except Exception:
            pass

    for sig_name in ("SIGINT", "SIGTERM", "SIGHUP"):
        sig = getattr(signal, sig_name, None)
        if sig is not None:
            signal.signal(sig, forward)

    code = proc.wait()
    if code < 0:
        os.kill(os.getpid(), -code)
    return code


if __name__ == "__main__":
    raise SystemExit(main())
