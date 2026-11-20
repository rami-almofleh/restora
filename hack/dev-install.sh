#!/usr/bin/env bash
# Builds the Restora images inside the local minikube cluster and installs the Helm chart.
# Only ever talks to the "restora-dev" cluster, never to your default kubectl context.
set -euo pipefail

PROFILE="restora-dev"
CONTEXT="restora-dev"
NAMESPACE="restora-system"
VERSION="dev"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! minikube status -p "$PROFILE" >/dev/null 2>&1; then
  echo "The minikube cluster '$PROFILE' is not running." >&2
  echo "Start it with: minikube start -p $PROFILE --driver=docker --cpus=2 --memory=4096 --keep-context" >&2
  exit 1
fi

# Point docker at the Docker daemon inside minikube so the cluster can use the images directly.
eval "$(minikube -p "$PROFILE" docker-env)"

docker build -f controller/Dockerfile -t "restora-controller:$VERSION" --build-arg "VERSION=$VERSION" .
docker build -f api/Dockerfile -t "restora-api:$VERSION" --build-arg "VERSION=$VERSION" .

# Helm never upgrades CRDs, so apply them ourselves.
kubectl --context "$CONTEXT" apply -f charts/restora/crds

helm upgrade --install restora charts/restora \
  --kube-context "$CONTEXT" \
  --namespace "$NAMESPACE" --create-namespace \
  --set "image.tag=$VERSION" \
  --wait --timeout 120s

# The image tag stays the same during development, so restart to pick up the new build.
kubectl --context "$CONTEXT" --namespace "$NAMESPACE" rollout restart deployment/restora-api deployment/restora-controller
kubectl --context "$CONTEXT" --namespace "$NAMESPACE" rollout status deployment/restora-api --timeout=120s
kubectl --context "$CONTEXT" --namespace "$NAMESPACE" rollout status deployment/restora-controller --timeout=120s

echo
echo "Restora is running. Open the interface with: hack/dev-port-forward.sh"
