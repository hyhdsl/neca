#!/usr/bin/env bash
# neca CLI wrapper: run the pinned Docker image with the production compose file.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  set -- help
fi

compose_args=(-f /opt/ipsec/docker-compose.neca.yml)
run_args=(--rm)
if [ ! -t 0 ]; then
  run_args+=(-T)
fi
if [ -n "${NECA_LANG:-}" ]; then
  run_args+=(-e "NECA_LANG=${NECA_LANG}")
fi

exec docker compose "${compose_args[@]}" run "${run_args[@]}" neca "$@"
