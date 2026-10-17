#!/usr/bin/env bash
# Dumps the control plane's own database (leads, sales, team, settings). Demo databases are disposable and not saved.
# Run it daily from cron, then copy the file off this server (a storage bucket, another machine): a backup on the same
# disk is lost with it.
#   0 2 * * *  /opt/erp-host/backup.sh /var/backups/controlplane
set -euo pipefail
dir=${1:?usage: backup.sh <directory>}
mkdir -p "$dir"
file="$dir/controlplane-$(date -u +%F).sql.gz"
docker compose -f "$(dirname "$0")/docker-compose.yml" exec -T postgres pg_dump -U postgres controlplane | gzip > "$file"
# Keep two weeks.
find "$dir" -name 'controlplane-*.sql.gz' -mtime +14 -delete
echo "saved $file"
