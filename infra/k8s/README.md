# Kubernetes manifests (minimal)

These manifests deploy the **core services**. Dependencies (Postgres, Kafka/Redpanda, Redis, Keycloak)
are usually installed via Helm charts in real setups.

For local k8s (kind/minikube):
- Install Postgres + Redpanda via Helm (or use managed cloud services)
- Apply these manifests:
```bash
kubectl apply -f infra/k8s/
```
