#!/usr/bin/env bash
# Builds the control plane and its admin app for linux/amd64 and pushes them, so the server can pull them (they are not
# built by a release workflow: this folder has no git remote). Run it from your own machine, logged in to the registry:
#   echo $GITHUB_TOKEN | docker login ghcr.io -u <user> --password-stdin
#   REGISTRY=ghcr.io/redefynelabs ./publish.sh v0.1.0
# Then put the printed lines in the server's .env. The packages must be private (or the server logged in to read them).
set -euo pipefail
version=${1:?usage: REGISTRY=ghcr.io/<owner> publish.sh <version>}
registry=${REGISTRY:?set REGISTRY, for example ghcr.io/redefynelabs}
root=$(cd "$(dirname "$0")/.." && pwd)

docker buildx build --platform linux/amd64 -t "$registry/erp-control-plane:$version" --push "$root"
docker buildx build --platform linux/amd64 -t "$registry/erp-control-plane-web:$version" --push "$root/web"

echo
echo "CONTROLPLANE_IMAGE=$registry/erp-control-plane:$version"
echo "ADMIN_IMAGE=$registry/erp-control-plane-web:$version"
