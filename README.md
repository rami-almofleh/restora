# Restora

Self-hosted Backup und Restore für Kubernetes. Läuft im eigenen Cluster, Daten und Zugangsdaten bleiben bei dir.

**Status:** Phase 0 bis 10 umgesetzt. Siehe [PLAN.md](PLAN.md).

- Regeln für die Oberfläche: [docs/DESIGN.md](docs/DESIGN.md)
- Regeln für den Code: [docs/CONVENTIONS.md](docs/CONVENTIONS.md)

## Lokal ausprobieren

Voraussetzungen: Docker, minikube, kubectl, Helm, Go, Node.

```bash
minikube start -p restora-dev --driver=docker --cpus=2 --memory=4096 --keep-context
```

```bash
hack/dev-install.sh
```

```bash
hack/dev-port-forward.sh
```

Danach ist die Oberfläche unter http://localhost:8080 erreichbar. Die Skripte arbeiten nur mit dem Cluster `restora-dev` und fassen deinen Standard-`kubectl`-Kontext nicht an.

## Tests

```bash
go test ./...
```

```bash
cd ui && npm test && npm run typecheck
```
