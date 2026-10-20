# Running it on Google Cloud, then moving to a VPS

One Compute Engine VM runs the whole stack in `README.md`. Nothing here is GCP-specific except creating the VM and its
firewall, so moving to a VPS later is a copy (section 6). Check prices in the console; the sizes below are a starting
point, not a promise.

## 1. Before you start

- A domain with DNS on **Cloudflare** (free). The wildcard certificate needs DNS access, and keeping DNS off GCP means
  the move to a VPS changes only two records. In `.env` set `ACME_DNS_PROVIDER=cloudflare`; in `acme.env` set
  `CF_DNS_API_TOKEN` (Zone:DNS:Edit on that zone only).
- The ERP release you want to run, published: `erp-server`, `erp-web` and `erp-demo-tools` images from the release
  workflow (`v0.5.1` or later).
- A GitHub token that can write packages (to publish) and one that can read them (for the server).

## 2. Publish the control plane images

From your machine, once per control plane change:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u <you> --password-stdin
REGISTRY=ghcr.io/redefynelabs ./publish.sh v0.1.0      # prints CONTROLPLANE_IMAGE and ADMIN_IMAGE
```

## 3. Create the VM

```bash
PROJECT=<your-project>; ZONE=asia-south1-b; REGION=asia-south1
gcloud config set project $PROJECT

# A fixed address, so DNS never has to change when the VM restarts.
gcloud compute addresses create erp-host-ip --region $REGION

gcloud compute instances create erp-host \
  --zone $ZONE --machine-type e2-standard-2 \
  --image-family debian-12 --image-project debian-cloud \
  --boot-disk-size 50GB --boot-disk-type pd-balanced \
  --address erp-host-ip --tags erp-host \
  --metadata-from-file startup-script=gcp-startup.sh

# Web traffic from everyone. SSH is not opened: use IAP (below) so port 22 is never on the internet.
gcloud compute firewall-rules create erp-host-web --allow tcp:80,tcp:443 --target-tags erp-host
gcloud compute firewall-rules create erp-host-iap-ssh --allow tcp:22 --source-ranges 35.235.240.0/20 --target-tags erp-host

# Stop a mistake from using up the credits: alert at 50%, 80% and 100% of a monthly amount.
#   Console > Billing > Budgets & alerts > Create budget (INR 6,000 a month is plenty).

gcloud compute addresses describe erp-host-ip --region $REGION --format='value(address)'   # the address for DNS
gcloud compute ssh erp-host --zone $ZONE --tunnel-through-iap
```

`e2-standard-2` (2 vCPU, 8 GB) holds about 16 live demos. `CP_MAX_DEMOS=10` leaves room; measure with
`docker stats --no-stream` after starting three demos and lower it if memory is tight. If credits run short, `e2-medium`
(4 GB) holds about 6 demos: set `CP_MAX_DEMOS=5`.

## 4. DNS (Cloudflare)

Three A records pointing at the reserved address, **DNS only** (grey cloud, not proxied): `admin`, `api` and
`*.demo`. Set the TTL to 5 minutes now, so the later move is quick.

## 5. Install

On the VM (the startup script has installed Docker, Docker Compose and `psql`):

```bash
sudo usermod -aG docker $USER && newgrp docker        # your login is created at the first SSH, after the script ran
sudo mkdir -p /opt/erp-host && sudo chown $USER /opt/erp-host && cd /opt/erp-host
# Copy this folder and ../../erp-software/ops/demo here from your machine:
#   gcloud compute scp --recurse deploy ops erp-host:/opt/erp-host --zone $ZONE --tunnel-through-iap
cd /opt/erp-host/deploy
cp .env.example .env && cp acme.env.example acme.env && chmod 600 .env acme.env
sed -i "s/^DOCKER_GID=.*/DOCKER_GID=$(getent group docker | cut -d: -f3)/" .env
for v in PG_ADMIN_PASSWORD CP_DB_PASSWORD DEMO_ROLE_PASSWORD; do sed -i "s/^$v=.*/$v=$(openssl rand -hex 24)/" .env; done
sed -i "s/^CP_SECRETS_KEY=.*/CP_SECRETS_KEY=$(openssl rand -hex 32)/" .env
$EDITOR .env acme.env      # domain, images, admin login, CP_MAX_DEMOS, demo AI key with a spend limit, Cloudflare token
echo $GHCR_READ_TOKEN | docker login ghcr.io -u <you> --password-stdin
```

Then follow `README.md`: make the entitlement key first (it must exist before the control plane starts), `docker compose up -d`,
build the demo template, sign in to the admin app. Put `ENTITLEMENT_PUBLIC_KEY` into the ERP builds you sell.

**Back up three things now, off the VM:** `keys/entitlement.key` (offline, in two places: every licence you sell
depends on it), the `.env` file (`CP_SECRETS_KEY` decrypts the stored settings), and a database dump.

Daily backup to a bucket (a few cents a month):

```bash
gsutil mb -l asia-south1 gs://<your-bucket>
( crontab -l 2>/dev/null; echo '0 2 * * * /opt/erp-host/deploy/backup.sh /var/backups/controlplane && gsutil cp /var/backups/controlplane/controlplane-$(date -u +\%F).sql.gz gs://<your-bucket>/' ) | crontab -
```

(The VM's default service account needs write access to the bucket: `--scopes storage-rw` when creating the VM, or grant the
role afterwards.)

## 6. Moving to a VPS

1. Create the VPS (4 vCPU / 8 GB is plenty), install Docker as `gcp-startup.sh` does, open ports 80 and 443, and copy this
   folder and `ops/` over.
2. Copy `.env`, `acme.env` and `keys/` from the GCP VM (same values: the passwords and `CP_SECRETS_KEY` must not change),
   and set `DOCKER_GID` for the new machine.
3. `docker compose up -d postgres`, then restore: `gunzip -c controlplane-<date>.sql.gz | docker compose exec -T postgres psql -U postgres controlplane`.
   Start the rest, rebuild the demo template, and check the admin app on the VPS's address (add a temporary hosts-file
   entry for `admin.<domain>`).
4. Stop writes on GCP (`docker compose stop controlplane`), take a last dump, restore it on the VPS, start the control plane
   there.
5. Point the three DNS records at the VPS. With a 5-minute TTL the switch is done in minutes. Keep the GCP VM stopped
   (not deleted) for a week, then delete it and release the address.

The marketing site and customers' servers use the hostnames, never the IP address, so they notice nothing. Lifetime
customers make no automatic calls to the control plane, so only your own site, the admin app and the demos depend on it.

## 7. Keeping the cost down

- Keep `CP_DEMO_TTL` at one hour or less, and `CP_MAX_DEMOS` below what fits.
- Set a spend limit on the demo AI key before you set `CP_DEMO_AI_API_KEY`.
- Stop the VM when you do not need demos (nights, while testing): the static address costs a little while it is unused,
  the disk costs a little, the rest nothing.
- Turn on the budget alert in section 3; it is the only thing between a mistake and the credits.
