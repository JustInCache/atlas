#!/bin/sh
# Writes config and kubeconfigs from env before starting Atlas. For ECS Fargate,
# Secrets Manager JSON keys are injected as environment variables.
set -eu

# Path where generated config is written (must be writable by appuser).
WRITE_PATH="${CONFIG_WRITE_PATH:-/home/appuser/config.yaml}"

if [ -n "${CONFIG_YAML_B64:-}" ]; then
  echo "$CONFIG_YAML_B64" | base64 -d >"$WRITE_PATH"
  export CONFIG_PATH="$WRITE_PATH"
elif [ -n "${CONFIG_YAML:-}" ]; then
  printf '%s\n' "$CONFIG_YAML" >"$WRITE_PATH"
  export CONFIG_PATH="$WRITE_PATH"
fi

# Optional: materialize kubeconfig files expected by config.yaml (e.g. /app/kubeconfigs/qa10.yaml).
# Set KUBECONFIG_ENV_MAP to comma-separated cluster_id=ENV_NAME pairs. Each ENV_NAME must hold
# the full kubeconfig file body (e.g. from Secrets Manager keys kubeconfig_qa10).
# Example: KUBECONFIG_ENV_MAP=qa10=KUBECONFIG_QA10,qa11=KUBECONFIG_QA11
KUBECONFIG_DIR="${KUBECONFIG_DIR:-/app/kubeconfigs}"

if [ -n "${KUBECONFIG_ENV_MAP:-}" ]; then
  mkdir -p "$KUBECONFIG_DIR"
  OLDIFS=$IFS
  IFS=,
  for pair in $KUBECONFIG_ENV_MAP; do
    cluster_id=$(echo "$pair" | cut -d= -f1)
    env_name=$(echo "$pair" | cut -d= -f2-)
    if [ -z "$cluster_id" ] || [ -z "$env_name" ]; then
      continue
    fi
    # shellcheck disable=SC2086
    eval "val=\$$env_name"
    if [ -n "${val:-}" ]; then
      printf '%s\n' "$val" >"${KUBECONFIG_DIR}/${cluster_id}.yaml"
    fi
  done
  IFS=$OLDIFS
fi

exec /app/atlas "$@"
