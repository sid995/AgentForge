# Vision

## Problem

AI coding agents can generate code, but production use requires durable scheduling, isolation, validation, deployment, observability, cost control, and incident handling. AgentForge provides that operational control plane.

## Target outcome

A user submits plain-language intent. AgentForge creates a durable run, schedules it fairly, provisions an isolated Kubernetes workload, records the complete execution trajectory, validates output, builds and signs an image, deploys it through Argo CD, and exposes operational and cost data.

## Portfolio objective

The project should demonstrate production-grade Go engineering, Kubernetes controllers, GitOps, distributed messaging, PostgreSQL, Redis, Kafka, observability, cloud infrastructure, security, and AI workload orchestration.

## Non-goals

- Training foundation models.
- Providing a fully general IDE.
- Supporting arbitrary privileged workloads.
- Replacing Kubernetes or Argo CD.
- Building every cloud integration in the first release.
