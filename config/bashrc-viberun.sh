export LANG="${LANG:-C.UTF-8}"
export LC_ALL="${LC_ALL:-C.UTF-8}"
export LC_CTYPE="${LC_CTYPE:-C.UTF-8}"

if [ -n "${PS1:-}" ] && command -v starship >/dev/null 2>&1; then
  export STARSHIP_CONFIG=/root/.config/starship.toml
  eval "$(starship init bash)"
fi
