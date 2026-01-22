export LANG="${LANG:-C.UTF-8}"
export LC_ALL="${LC_ALL:-C.UTF-8}"
export LC_CTYPE="${LC_CTYPE:-C.UTF-8}"
export PATH="${HOME}/.local/bin:${HOME}/.linuxbrew/bin:${HOME}/.linuxbrew/sbin:${PATH}"

if [ -n "${PS1:-}" ]; then
  if command -v dircolors >/dev/null 2>&1; then
    if [ -r /etc/DIR_COLORS ]; then
      eval "$(dircolors -b /etc/DIR_COLORS)"
    else
      eval "$(dircolors -b)"
    fi
  fi
  alias ls='ls --color=auto'
  alias ll='ls -lah --color=auto'
  alias la='ls -A --color=auto'
  alias l='ls -CF --color=auto'
fi

if [ -n "${PS1:-}" ] && command -v starship >/dev/null 2>&1; then
  export STARSHIP_CONFIG="${HOME}/.config/starship.toml"
  eval "$(starship init bash)"
fi

if [ -n "${PS1:-}" ] && command -v mise >/dev/null 2>&1; then
  eval "$(mise activate bash)"
fi
