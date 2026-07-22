#!/usr/bin/env bash
set -euo pipefail

compose_project="${COMPOSE_PROJECT_NAME:-agentforge}"

create_topic() {
  local topic="$1"
  local retention_ms="$2"
  docker compose -f docker-compose.yml -p "${compose_project}" exec -T redpanda \
    rpk topic create "${topic}" --if-not-exists -X brokers=localhost:9092 \
    --partitions 3 --replicas 1 --topic-config "retention.ms=${retention_ms}"
}

create_topic agentforge.agent-run.lifecycle.v1 604800000
create_topic agentforge.build.lifecycle.v1 604800000
create_topic agentforge.deployment.lifecycle.v1 1209600000
create_topic agentforge.governance.events.v1 2592000000
create_topic agentforge.agent-run.lifecycle.retry.1m.v1 1209600000
create_topic agentforge.agent-run.lifecycle.retry.5m.v1 1209600000
create_topic agentforge.agent-run.lifecycle.retry.30m.v1 1209600000
create_topic agentforge.agent-run.lifecycle.dlq.v1 7776000000
