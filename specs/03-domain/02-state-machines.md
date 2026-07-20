# State Machines

## AgentRun

```text
REQUESTED -> VALIDATING -> QUEUED -> SCHEDULING -> PROVISIONING
PROVISIONING -> RUNNING -> TESTING -> BUILDING -> SCANNING
SCANNING -> READY_TO_DEPLOY -> DEPLOYING -> SUCCEEDED

Any active state -> CANCELLING -> CANCELLED
RUNNING/TESTING -> EXECUTION_FAILED
BUILDING -> BUILD_FAILED
SCANNING -> POLICY_REJECTED
DEPLOYING -> DEPLOYMENT_FAILED
Active state -> TIMED_OUT
```

## Attempt

```text
PENDING -> STARTING -> ACTIVE -> COMPLETED
STARTING/ACTIVE -> FAILED
STARTING/ACTIVE -> CANCELLED
ACTIVE -> TIMED_OUT
```

## Deployment

```text
REQUESTED -> AWAITING_APPROVAL -> COMMITTING -> SYNCING -> PROGRESSING -> HEALTHY
COMMITTING/SYNCING/PROGRESSING -> FAILED
HEALTHY -> ROLLING_BACK -> HEALTHY
```

## Transition implementation rules

- Validate expected current state and version in one SQL update.
- Emit transition event through the same transaction's outbox row.
- Record actor and reason.
- Treat repeated transition requests as idempotent when the target state is already reached.
