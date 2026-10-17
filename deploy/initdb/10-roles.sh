#!/bin/sh
# Runs once, when Postgres first starts: the control plane's own database, and the role that owns every demo database.
set -eu
psql -v ON_ERROR_STOP=1 -U postgres <<SQL
CREATE ROLE controlplane LOGIN PASSWORD '${CP_DB_PASSWORD}';
CREATE DATABASE controlplane OWNER controlplane;
CREATE ROLE erp_demo LOGIN PASSWORD '${DEMO_ROLE_PASSWORD}';
GRANT erp_demo TO postgres;
SQL
