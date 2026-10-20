#!/bin/bash
# Installed on the server as /opt/erp-host/ci-deploy.sh; it runs as the forced command of the CI deploy key (see deploy/README.md, "Deploy from GitHub Actions"). The
# key can do nothing else: GitHub Actions sends "deploy <version> [<registry user>]" and, on stdin, a registry token that
# is only used to pull the two images and is then dropped.
set -euo pipefail

read -ra args <<< "${SSH_ORIGINAL_COMMAND:-}"
[ "${args[0]:-}" = deploy ] || { echo "usage: deploy <version> [registry user]" >&2; exit 2; }
version=${args[1]:-}
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9.]+)?$ ]] || { echo "bad version" >&2; exit 2; }
user=${args[2]:-}
[[ -z $user || $user =~ ^[A-Za-z0-9-]+$ ]] || { echo "bad user" >&2; exit 2; }

cd "$(dirname "$0")/erp-control-plane/deploy"
[ -f .env ] || { echo "no .env here" >&2; exit 1; }

if [ -n "$user" ]; then
  trap 'docker logout ghcr.io >/dev/null 2>&1 || true' EXIT
  docker login ghcr.io -u "$user" --password-stdin >/dev/null
fi

# Keep the old tags so a failed start can go back to them.
old_cp=$(grep -E '^CONTROLPLANE_IMAGE=' .env | cut -d= -f2-)
old_admin=$(grep -E '^ADMIN_IMAGE=' .env | cut -d= -f2-)
set_tags() { # $1 control plane image, $2 admin image
  sed -i "s|^CONTROLPLANE_IMAGE=.*|CONTROLPLANE_IMAGE=$1|; s|^ADMIN_IMAGE=.*|ADMIN_IMAGE=$2|" .env
}
new_cp="${old_cp%:*}:$version"
new_admin="${old_admin%:*}:$version"

healthy() {
  for _ in $(seq 1 30); do
    if docker compose exec -T controlplane wget -qO- http://127.0.0.1:8090/healthz 2>/dev/null | grep -q '"ok"'; then return 0; fi
    sleep 2
  done
  return 1
}

set_tags "$new_cp" "$new_admin"
docker compose pull controlplane admin
docker compose up -d controlplane admin
if healthy; then
  docker image prune -f >/dev/null
  echo "deployed $version"
  exit 0
fi

echo "the new version did not become healthy; going back to $old_cp" >&2
docker compose logs --tail 40 controlplane >&2 || true
set_tags "$old_cp" "$old_admin"
docker compose up -d controlplane admin
exit 1
