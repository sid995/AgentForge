#!/usr/bin/env sh
set -eu

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
create role agentforge_app login password '${POSTGRES_APP_PASSWORD}' noinherit nobypassrls nosuperuser nocreatedb nocreaterole;
create role agentforge_relay login password '${POSTGRES_RELAY_PASSWORD}' noinherit bypassrls nosuperuser nocreatedb nocreaterole;
create role agentforge_scheduler login password '${POSTGRES_SCHEDULER_PASSWORD}' noinherit bypassrls nosuperuser nocreatedb nocreaterole;
create role agentforge_handoff login password '${POSTGRES_HANDOFF_PASSWORD}' noinherit bypassrls nosuperuser nocreatedb nocreaterole;
SQL
