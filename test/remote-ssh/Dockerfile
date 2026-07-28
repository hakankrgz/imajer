FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server sudo util-linux ca-certificates \
    && useradd --create-home --shell /bin/bash forensic \
    && usermod --password '*' forensic \
    && mkdir -p /home/forensic/.ssh /evidence \
    && printf 'forensic ALL=(ALL) NOPASSWD: ALL\n' > /etc/sudoers.d/imajer-forensic \
    && chmod 0440 /etc/sudoers.d/imajer-forensic \
    && rm -rf /var/lib/apt/lists/*

COPY sshd_config /etc/ssh/sshd_config
COPY authorized_keys /home/forensic/.ssh/authorized_keys
COPY source.raw /evidence/source.raw

RUN chown -R forensic:forensic /home/forensic/.ssh \
    && chmod 0700 /home/forensic/.ssh \
    && chmod 0600 /home/forensic/.ssh/authorized_keys \
    && chmod 0444 /evidence/source.raw \
    && ssh-keygen -A

EXPOSE 22

CMD ["sh", "-c", "mkdir -p /run/sshd && exec /usr/sbin/sshd -D -e"]
