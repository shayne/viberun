FROM ubuntu:25.10

ENV DEBIAN_FRONTEND=noninteractive
ENV VIBERUN_USER=viberun
ENV VIBERUN_HOME=/home/viberun
ENV HOME=/home/viberun
ENV CODEX_HOME=/home/viberun/.codex
ENV VIBERUN_SKILLS_HOME=/opt/viberun/skills
ENV VIBERUN_APP_DIR=/home/viberun/app
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8
ENV LC_CTYPE=C.UTF-8

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    git \
    iproute2 \
    iputils-ping \
    lsof \
    ncurses-bin \
    ncurses-term \
    nano \
    openssh-client \
    ripgrep \
    s6 \
    python3 \
    python3-venv \
    python-is-python3 \
    sudo \
    tmux \
    tzdata \
    vim \
    wget \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

RUN useradd -m -d ${VIBERUN_HOME} -s /bin/bash ${VIBERUN_USER}

RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
  && apt-get install -y --no-install-recommends nodejs \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_UNMANAGED_INSTALL="/usr/local/bin" UV_NO_MODIFY_PATH=1 sh

RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y \
  && mkdir -p ${VIBERUN_HOME}/.config

RUN set -eux; \
  mkdir -p -m 755 /etc/apt/keyrings; \
  wget -nv -O /etc/apt/keyrings/githubcli-archive-keyring.gpg https://cli.github.com/packages/githubcli-archive-keyring.gpg; \
  chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg; \
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    > /etc/apt/sources.list.d/github-cli.list; \
  apt-get update; \
  apt-get install -y --no-install-recommends gh; \
  apt-get clean; \
  rm -rf /var/lib/apt/lists/*

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
COPY config/bashrc-viberun.sh /etc/profile.d/viberun.sh
RUN chmod +x /usr/local/bin/viberun-tmux-status \
  && chmod +x /usr/local/bin/vrctl \
  && cat /etc/profile.d/viberun.sh >> /etc/bash.bashrc

RUN mkdir -p ${VIBERUN_SKILLS_HOME} /opt/viberun/templates
COPY skills/ ${VIBERUN_SKILLS_HOME}/
COPY config/starship.toml /opt/viberun/templates/starship.toml
COPY config/AGENTS.app.md /opt/viberun/templates/AGENTS.app.md
COPY config/codex-config.toml /opt/viberun/templates/codex-config.toml
RUN mkdir -p ${VIBERUN_HOME}/.local/services ${VIBERUN_HOME}/.local/logs ${VIBERUN_APP_DIR} \
  && chown -R ${VIBERUN_USER}:${VIBERUN_USER} ${VIBERUN_HOME}

COPY bin/viberun-entrypoint /usr/local/bin/viberun-entrypoint
COPY bin/viberun-env /usr/local/bin/viberun-env
RUN chmod +x /usr/local/bin/viberun-entrypoint /usr/local/bin/viberun-env

USER ${VIBERUN_USER}

WORKDIR /home/viberun/app

ENTRYPOINT ["/usr/local/bin/viberun-entrypoint"]
CMD ["/usr/bin/s6-svscan", "/home/viberun/.local/services"]
