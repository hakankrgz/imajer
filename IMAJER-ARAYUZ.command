#!/bin/zsh
set -eu

SCRIPT_DIR="${0:A:h}"
cd "$SCRIPT_DIR"
if [[ ! -x "$SCRIPT_DIR/dist/imajer" ]]; then
  make build VERSION=0.6.4
fi
if [[ ! -f "$SCRIPT_DIR/demo/local-job.yaml" ]]; then
  "$SCRIPT_DIR/demo/prepare.sh"
fi
exec "$SCRIPT_DIR/dist/imajer" ui
