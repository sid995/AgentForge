# Start Here

## 1. Create the repository

Extract this bundle into an empty project directory, then run:

```bash
git init
git branch -M main
git add .
git commit -m "docs: bootstrap AgentForge specifications"
```

## 2. Verify local tools

The first implementation phases use Go and Docker. Later phases add Kubernetes, Helm, Terraform, and cloud tools.

```bash
git --version
go version
docker --version
make --version
```

Do not block Phase 0 merely because later cloud tooling is not installed yet.

## 3. Open in GPT-5.6 Codex

Begin with the first prompt in:

`specs/15-llm-development/05-codex-execution-playbook.md`

Codex should first inspect and normalize the repository without writing production code.

## 4. Development rule

Use one bounded Codex task at a time:

1. Orient
2. Plan
3. Implement
4. Test
5. Review
6. Fix
7. Update specifications
8. Prepare commit

Do not prompt Codex with “build the whole platform.” That produces a large amount of syntax and a smaller amount of infrastructure.

## 5. Initial phase sequence

1. Repository and specification bootstrap
2. Platform API foundation
3. PostgreSQL and migrations
4. AgentRun domain and state machine
5. Transactional outbox and Kafka
6. Scheduler
7. Kubernetes Operator
8. Agent Runner
9. Artifact storage
10. Model Gateway
11. Build Service
12. Deployment and GitOps
13. Security
14. Observability
15. Cost attribution
16. Networking and edge
17. Terraform and cloud
18. Reliability and scaling
19. Chaos and incidents
20. Disaster recovery
21. Portfolio demonstration

## 6. Status discipline

After every completed task, update:

- `specs/PROJECT-STATUS.md`
- `specs/TRACEABILITY.md`
- affected specifications
- ADRs when architectural decisions change
