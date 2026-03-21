#!/bin/bash
# Creates the pocket-id database in the same PostgreSQL instance.
# Mounted into postgres via /docker-entrypoint-initdb.d/.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE pocketid;
EOSQL
