FROM alpine:3.22

RUN apk add --no-cache openssh-server

COPY sshd_config /etc/ssh/sshd_config
COPY authorized_keys /root/.ssh/authorized_keys
COPY source.raw /evidence/source.raw

RUN chmod 0700 /root/.ssh \
    && chmod 0600 /root/.ssh/authorized_keys \
    && chmod 0444 /evidence/source.raw \
    && ssh-keygen -A

EXPOSE 22

CMD ["/usr/sbin/sshd", "-D", "-e"]
