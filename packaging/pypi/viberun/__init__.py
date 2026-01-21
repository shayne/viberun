from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("viberun")
except PackageNotFoundError:  # pragma: no cover
    __version__ = "0.0.0"
