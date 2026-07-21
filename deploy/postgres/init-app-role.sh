#!/usr/bin/env sh
set -eu

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
create role agentforge_app login password '${POSTGRES_APP_PASSWORD}' noinherit nobypassrls nosuperuser nocreatedb nocreaterole;
SQL
