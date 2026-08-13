#!/usr/bin/env bash
# Create a local kind cluster and install Crossplane (v2) into it, so the whole
# install/e2e loop is reproducible from nothing. Idempotent.
source "$(dirname "$0")/lib.sh"
require_cmd kind kubectl helm

if kind get clusters 2>/dev/null | grep -qx "$KIND_CLUSTER"; then
  ok "kind cluster '$KIND_CLUSTER' already exists"
else
  log "creating kind cluster '$KIND_CLUSTER'"
  kind create cluster --name "$KIND_CLUSTER"
fi
kubectl config use-context "kind-${KIND_CLUSTER}"

log "installing Crossplane via helm"
helm repo add crossplane-stable https://charts.crossplane.io/stable >/dev/null 2>&1 || true
helm repo update >/dev/null

# Crossplane's package manager fetches package images ITSELF, over HTTPS, so a
# local registry serving a self-signed cert has to be trusted explicitly.
# `crossplane core start` exposes exactly one knob for it — `--ca-bundle-path`,
# surfaced by the chart as registryCaBundleConfig — and there is no
# insecure/plain-HTTP alternative. CROSSPLANE_REGISTRY_CA is set by e2e.sh; a
# bare `cluster-up.sh` (no local registry) leaves it empty and installs as
# before.
CP_ARGS=()
if [ -n "${CROSSPLANE_REGISTRY_CA:-}" ] && [ -f "${CROSSPLANE_REGISTRY_CA}" ]; then
  log "trusting the local registry CA in Crossplane (registryCaBundleConfig)"
  kubectl create namespace crossplane-system --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n crossplane-system create configmap e2e-registry-ca \
    --from-file=ca.crt="${CROSSPLANE_REGISTRY_CA}" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  CP_ARGS+=(--set registryCaBundleConfig.name=e2e-registry-ca --set registryCaBundleConfig.key=ca.crt)
fi

# PINNED. This install was unpinned, so a month of upstream drift landed on
# whoever opened the next PR rather than on a deliberate bump: e2e last passed
# 2026-07-09 and the next run failed at package install. Bump this on purpose.
helm upgrade --install crossplane crossplane-stable/crossplane \
  --version "${CROSSPLANE_CHART_VERSION:-2.3.4}" \
  --namespace crossplane-system --create-namespace --wait "${CP_ARGS[@]}"

log "waiting for Crossplane to be ready"
kubectl -n crossplane-system rollout status deploy/crossplane --timeout=180s
ok "cluster '$KIND_CLUSTER' ready with Crossplane — context kind-${KIND_CLUSTER}"
