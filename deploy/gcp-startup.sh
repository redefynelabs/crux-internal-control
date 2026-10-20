#!/bin/bash
# Runs once as root when the VM first starts (Debian 12): installs Docker with Compose, psql and gsutil's dependencies.
set -euo pipefail
[ -f /var/lib/erp-host-ready ] && exit 0
apt-get update
apt-get install -y ca-certificates curl gnupg postgresql-client cron
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
systemctl enable --now docker
# Containers rotate their logs, so a busy demo host does not fill the disk.
cat > /etc/docker/daemon.json <<'JSON'
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }
JSON
systemctl restart docker
# Everyone who signs in over IAP SSH can use Docker.
for u in $(getent passwd | awk -F: '$3>=1000 && $3<60000 {print $1}'); do usermod -aG docker "$u" || true; done
touch /var/lib/erp-host-ready
