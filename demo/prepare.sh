#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
KEY_DIR="$SCRIPT_DIR/keys"
SOURCE_PATH="$SCRIPT_DIR/source.raw"
JOB_PATH="$SCRIPT_DIR/local-job.yaml"
EVIDENCE_DIR="$SCRIPT_DIR/evidence"
PRIVATE_KEY="$KEY_DIR/examiner-private.pem"
PUBLIC_KEY="$KEY_DIR/examiner-public.pem"
AGENT_PATH="$PROJECT_DIR/dist/imajer-agent"

command -v openssl >/dev/null 2>&1 || {
  echo "HATA: openssl bulunamadı." >&2
  exit 1
}
test -x "$AGENT_PATH" || {
  echo "HATA: dist/imajer-agent bulunamadı. Önce 'make build' çalıştırın." >&2
  exit 1
}

mkdir -p "$KEY_DIR" "$EVIDENCE_DIR"

if test ! -f "$SOURCE_PATH"; then
  dd if=/dev/zero of="$SOURCE_PATH" bs=1048576 count=2 2>/dev/null
fi

if test ! -f "$PRIVATE_KEY"; then
  openssl genpkey -algorithm ED25519 -out "$PRIVATE_KEY"
  chmod 0600 "$PRIVATE_KEY"
fi
if test ! -f "$PUBLIC_KEY"; then
  openssl pkey -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY"
fi

cat > "$JOB_PATH" <<EOF
case:
  id: CASE-LOCAL-001
  evidence_id: EVID-LOCAL-001
  examiner: Local Demo Operator
  organization: IMAJER Lab
  authority_ref: AUTHORIZED-FUNCTIONAL-TEST
  authorized: true

target:
  transport: local

acquisition:
  profile: disk
  chunk_size: 1048576
  segment_size: 2097152
  disk:
    path: $SOURCE_PATH
    id: local-test-source
    model: synthetic-file
    size: 2097152
    sector_size: 512
    provider: native

output:
  directory: $EVIDENCE_DIR
  signing_key: $PRIVATE_KEY

agent:
  local_path: $AGENT_PATH

retry:
  max_attempts: 3
  connect_timeout: 5s
  chunk_timeout: 30s
  cleanup_timeout: 30s
EOF
chmod 0600 "$JOB_PATH"

echo "Yerel demo hazırlandı:"
echo "  Job: $JOB_PATH"
echo "  Public key: $PUBLIC_KEY"
