FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV VIBERUN_USER=viberun
ENV VIBERUN_HOME=/home/viberun
ENV HOME=/home/viberun
ENV CODEX_HOME=/home/viberun/.codex
ENV VIBERUN_APP_DIR=/home/viberun/app
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV LC_CTYPE=C.UTF-8

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    ncurses-bin \
    ncurses-term \
    s6 \
    python3 \
    python3-venv \
    sudo \
    tmux \
    tzdata \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

RUN useradd -m -d ${VIBERUN_HOME} -s /bin/bash ${VIBERUN_USER}

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_UNMANAGED_INSTALL="/usr/local/bin" UV_NO_MODIFY_PATH=1 sh

RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y \
  && mkdir -p ${VIBERUN_HOME}/.config

RUN printf '%s\n' \
  'Defaults:viberun !authenticate' \
  'viberun ALL=(root) NOPASSWD: /usr/bin/apt *, /usr/bin/apt-get *' \
  > /etc/sudoers.d/viberun \
  && chmod 0440 /etc/sudoers.d/viberun

COPY internal/agents/agents.json /etc/viberun/agents.json
COPY bin/viberun-agent-shims /usr/local/bin/viberun-agent-shims
RUN /usr/local/bin/viberun-agent-shims /etc/viberun/agents.json /usr/local/bin

RUN set -eux; \
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
  chmod +x /usr/local/bin/xdg-open /usr/local/bin/viberun-agent-check

COPY config/terminfo/ghostty-terminfo /tmp/ghostty-terminfo
RUN tic -x /tmp/ghostty-terminfo \
  && rm -f /tmp/ghostty-terminfo

COPY bin/viberun-tmux-status /usr/local/bin/viberun-tmux-status
COPY bin/vrctl /usr/local/bin/vrctl
COPY config/tmux.conf /etc/tmux.conf
COPY config/starship.toml /home/viberun/.config/starship.toml
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
RUN mkdir -p ${VIBERUN_HOME}/.local/services ${VIBERUN_HOME}/.local/logs ${VIBERUN_APP_DIR} \
  && chown -R ${VIBERUN_USER}:${VIBERUN_USER} ${VIBERUN_HOME}

USER ${VIBERUN_USER}

WORKDIR /home/viberun/app

CMD ["/usr/bin/s6-svscan", "/home/viberun/.local/services"]
