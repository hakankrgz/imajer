#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
RUNTIME_DIR="$SCRIPT_DIR/runtime"
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
RUN_DIR="$RUNTIME_DIR/$RUN_ID"
CONTEXT_DIR="$RUN_DIR/context"
CONTAINER_NAME="imajer-remote-e2e"
IMAGE_NAME="imajer-remote-e2e:local"
SSH_PORT="${IMAJER_TEST_SSH_PORT:-22222}"

cleanup() {
  if test "${IMAJER_TEST_KEEP_CONTAINER:-0}" != "1"; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "HATA: '$1' komutu bulunamadı." >&2
    exit 1
  }
}

for command_name in docker ssh ssh-keygen ssh-keyscan openssl dd shasum; do
  need "$command_name"
done

test -x "$PROJECT_DIR/dist/imajer" || {
  echo "HATA: dist/imajer bulunamadı. Önce 'make build cross' çalıştırın." >&2
  exit 1
}

DOCKER_ARCH=$(docker info --format '{{.Architecture}}')
case "$DOCKER_ARCH" in
  arm64|aarch64) AGENT_ARCH=arm64 ;;
  amd64|x86_64) AGENT_ARCH=amd64 ;;
  *)
    echo "HATA: Desteklenmeyen Docker mimarisi: $DOCKER_ARCH" >&2
    exit 1
    ;;
esac
AGENT_BINARY="$PROJECT_DIR/dist/imajer-agent-linux-$AGENT_ARCH"
test -x "$AGENT_BINARY" || {
  echo "HATA: $AGENT_BINARY bulunamadı." >&2
  exit 1
}

mkdir -p "$CONTEXT_DIR" "$RUN_DIR/evidence"
cp "$SCRIPT_DIR/Dockerfile" "$CONTEXT_DIR/Dockerfile"
cp "$SCRIPT_DIR/sshd_config" "$CONTEXT_DIR/sshd_config"
cp "$AGENT_BINARY" "$RUN_DIR/imajer-agent-linux-$AGENT_ARCH"
SIGNED_AGENT="$RUN_DIR/imajer-agent-linux-$AGENT_ARCH"
IMAJER_VERSION=$("$PROJECT_DIR/dist/imajer" version)

ssh-keygen -q -t ed25519 -N "" -C "imajer-e2e-$RUN_ID" -f "$RUN_DIR/ssh-client"
cp "$RUN_DIR/ssh-client.pub" "$CONTEXT_DIR/authorized_keys"
dd if=/dev/zero of="$CONTEXT_DIR/source.raw" bs=1048576 count=24 2>/dev/null
SOURCE_SIZE=$(wc -c < "$CONTEXT_DIR/source.raw" | tr -d ' ')

openssl genpkey -algorithm ED25519 -out "$RUN_DIR/tool-release-private.pem"
openssl pkey -in "$RUN_DIR/tool-release-private.pem" -pubout -out "$RUN_DIR/tool-release-public.pem"
openssl genpkey -algorithm ED25519 -out "$RUN_DIR/examiner-private.pem"
openssl pkey -in "$RUN_DIR/examiner-private.pem" -pubout -out "$RUN_DIR/examiner-public.pem"
chmod 0600 "$RUN_DIR/ssh-client" "$RUN_DIR/tool-release-private.pem" "$RUN_DIR/examiner-private.pem"

cat > "$RUN_DIR/tools.yaml" <<EOF
- name: imajer-agent
  version: "$IMAJER_VERSION"
  os: linux
  arch: $AGENT_ARCH
  path: $SIGNED_AGENT
  license: Apache-2.0
EOF

"$PROJECT_DIR/dist/imajer" tools sign \
  --spec "$RUN_DIR/tools.yaml" \
  --key "$RUN_DIR/tool-release-private.pem" \
  --out "$RUN_DIR/tool-manifest.json"
"$PROJECT_DIR/dist/imajer" tools verify \
  --manifest "$RUN_DIR/tool-manifest.json" \
  --key "$RUN_DIR/tool-release-public.pem"

docker build -t "$IMAGE_NAME" "$CONTEXT_DIR"
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
docker run -d \
  --name "$CONTAINER_NAME" \
  --hostname imajer-test-linux \
  --read-only \
  --tmpfs /run \
  --tmpfs /tmp:rw,noexec,nosuid,size=32m \
  --tmpfs /dev/shm:rw,exec,nosuid,size=64m \
  -p "127.0.0.1:$SSH_PORT:22" \
  "$IMAGE_NAME" >/dev/null

attempt=0
while :; do
  attempt=$((attempt + 1))
  if ssh-keyscan -p "$SSH_PORT" 127.0.0.1 > "$RUN_DIR/known_hosts" 2>"$RUN_DIR/ssh-keyscan.log"; then
    break
  fi
  if test "$attempt" -ge 20; then
    docker logs "$CONTAINER_NAME" >&2 || true
    echo "HATA: SSH sunucusu hazır olmadı." >&2
    exit 1
  fi
  sleep 1
done

echo "Sunucu host-key fingerprint:"
docker exec "$CONTAINER_NAME" ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
echo "Controller known_hosts fingerprintleri:"
ssh-keygen -lf "$RUN_DIR/known_hosts"

ssh \
  -i "$RUN_DIR/ssh-client" \
  -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$RUN_DIR/known_hosts" \
  -p "$SSH_PORT" forensic@127.0.0.1 \
  "uname -a && test \"\$(sudo -n id -u)\" = 0"

cat > "$RUN_DIR/job.yaml" <<EOF
case:
  id: CASE-SSH-E2E
  evidence_id: EVID-DISK-001
  examiner: IMAJER Test Operator
  organization: IMAJER Lab
  authority_ref: AUTHORIZED-E2E-TEST
  notes: Docker üzerinde gerçek SSH/SFTP ve in-memory streaming testi
  authorized: true

target:
  transport: ssh
  host: 127.0.0.1
  port: $SSH_PORT
  user: forensic
  private_key: $RUN_DIR/ssh-client
  known_hosts: $RUN_DIR/known_hosts

acquisition:
  profile: disk
  chunk_size: 8388608
  segment_size: 16777216
  disk:
    path: /evidence/source.raw
    id: source.raw
    model: ""
    size: $SOURCE_SIZE
    sector_size: 512
    provider: native

output:
  directory: $RUN_DIR/evidence
  signing_key: $RUN_DIR/examiner-private.pem

agent:
  local_path: $SIGNED_AGENT
  tool_manifest: $RUN_DIR/tool-manifest.json
  trust_public_key: $RUN_DIR/tool-release-public.pem

retry:
  max_attempts: 3
  connect_timeout: 10s
  chunk_timeout: 1m
  cleanup_timeout: 30s
EOF

echo
echo "1/5 NEGATIVE: HOST-KEY DEĞİŞİMİ REDDEDİLİYOR"
ssh-keygen -q -t ed25519 -N "" -f "$RUN_DIR/wrong-host-key"
{
  printf '[127.0.0.1]:%s ' "$SSH_PORT"
  awk '{print $1 " " $2}' "$RUN_DIR/wrong-host-key.pub"
} > "$RUN_DIR/wrong-known_hosts"
sed "s|known_hosts: $RUN_DIR/known_hosts|known_hosts: $RUN_DIR/wrong-known_hosts|" \
  "$RUN_DIR/job.yaml" > "$RUN_DIR/wrong-host-key-job.yaml"
if "$PROJECT_DIR/dist/imajer" discover --job "$RUN_DIR/wrong-host-key-job.yaml" \
  >"$RUN_DIR/wrong-host-key.log" 2>&1; then
  echo "HATA: Değişmiş SSH host key kabul edildi." >&2
  exit 1
fi
grep -Eiq 'knownhosts|host key|key mismatch|eşleş' "$RUN_DIR/wrong-host-key.log" || {
  echo "HATA: Host-key reddi beklenen nedenle olmadı." >&2
  cat "$RUN_DIR/wrong-host-key.log" >&2
  exit 1
}

echo
echo "2/5 NEGATIVE: YANLIŞ DİSK KİMLİĞİ REDDEDİLİYOR"
sed \
  -e 's/id: CASE-SSH-E2E/id: CASE-SSH-NEGATIVE/' \
  -e 's/evidence_id: EVID-DISK-001/evidence_id: EVID-DISK-NEGATIVE/' \
  -e 's/id: source.raw/id: definitely-wrong-disk/' \
  "$RUN_DIR/job.yaml" > "$RUN_DIR/wrong-disk-job.yaml"
if "$PROJECT_DIR/dist/imajer" discover --job "$RUN_DIR/wrong-disk-job.yaml" \
  >"$RUN_DIR/wrong-disk.log" 2>&1; then
  echo "HATA: Yanlış disk kimliği kabul edildi." >&2
  exit 1
fi
grep -q 'does not match target identifiers' "$RUN_DIR/wrong-disk.log" || {
  echo "HATA: Disk kimliği reddi beklenen nedenle olmadı." >&2
  cat "$RUN_DIR/wrong-disk.log" >&2
  exit 1
}

echo
echo "3/5 DISCOVER (UBUNTU + PAROLASIZ SUDO)"
"$PROJECT_DIR/dist/imajer" discover --job "$RUN_DIR/job.yaml" | tee "$RUN_DIR/discover.log"

echo
echo "4/5 ACQUIRE"
"$PROJECT_DIR/dist/imajer" acquire --job "$RUN_DIR/job.yaml" 2>&1 | tee "$RUN_DIR/acquire.log"

CASE_DIR="$RUN_DIR/evidence/CASE-SSH-E2E/EVID-DISK-001"
ARTIFACT_DIR="$CASE_DIR/artifacts/disk"

echo
echo "5/5 VERIFY"
"$PROJECT_DIR/dist/imajer" verify \
  --case-dir "$CASE_DIR" \
  --public-key "$RUN_DIR/examiner-public.pem" | tee "$RUN_DIR/verify.log"

REMOTE_HASH=$(docker exec "$CONTAINER_NAME" sha256sum /evidence/source.raw | awk '{print $1}')
LOCAL_HASH=$(cat "$ARTIFACT_DIR"/disk.[0-9][0-9][0-9] | shasum -a 256 | awk '{print $1}')
test "$REMOTE_HASH" = "$LOCAL_HASH" || {
  echo "HATA: Uzak ve yerel mantıksal SHA-256 eşleşmedi." >&2
  exit 1
}

LEFTOVERS=$(docker exec "$CONTAINER_NAME" sh -c \
  "find /dev/shm /tmp -maxdepth 2 -name 'imajer-*' -print")
test -z "$LEFTOVERS" || {
  echo "HATA: Uzak geçici agent kalıntısı bulundu:" >&2
  echo "$LEFTOVERS" >&2
  exit 1
}

ln -sfn "$RUN_DIR" "$RUNTIME_DIR/latest"

echo
echo "TEST BAŞARILI"
echo "Hedef        : Ubuntu 24.04 / $AGENT_ARCH / passwordless sudo"
echo "Uzak SHA-256 : $REMOTE_HASH"
echo "Yerel SHA-256: $LOCAL_HASH"
echo "Job          : $RUN_DIR/job.yaml"
echo "Kanıt dizini : $CASE_DIR"
if test "${IMAJER_TEST_KEEP_CONTAINER:-0}" = "1"; then
  echo "Container    : $CONTAINER_NAME (127.0.0.1:$SSH_PORT)"
  echo "Durdurmak için: docker rm -f $CONTAINER_NAME"
else
  echo "Container    : test sonunda otomatik kaldırılacak"
fi
