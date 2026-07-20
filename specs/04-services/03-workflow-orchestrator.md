# Workflow Orchestrator

## Purpose

Coordinate the long-running saga from run acceptance through execution, build, scan, and deployment.

## Responsibilities

- Persist workflow step and version.
- React to lifecycle events.
- Issue commands to the next participant.
- Apply timeout and compensation rules.
- Expose understandable workflow status.

## Workflow steps

1. Validate and reserve budget.
2. Schedule execution.
3. Await successful attempt.
4. Request build.
5. Await scan and policy acceptance.
6. Create release.
7. Optionally request deployment.
8. Finalize cost and release reservation.

## Compensations

- Release unused budget reservation.
- Delete temporary workspace and artifacts.
- Mark unreferenced image for lifecycle cleanup.
- Revert GitOps commit when rollout policy requires it.

Audit and analytics are event choreography, not blocking workflow steps.
