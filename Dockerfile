FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV CODEX_HOME=/root/.codex
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV LC_CTYPE=C.UTF-8

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    ncurses-bin \
    ncurses-term \
    s6 \
    nodejs \
    npm \
    sudo \
    tmux \
    tzdata \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y \
  && mkdir -p /root/.config

RUN set -eux; \
  printf '%s\n' \
    '#!/bin/sh' \
    'if [ -n "${VIBERUN_AGENT_CHECK:-}" ]; then' \
    '  exec sh -c "$VIBERUN_AGENT_CHECK"' \
    'fi' \
    'exec npx -y @openai/codex@latest --dangerously-bypass-approvals-and-sandbox "$@"' \
    > /usr/local/bin/codex; \
  printf '%s\n' \
    '#!/bin/sh' \
    'if [ -n "${VIBERUN_AGENT_CHECK:-}" ]; then' \
    '  exec sh -c "$VIBERUN_AGENT_CHECK"' \
    'fi' \
    'exec npx -y @anthropic-ai/claude-code@latest --dangerously-skip-permissions "$@"' \
    > /usr/local/bin/claude; \
  printf '%s\n' \
    '#!/bin/sh' \
    'if [ -n "${VIBERUN_AGENT_CHECK:-}" ]; then' \
    '  exec sh -c "$VIBERUN_AGENT_CHECK"' \
    'fi' \
    'exec npx -y @google/gemini-cli@latest --approval-mode=yolo "$@"' \
    > /usr/local/bin/gemini; \
  printf '%s\n' \
    '#!/bin/sh' \
    'set -e' \
    'url="${1:-}"' \
    'if [ -z "$url" ]; then' \
    '  echo "xdg-open: missing url" >&2' \
    '  exit 2' \
    'fi' \
    'socket="${VIBERUN_XDG_OPEN_SOCKET:-/tmp/viberun-open.sock}"' \
    'if [ -S "$socket" ]; then' \
    '  exec curl -sS --unix-socket "$socket" -X POST --data-urlencode "url=$url" http://localhost/open' \
    'fi' \
    'if [ -x /usr/bin/xdg-open ]; then' \
    '  exec /usr/bin/xdg-open "$url"' \
    'fi' \
    'echo "xdg-open forwarding unavailable; missing socket $socket" >&2' \
    'exit 1' \
    > /usr/local/bin/xdg-open; \
  printf '%s\n' \
    '#!/bin/sh' \
    "printf 'viberun-agent-check ok\\\\n'" \
    > /usr/local/bin/viberun-agent-check; \
  chmod +x /usr/local/bin/codex /usr/local/bin/claude /usr/local/bin/gemini /usr/local/bin/xdg-open /usr/local/bin/viberun-agent-check

COPY config/terminfo/ghostty-terminfo /tmp/ghostty-terminfo
RUN tic -x /tmp/ghostty-terminfo \
  && rm -f /tmp/ghostty-terminfo

COPY bin/viberun-tmux-status /usr/local/bin/viberun-tmux-status
COPY bin/vrctl /usr/local/bin/vrctl
COPY config/tmux.conf /etc/tmux.conf
COPY config/starship.toml /root/.config/starship.toml
COPY config/bashrc-viberun.sh /etc/profile.d/viberun.sh
RUN chmod +x /usr/local/bin/viberun-tmux-status \
  && chmod +x /usr/local/bin/vrctl \
  && cat /etc/profile.d/viberun.sh >> /etc/bash.bashrc

RUN mkdir -p ${CODEX_HOME}/skills
COPY skills/ ${CODEX_HOME}/skills/
RUN mkdir -p ${CODEX_HOME} \
  && printf '%s\n' \
    '[features]' \
    'skills = true' \
    'web_search_request = true' \
    'unified_exec = true' \
    'shell_snapshot = true' \
    'steer = true' \
    > ${CODEX_HOME}/config.toml
RUN mkdir -p /etc/services.d /var/log/vrctl

CMD ["/usr/bin/s6-svscan", "/etc/services.d"]
