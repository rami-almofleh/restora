# Restora – Konventionen

Ein Muster pro Aufgabe. Vor neuem Code zuerst nachsehen, wie es an anderer Stelle gelöst ist, und dasselbe Muster nutzen.

## Allgemein

- **Sprache im Code:** Englisch (Namen, Kommentare, Logs, API). Deutsch, Englisch und Arabisch nur in den Texten der Oberfläche.
- **Kein Commit durch Claude.** Der Nutzer committet selbst.
- **Eine Funktion pro Phase.** Keine Nebenbaustellen.
- Kommentare erklären das Warum, nicht das Was.
- Keine toten Codeteile, kein auskommentierter Code.

## Ordnerstruktur

```
controller/   Operator (Go): CRDs, Reconciler, Jobs
api/          REST-Schnittstelle (Go)
ui/           Weboberfläche (React, TypeScript)
charts/       Helm-Chart
docs/         Dokumentation
hack/         Skripte und Test-Manifeste für die Entwicklung
```

Im Go-Code pro Funktion ein Paket unter `internal/<funktion>` (zum Beispiel `internal/discovery`, `internal/snapshot`, `internal/restore`). Jedes Paket hat dieselbe Form: `types.go`, `service.go`, `service_test.go`, und bei Funktionen mit API-Endpunkt zusätzlich `handler.go` (mit `Register(mux, svc)`). Gemeinsame HTTP-Hilfen liegen in `internal/server`. Vorbild: `internal/info`.

Die Oberfläche wird in das API-Programm eingebettet (`ui/embed.go`). Es gibt zwei Images: `restora-controller` und `restora-api` (API und Oberfläche).

## Go

- `gofmt -l .` muss leer sein und `go vet ./...` sauber. Ein strengerer Linter folgt, sobald es genug Code gibt.
- Fehler werden mit Kontext weitergereicht: `fmt.Errorf("create snapshot %s: %w", name, err)`.
- Kein `panic` außer beim Start.
- Kontext (`context.Context`) ist immer das erste Argument.
- Logging strukturiert, ein Logger, gleiche Schlüsselnamen (`namespace`, `application`, `snapshot`, `job`).
- Dateien klein halten, ein Hauptthema pro Datei.
- Tests: Tabellentests, Dateiname `*_test.go` neben dem Code.

## Kubernetes-Objekte (CRDs)

- API-Gruppe: `restora.local` bis die Domain feststeht, dann einmalig ändern.
- Namen: `Policy`, `Snapshot`, `RestoreRequest`, `LocationProfile`, `Export`, `TransformSet`. Immer Singular, CamelCase.
- Jede Ressource, die einen Auftrag ausführt (`Snapshot`, `RestoreRequest`), hat `status.phase` mit denselben Werten: `Pending`, `Running`, `Succeeded`, `Failed`. Dauerhafte Konfiguration wie `Policy` hat keine Phase, sondern beschreibt ihren Zustand (`nextRunAt`, `lastRun`, bei Fehlern `errorCode`).
- Jede Ressource hat `status.conditions` und `status.message` (Klartext für die UI).
- Bei Fehlern zusätzlich `status.errorCode` (stabile Kennung wie `snapshot_class_missing`) und `status.hint` (was der Nutzer tun kann). Die UI übersetzt den Code in DE/EN/AR und zeigt den englischen Text nur unter "Technische Details".
- Eigene Ressourcen werden vom Controller mit einem Finalizer aufgeräumt (VolumeSnapshots, Dateien), nie vom API-Programm. Löschen heißt für API und Controller: die Ressource löschen. Wer eine Ressource mit Finalizer in Listen zeigt, blendet Einträge mit `deletionTimestamp` aus (`Deleting()`).
- Löschen, das nicht rückgängig zu machen ist, geht immer über den Dienst mit seinen Schutzprüfungen (`AddDeleteGuard`), nie am Dienst vorbei, auch nicht für automatische Aufräumläufe.
- CRDs liegen in `charts/restora/crds/`. Helm aktualisiert sie nicht, `hack/dev-install.sh` wendet sie bei jedem Lauf an.
- Aufgaben, die lange warten (Verbindungstests), laufen mit `reconcile.RunWorkers`, damit sie sich nicht gegenseitig aufhalten.
- Labels: `app.kubernetes.io/managed-by: restora`, `restora.local/application: <name>`.
- **Datenkopierer (Mover):** Alles, was Daten aus oder in einen Speicherort bewegt, läuft in kurzlebigen Pods mit dem Controller-Image (`mover <befehl>`), nie im Controller selbst. Der Pod meldet sein Ergebnis über die Terminierungsnachricht (bis 4 KB), größere Ausgaben über das Log zwischen Markern. Alle Pods tragen das Label `restora.local/mover` und werden darüber aufgeräumt. Fehler werden zu stabilen Codes (`Classify`), nie als rohe Kopia-Ausgabe in die Oberfläche gereicht.
- **Export-Einträge** (`Export`) sind Aufzeichnungen über eine Kopie im Speicherort. Sie gehören weder Snapshot noch Speicherort und überleben beide, damit eine Wiederherstellung auch ohne Cluster-Zustand geht.
- **Archivschlüssel:** `restora-repo-<speicherort>` wird einmal erzeugt, nie ersetzt, gehört niemandem und wird nur vom Controller gelesen. Wiederherstellen benutzt ihn nur (`Existing`), erzeugt nie einen neuen.

## REST-API

- Pfad: `/api/v1/<ressource-plural>` (`/api/v1/applications`, `/api/v1/snapshots`).
- Verben: `GET` lesen, `POST` anlegen, `PUT` ersetzen, `DELETE` löschen. Aktionen als Unterpfad: `POST /api/v1/applications/{id}/snapshots`.
- JSON-Felder in `camelCase`.
- Fehler immer in diesem Format:
  ```json
  { "error": { "code": "snapshot_class_missing", "message": "…", "hint": "…" } }
  ```
- Listen sind paginiert (`limit`, `cursor`). Handler lesen `limit` mit `server.ParseLimit`.
- Eingaben, die eine Ressource anlegen oder ändern, prüft der Dienst (Namen, Zeitplan, Auswahl) und antwortet mit `400` und eigenem Fehlercode. Unbekannte JSON-Felder werden abgelehnt. Wer vor dem Speichern wissen soll, was passiert, bekommt einen `POST …/preview` ohne Nebenwirkung.
- Zeitangaben nach ISO 8601, in UTC.

## UI (React, TypeScript)

- Ordner pro Funktion: `ui/src/features/<funktion>/` mit `api.ts`, `components/`, `pages/`.
- Gemeinsame Bauteile (zum Beispiel `Field`, `Checkbox`, `Dialog`) nur in `ui/src/components/`. Neue Bauteile erst, wenn kein vorhandenes passt.
- Alle Texte über Übersetzungsschlüssel: `feature.element.text` (zum Beispiel `snapshots.create.button`).
- Farben, Abstände, Schriftgrößen ausschließlich über Tokens aus [DESIGN.md](DESIGN.md).
- Logische CSS-Eigenschaften für Rechts-nach-links.
- Jede Seite behandelt leer, lädt, Fehler, normal.
- Zustand, der sich aus dem Cluster ableiten lässt, wird abgeleitet und nicht zusätzlich gespeichert (Beispiel: Fortschritt des Einrichtungsassistenten in `features/setup/progress.ts`). Im Browser liegen nur Vorlieben des Einzelnen (`localStorage`, immer mit `try/catch` und ohne Funktionsverlust, wenn es fehlt).
- Wiederkehrende Karten und Listen (`ExportList`, `Dialog`) liegen in `ui/src/components/`, damit Assistent und Detailseite dasselbe zeigen. Formulare, die auch eingebettet laufen, haben die Eigenschaft `embedded`.
- Formulare: Feld mit `Field` (Beschriftung oben, Hinweis und Fehler unten). Fehler erscheinen erst nach dem ersten Speichern-Versuch, danach springt der Fokus zur Zusammenfassung ganz oben. Eingaben sind Texte (`PolicyForm`), `toInput` baut daraus die API-Eingabe, Prüfungen ohne Server stehen in `findProblems`.
- Kein Farbverlauf. Prüfung: `grep -ri gradient ui/src ui/public ui/index.html` muss leer sein.
- Keine Inline-Styles (`style={{ … }}`) und keine Inline-Skripte. Die Sicherheitsrichtlinie (CSP) des Servers blockiert sie. Stattdessen CSS-Klassen und Dateien.
- Typprüfung (`npm run typecheck`) und Tests (`npm test`) müssen vor jedem Phasenabschluss grün sein.

## Benennung

| Ding | Form | Beispiel |
|---|---|---|
| Go-Paket | klein, ein Wort | `snapshot` |
| Go-Typ | CamelCase | `SnapshotService` |
| Datei (Go) | snake_case | `snapshot_service.go` |
| Datei (UI, Komponente) | PascalCase | `SnapshotList.tsx` |
| Datei (UI, sonst) | kebab-case | `use-snapshots.ts` |
| Übersetzungsschlüssel | punkt-getrennt | `snapshots.create.button` |
| Helm-Wert | camelCase | `controller.replicaCount` |

## Tests und Phasenabschluss

Jede Phase endet in dieser Reihenfolge:

1. Automatische Tests laufen durch (`go test ./...`, UI-Tests).
2. Manueller Test aus [PLAN.md](../PLAN.md) wird durchgespielt.
3. Prüfliste aus [DESIGN.md](DESIGN.md) wird abgehakt.
4. Claude nennt alle neuen und geänderten Dateien.
5. Der Nutzer committet selbst.

## Sicherheit

- Keine Zugangsdaten in Code, Logs, API-Antworten oder Test-Manifesten außer klar markierten Testwerten für den lokalen Cluster.
- API und Controller haben getrennte ServiceAccounts mit minimalen Rechten. Nur der Controller darf Secrets lesen, die API liest Secrets und ConfigMaps nur als Metadaten (Namen).
- Zugangsdaten liegen als Kubernetes-Secret, das der Ressource gehört (Owner-Referenz), damit Löschen es mit aufräumt. Die API darf Secrets nur anlegen und ersetzen, nie lesen, und gibt nie ein Secret zurück, nur ob eines gesetzt ist. Der Controller liest sie. Schlüssel kommen nicht in Ressourcen, Logs oder Fehlermeldungen.
- Entwicklung nur gegen `kubectl --context restora-dev`.
