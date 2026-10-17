#!/usr/bin/env bash
# Installs the ERP on this server with one command. Run it as root on a fresh Ubuntu or Debian server whose domain already
# points at it (a user who can already run docker needs no root):
#
#   curl -fsSL __CP_URL__/install | sudo bash -s -- --code ACT-XXXXX-XXXXX-XXXXX-XXXXX --domain erp.example.com
#
# It checks the code with your provider, installs Docker if needed, downloads the application, and starts it with HTTPS
# and nightly backups. Afterwards you open the address and create your administrator account. Safe to run again.
#
# Options: --dir /opt/erp (where it goes), --http (no HTTPS: for trying it on an IP address), --dry-run (check everything,
# change nothing).
set -euo pipefail

CP_URL="__CP_URL__"
DIR=/opt/erp
CODE=""
DOMAIN=""
HTTP=0
DRY=0

say() { echo "==> $*"; }
die() { echo "error: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --code) CODE=${2:-}; shift 2 ;;
    --domain) DOMAIN=${2:-}; shift 2 ;;
    --dir) DIR=${2:-}; shift 2 ;;
    --http) HTTP=1; shift ;;
    --dry-run) DRY=1; shift ;;
    -h | --help) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option $1" ;;
  esac
done
[ -n "$CODE" ] && [ -n "$DOMAIN" ] || die "both --code and --domain are needed (the activation code your provider gave you, and the address you will use, for example erp.example.com)"
case "$DOMAIN" in *[!A-Za-z0-9.:-]* | "") die "the domain may hold only letters, digits, dots, dashes and a port" ;; esac
# Root is needed to install Docker and to write to /opt; a user who can already run docker and write to --dir needs neither.
if ! command -v docker >/dev/null && [ "$DRY" = 0 ]; then
  [ "$(id -u)" = 0 ] || die "Docker is not installed, and installing it needs root: put sudo before bash"
  [ "$(uname -s)" = Linux ] || die "this installer installs Docker only on Linux; install Docker first"
fi
command -v curl >/dev/null || die "curl is required (apt-get install -y curl)"

# 1. Ask the provider. This does not use the code up: the application does that when you create your account.
say "checking the activation code with your provider"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
status=$(curl -sS --max-time 30 -o "$tmp" -w '%{http_code}' -X POST "$CP_URL/v1/install/start" \
  -H 'Content-Type: application/json' -H 'Accept: text/x-shellvars' -d "{\"code\":\"$CODE\"}") || die "could not reach your provider at $CP_URL; check this server's internet connection"
case "$status" in
  200) ;;
  422) die "that code is not valid, was already used, or has expired: ask your provider for a new one" ;;
  429) die "too many attempts from this address: wait a few minutes and try again" ;;
  409) die "your provider has not published a version yet: tell them, and try again later" ;;
  *) die "your provider answered $status: $(head -c 200 "$tmp")" ;;
esac
REGISTRY=""; REGISTRY_USER=""; REGISTRY_TOKEN=""; VERSION=""; CUSTOMER=""
# Only known keys are read, and the provider only sends safe characters, so nothing is run from the answer.
while IFS='=' read -r key value; do
  value=${value#\'}; value=${value%\'}
  case "$key" in
    REGISTRY) REGISTRY=$value ;;
    REGISTRY_USER) REGISTRY_USER=$value ;;
    REGISTRY_TOKEN) REGISTRY_TOKEN=$value ;;
    VERSION) VERSION=$value ;;
    CUSTOMER) CUSTOMER=$value ;;
  esac
done < "$tmp"
[ -n "$REGISTRY" ] && [ -n "$VERSION" ] || die "your provider's answer was incomplete"
say "installing $VERSION${CUSTOMER:+ for $CUSTOMER}"

# 2. The domain must point here, or HTTPS cannot be issued.
if [ "$HTTP" = 0 ]; then
  here=$(curl -fsS --max-time 15 "$CP_URL/v1/install/ip" 2>/dev/null || true)
  there=$(getent hosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | head -1 || true)
  if [ -z "$there" ]; then
    die "$DOMAIN does not point anywhere yet. In your DNS add an A record for it with this server's address${here:+ ($here)}, wait a few minutes, and run this again"
  fi
  if [ -n "$here" ] && [ "$there" != "$here" ]; then
    die "$DOMAIN points to $there but this server is $here. Fix the A record, wait a few minutes, and run this again"
  fi
fi

if [ "$DRY" = 1 ]; then
  say "dry run: the code is valid and $DOMAIN is set up. A real run would install Docker if needed, sign in to $REGISTRY, download erp-server:$VERSION into $DIR and start it."
  exit 0
fi

# 3. Docker.
if ! command -v docker >/dev/null; then
  say "installing Docker"
  curl -fsSL https://get.docker.com | sh >/dev/null
fi
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required; install the docker-compose-plugin package"

# 4. The application. The installer files travel inside the image itself, so there is nothing else to download or copy.
host=${REGISTRY%%/*}
if [ -n "$REGISTRY_TOKEN" ]; then
  say "signing in to the registry"
  echo "$REGISTRY_TOKEN" | docker login "$host" -u "$REGISTRY_USER" --password-stdin >/dev/null
fi
image="$REGISTRY/erp-server:$VERSION"
say "downloading $VERSION (a few minutes the first time)"
docker pull --quiet "$image" >/dev/null || die "could not download $image"
mkdir -p "$DIR" 2>/dev/null || die "could not create $DIR: run as root (sudo), or choose a folder you can write to with --dir"
docker run --rm --user 0 --entrypoint /app/server -v "$DIR:/out" "$image" install-files /out || die "could not unpack the installer from the image"
chmod +x "$DIR"/*.sh

# 5. Start it. install.sh creates the secrets once, starts everything, waits until it is healthy and schedules backups.
cd "$DIR"
opts=(--version "$VERSION" --registry "$REGISTRY")
[ "$HTTP" = 1 ] && opts+=(--insecure-http)
ACTIVATION_CODE="$CODE" ./install.sh "$DOMAIN" "${opts[@]}"
