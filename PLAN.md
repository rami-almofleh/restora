# Restora – Phasenplan

Self-hosted Backup und Restore für Kubernetes (Vorbild: Kasten K10, Ergänzungen aus Velero, Trilio, Portworx PX-Backup, CloudCasa, Stash, Commvault).
Läuft **im Cluster des Kunden**. Zugangsdaten und Daten bleiben beim Kunden.

## Spielregeln (gelten für jede Phase)

1. **Eine Funktion pro Phase.** Keine Phase baut zwei neue Dinge gleichzeitig.
2. **Jede Phase endet mit einem manuellen Test.** Die Schritte stehen in der Phase. Erst wenn sie klappen, ist die Phase fertig.
3. **Nichts wird von Claude committet.** Am Ende jeder Phase gibt es eine Liste der geänderten Dateien, du committest selbst.
4. **Konsistenter Code.** Regeln in [docs/CONVENTIONS.md](docs/CONVENTIONS.md).
5. **UI: modern, responsiv, viel Weißraum, keine Farbverläufe.** Regeln in [docs/DESIGN.md](docs/DESIGN.md).
6. **Test-Cluster:** nur `kubectl --context restora-dev`. Dein Standard-Kontext zeigt auf einen echten Cluster und wird nie angefasst.

## Architektur (Entwurf)

```
Kunden-Cluster
 └── Namespace "restora-system"
      ├── restora-controller  (Operator, Go)  – CRDs, Zeitpläne, Snapshots, Restore
      ├── restora-api         (REST, Go)      – Schnittstelle, liefert auch die Oberfläche aus
      │    └── Weboberfläche  (React, eingebettet; DE / EN / AR, RTL)
      └── Datenbank           (SQLite → Postgres, ab der Phase, die sie braucht)
Speicher des Kunden: S3 / NFS / Azure Blob / GCS
```

Installation per **Helm-Chart**: `helm install restora ./charts/restora`

## Übersicht

| Block | Phasen |
|---|---|
| A. Kern | 0 Fundament · 1 Entdecken · 2 Snapshot · 3 Restore · 4 Aktivität · 4a Anwendungszustand |
| B. Automatik | 5 Richtlinien · 6 Aufbewahrung |
| C. Externer Speicher | 7 Speicherorte · 8 Export · 9 Restore aus Export · 10 Einrichtungsassistent · 11 Zweite Kopie (3-2-1) |
| D. Wiederherstellung+ | 12 Vorschau und anderer Namespace · 13 Transformationen · 14 Datenbank-Hooks · 15 Cluster-Migration |
| E. Überblick | 16 Dashboard · 17 Berichte · 18 Benachrichtigungen · 19 Metriken |
| F. Zugriff und Sicherheit | 20 Login · 21 Rollen und Teams · 22 Audit-Log · 23 Unveränderbare Backups · 24 Schlüsselverwaltung |
| G. Betrieb | 25 Backup von Restora selbst · 26 Multi-Cluster · 27 GitOps · 28 Lizenz |
| H. Mehrwert gegenüber K10 | 29 Automatische Restore-Tests · 30 Anomalie-Erkennung · 31 Kosten und Prognose · 32 Compliance-Berichte · 33 Feingranulare Wiederherstellung · 34 Schutz vor Cluster-Upgrades · 35 KI-Assistent |

Hinweis zur Herkunft: Die Funktionsliste stammt aus Herstellerseiten und Vergleichsartikeln zu K10, Velero, Trilio, PX-Backup und CloudCasa. Die Punkte in Block H sind Lücken, die Nutzer laut Bewertungen und Vergleichen nennen (zum Beispiel einheitliche Multi-Cluster-Ansicht, Filter, eingebaute Fehler- und Speicherberichte) oder die Velero-Nutzer selbst bauen müssen (Restore-Validierung). Eine Marktprüfung mit echten Nutzern steht noch aus.

---

## Block A – Kern

### Phase 0 – Fundament und Oberfläche
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
Der Controller hat in dieser Phase nur Health-Endpunkte. Die CRDs (und damit "registriert die CRDs") kommen mit der ersten Funktion, die sie braucht.

**Neu:** Leere, schöne App-Hülle.
- Projektstruktur, Controller-Grundgerüst, API mit `/healthz`
- UI: Navigation (Seitenleiste am Desktop, untere Leiste am Handy), Hell/Dunkel, Sprachen DE/EN/AR mit RTL
- Helm-Chart, das alles im Test-Cluster installiert

**Manueller Test**
1. `helm install` im Cluster `restora-dev`.
2. Oberfläche per Port-Forward öffnen.
3. Fenster schmal ziehen: Navigation wechselt auf Handy-Layout.
4. Sprache auf Arabisch stellen: alles spiegelt sich nach rechts.
5. Dunkel-Modus einschalten: kein Element wird unlesbar.

### Phase 1 – Anwendungen entdecken
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Liste aller Anwendungen im Cluster. *(K10, PX-Backup)*
- Erkennung pro Namespace: Deployments, StatefulSets, PVCs, ConfigMaps, Secrets
- Detailseite pro Anwendung, Suche und Filter
- Umsetzung: eine Anwendung = ein Namespace mit Workloads oder Volumes (Systembereiche ausgeblendet). Secrets und ConfigMaps nur als Namen (Metadaten), nie Inhalte. Die Liste aktualisiert sich alle 10 Sekunden von selbst.

**Manueller Test**
1. `demo` erscheint in der Liste mit Postgres und 1 Volume.
2. Neuen Namespace mit Testpod anlegen: erscheint nach wenigen Sekunden.
3. Suche nach "demo" filtert die Liste.

### Phase 2 – Manueller Snapshot
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Button "Jetzt sichern". *(alle)*
- CSI VolumeSnapshot plus Export der Kubernetes-Ressourcen
- Fortschritt und Ergebnis sichtbar
- Umsetzung: Die API legt eine `Snapshot`-Ressource (`restora.local/v1alpha1`) in `restora-system` an, der Controller führt sie aus. Schritte: Ressourcen exportieren, Volumes sichern. Die VolumeSnapshotClass wird über den CSI-Treiber des Volumes gewählt.
- Die exportierten Ressourcen (inklusive Secrets) liegen vorerst **unverschlüsselt** auf einem kleinen Volume des Controllers. Verschlüsselung kommt mit Phase 8.
- Die Sicherung ist absturzkonsistent, nicht anwendungskonsistent. Anwendungskonsistenz bringt Phase 14.
- Controller und API haben getrennte ServiceAccounts. Nur der Controller darf Secrets lesen.
- Löschen von Sicherungen: ab Phase 6 in der Oberfläche. Per `kubectl --context restora-dev -n restora-system delete snapshots.restora.local --all` geht es weiter, der Controller räumt die VolumeSnapshots mit auf.

**Manueller Test**
1. Auf `demo` klicken, "Jetzt sichern".
2. Status wechselt auf "läuft", dann "fertig".
3. `kubectl --context restora-dev -n demo get volumesnapshot` zeigt den Snapshot.
4. `kubectl --context restora-dev -n restora-system get snapshots.restora.local` zeigt die Sicherung mit Phase `Succeeded`.

### Phase 3 – Wiederherstellung (selber Namespace)
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Button "Wiederherstellen". *(alle)*
- Wiederherstellung aus Snapshot im selben Namespace
- Sicherheitsabfrage mit klarer Zusammenfassung
- Umsetzung: Die API legt eine `RestoreRequest`-Ressource an, der Controller führt sie in fünf Schritten aus: Sicherung prüfen, Workloads stoppen, Volumes aus den VolumeSnapshots neu erstellen, gespeicherte Ressourcen anwenden, auf Start warten.
- **Zerstörend:** Die betroffenen Volumes werden gelöscht und aus der Sicherung neu erstellt. Alles, was seit der Sicherung geschrieben wurde, geht verloren. Die Sicherheitsabfrage sagt das ausdrücklich.
- Ressourcen aus der Sicherung überschreiben vorhandene mit gleichem Namen. Ressourcen, die erst nach der Sicherung entstanden sind, bleiben unangetastet.
- Läuft ein Restore, sind Sicherungen und weitere Restores für diese Anwendung gesperrt.
- Noch nicht enthalten: eine automatische Sicherheitskopie vor dem Restore, Restore in einen anderen oder gelöschten Namespace (Phase 9 und 12). Eine Anwendung, aus deren Namespace alles gelöscht wurde, erscheint nicht in der Liste (Phase 9 ändert das).
- Das Löschen einer Sicherung wird verweigert, solange ein Restore sie benutzt (Phase 6).

**Manueller Test**
1. In der Demo-DB eine Zeile eintragen, dann sichern.
2. Danach die Tabelle löschen.
3. Wiederherstellen: die Zeile ist wieder da.
4. Während der Wiederherstellung sind "Jetzt sichern" und weitere "Wiederherstellen"-Knöpfe gesperrt, und die Karte sagt warum.

### Phase 4 – Aktivität und verständliche Fehler
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Aktivitätsliste mit Live-Status. *(K10 Dashboard; Nutzer bemängeln unklare Fehler)*
- Jede Aktion ist ein Eintrag mit Dauer, Status, Schritten
- Fehler in einfachen Worten mit Lösungsvorschlag
- Umsetzung: Seite "Aktivität" mit allen Sicherungen und Wiederherstellungen aller Anwendungen in einer Zeitleiste, neueste zuerst. Filter nach Art, Status (mit Zahlen) und Anwendung. Läuft etwas, aktualisiert sich die Liste alle 2 Sekunden, sonst alle 10. Fehlgeschlagene Einträge zeigen Schritte, Klartext, Lösungsvorschlag und eingeklappt die technischen Details. Neuer Endpunkt `GET /api/v1/activities?type=&status=&q=&limit=&cursor=`.
- Es gibt noch keine eigene Aktivitäts-Ressource: die Liste wird aus den vorhandenen `Snapshot`- und `RestoreRequest`-Ressourcen gebildet. Erst wenn Richtlinien (Phase 5) weitere Aktionsarten bringen, wird das neu bewertet.
- Aufräumen nebenbei: gemeinsame Bausteine für Schritte/Fehler/Hinweise (`JobDetails`, `WarningList`), `server.ParseLimit`, CSS-Klassen `job-*` statt `snapshot-*`.

**Manueller Test**
1. Snapshot mit nicht vorhandener VolumeSnapshotClass auslösen: Klasse löschen (`kubectl --context restora-dev delete volumesnapshotclass csi-hostpath-snapclass`), in der Oberfläche "Jetzt sichern", danach die Klasse wieder anlegen (`kubectl --context restora-dev apply -f hack/volumesnapshotclass.yaml`).
2. Seite "Aktivität" öffnen: Eintrag zeigt Fehler im Klartext plus Lösungsvorschlag.
3. Seite bleibt am Handy lesbar.

### Phase 4a – Anwendungszustand (nur lesen)
**Neu:** Auf der Detailseite einer Anwendung: Pods mit Status und Neustarts, Events, Logs. *(Hilfe bei der Fehlersuche nach Backup und Restore)*
- Nur lesend. Logs sind zunächst ausgeblendet und werden bewusst geöffnet, begrenzt auf die letzten Zeilen.
- **Bewusst nicht Teil von Restora:** Ressourcen bearbeiten oder konfigurieren, Terminal im Pod, allgemeine Cluster-Verwaltung. Dafür gibt es Lens, Headlamp und k9s.
- Vor dem Bauen mit zwei oder drei Kunden klären, ob sie diese Sicht wirklich in Restora wollen.

**Manueller Test**
1. Demo-DB stoppen (`kubectl --context restora-dev -n demo scale deploy/demo-db --replicas=0`): Anwendung zeigt "Gestoppt".
2. Wieder starten und kaputtes Image setzen: Pod-Status und Event nennen den Fehler im Klartext.
3. Logs öffnen: letzte Zeilen erscheinen, sonst sind sie verborgen.

## Block B – Automatik

### Phase 5 – Richtlinien und Zeitpläne
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Policies mit Zeitplan. *(K10, Velero, alle)*
- Auswahl per Namespace oder Label
- Zeitplan: stündlich, täglich, wöchentlich, eigene Cron-Angabe, mit Vorschau der nächsten Läufe
- Umsetzung: neue Ressource `Policy` (`restora.local/v1alpha1`). Die Seite "Richtlinien" legt an, bearbeitet, pausiert und löscht sie. Der Controller prüft jede Policy zur fälligen Zeit und legt für jede ausgewählte Anwendung eine `Snapshot`-Ressource an (mit Label `restora.local/policy` und `spec.policy`, die Aktivität zeigt "Richtlinie: Name"). Endpunkte: `/api/v1/policies` (GET, POST), `/{id}` (GET, PUT, DELETE), `/{id}/pause`, `/{id}/resume`, `/preview`.
- Auswahl: bestimmte Anwendungen, Namespace-Label (`schlüssel=wert`, mehrere müssen alle passen) oder alle Anwendungen. Es zählen nur Anwendungen im Sinne von Phase 1 (Systembereiche und leere Namespaces nicht). Die Vorschau zeigt vor dem Speichern, auf welche Anwendungen es passt und wann die nächsten fünf Läufe sind.
- Zeitplan mit Zeitzone (Standard: die des Browsers). Fünf-Felder-Cron, keine Kürzel wie `@daily`.
- Eine verpasste Zeit (Controller war aus) wird nicht nachgeholt: die Richtlinie läuft einmal sofort und danach nach Plan. Ändern, Pausieren und Fortsetzen starten die Zählung neu ab jetzt.
- Läuft für eine Anwendung gerade eine Sicherung oder Wiederherstellung, wird sie in diesem Lauf übersprungen. Der Grund steht in der Karte ("Übersprungen").
- Eine Policy hat kein `status.phase` (sie ist kein Auftrag), sondern `nextRunAt`, `lastRun` und bei Fehlern `errorCode`/`hint`. Von Hand mit kaputtem Zeitplan angelegt, meldet sie `invalid_schedule`.
- Ohne Aufbewahrungsregeln (Phase 6) bleibt jede Sicherung samt VolumeSnapshot liegen. Eine Policy "jede Minute" ist nur zum Testen gedacht.
- Löschen einer Richtlinie lässt ihre Sicherungen bestehen.
- Der Controller darf jetzt Namespaces listen und Snapshots anlegen.

**Manueller Test**
1. Unter "Richtlinien" eine Richtlinie anlegen: Name "Jede Minute", Anwendung `demo`, Zeitplan "Eigene Angabe" mit `* * * * *`. Die Vorschau zeigt die nächsten Läufe.
2. Nach 3 Minuten gibt es 3 Snapshots (Seite "Aktivität", Zeilen mit "Richtlinie: Jede Minute").
3. Policy pausieren: keine weiteren Läufe, "Nächster Lauf" zeigt "Pausiert".
4. Fortsetzen: der nächste Lauf kommt zur nächsten vollen Minute, die verpassten werden nicht nachgeholt.
5. Aufräumen: Richtlinie löschen, Sicherungen mit `kubectl --context restora-dev -n restora-system delete snapshots.restora.local --all`.

### Phase 6 – Aufbewahrung
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Aufbewahrungsregeln (GFS: Stunden, Tage, Wochen, Monate, Jahre). *(K10)*
- Umsetzung: Jede Richtlinie hat optional Regeln (`spec.retention`): "die letzten N" sowie N stündliche, tägliche, wöchentliche, monatliche und jährliche. Jede Zahl behält die neueste Sicherung der letzten N Stunden, Tage, Wochen, Monate oder Jahre, in denen es überhaupt eine Sicherung gab (wie bei restic). Eine Sicherung bleibt, wenn eine Regel sie behält, alles andere wird gelöscht. Alle Zahlen 0 heißt: nichts wird je gelöscht (so sind bestehende Richtlinien).
- Die Regeln gelten pro Richtlinie und pro Anwendung. Sicherungen einer Richtlinie tragen das Label `restora.local/policy`. Manuelle Sicherungen (ohne Label) werden nie automatisch gelöscht.
- Tage, Wochen, Monate und Jahre zählen in der Zeitzone der Richtlinie (Woche ab Montag), Stunden in UTC.
- Der Controller wendet die Regeln bei jedem Blick auf die Richtlinie an (mindestens jede Minute), nicht bei pausierten Richtlinien. Läuft für eine Anwendung gerade eine Sicherung, wird dort nichts gelöscht. Eine Sicherung, die gerade nicht gelöscht werden darf (zum Beispiel weil eine Wiederherstellung sie liest), bleibt bis zum nächsten Durchgang.
- Fehlgeschlagene Sicherungen einer Richtlinie sind keine Wiederherstellungspunkte, bleiben aber 7 Tage sichtbar und werden dann mit aufgeräumt (nur wenn Regeln gesetzt sind).
- **Vorschau vor dem Löschen:** Beim Bearbeiten einer Richtlinie zeigt die Seite sofort, welche vorhandenen Sicherungen die neuen Regeln löschen würden (`POST /api/v1/policies/{id}/retention-preview`). Beim Speichern fragt ein Dialog nach und nennt die Sicherungen. Gelöscht wird erst nach dem Speichern.
- **Einzelne Sicherung löschen:** Auf der Anwendungsseite gibt es "Löschen" mit Sicherheitsabfrage (`DELETE /api/v1/snapshots/{id}`). Der Controller räumt VolumeSnapshots und gespeicherte Dateien mit auf. Eine laufende Sicherung und eine Sicherung, von der gerade eine Wiederherstellung liest, lassen sich nicht löschen. Eine Sicherung, die gerade gelöscht wird, verschwindet sofort aus den Listen und lässt sich nicht mehr wiederherstellen.
- Wird eine Richtlinie gelöscht, bleiben ihre Sicherungen bestehen und werden nicht mehr aufgeräumt.
- Der Controller darf jetzt Snapshots löschen, die API ebenfalls.

**Manueller Test**
1. Richtlinie "Jede Minute" für `demo` anlegen (Eigene Angabe `* * * * *`), Aufbewahrung zunächst "Alle behalten".
2. Nach 4 Minuten gibt es 4 Sicherungen.
3. Richtlinie bearbeiten, Aufbewahrung "Nach Regeln", "Die letzten" = 2: unter den Feldern steht, welche 2 gelöscht würden. Speichern, im Dialog bestätigen.
4. Kurz darauf existieren nur die 2 neuesten (`kubectl --context restora-dev -n restora-system get snapshots.restora.local` und `kubectl --context restora-dev -n demo get volumesnapshot`). Danach läuft jede Minute eine neue, und die älteste verschwindet.
5. Einzeln löschen: auf der Anwendungsseite bei einer Sicherung "Löschen" wählen und bestätigen. Während einer Wiederherstellung, die von ihr liest, kommt stattdessen eine Meldung.
6. Aufräumen: Richtlinie löschen, übrige Sicherungen über die Oberfläche löschen.

## Block C – Externer Speicher

### Phase 7 – Speicherorte (Location Profiles)
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Speicherort anlegen und testen. *(K10, Velero, alle)*
- S3-kompatibel und NFS
- Zugangsdaten nur als Kubernetes-Secret, nie im Klartext in der UI
- Button "Verbindung testen"
- Umsetzung: neue Ressource `LocationProfile` (`restora.local/v1alpha1`). Die Seite "Speicherorte" legt an, bearbeitet, testet und löscht. Endpunkte: `/api/v1/locations` (GET, POST), `/{id}` (GET, PUT, DELETE), `/{id}/test` (POST).
- **Zugangsdaten:** Access Key und Secret Key tippst du einmal ein. Die API legt daraus ein Secret `restora-location-<id>` an (der Speicherort ist Eigentümer, Löschen räumt das Secret mit auf) und schreibt keine Schlüssel in die `LocationProfile`. Die API kann Secrets nur anlegen und ersetzen, nie lesen, und gibt nie einen Schlüssel zurück, nur `credentialsSet`. Beim Bearbeiten bleiben die Felder leer, dann bleiben die gespeicherten Schlüssel. Nur der Controller liest das Secret.
- **Test (macht der Controller):** Die API vermerkt nur den Testwunsch (`spec.testRequestedAt`) beim Anlegen, Ändern und auf Knopfdruck. Der Controller testet und schreibt das Ergebnis in `status.lastTest`: Schritte, stabiler Fehlercode, Text und Lösungsvorschlag (in der Oberfläche in DE/EN/AR). Bis dahin zeigt der Speicherort "Wird getestet".
  - S3: Bucket auflisten, Testobjekt schreiben, lesen, löschen. Erkannt werden falsche Schlüssel, fehlende Rechte, fehlendes Bucket, nicht erreichbare Adresse, Zertifikatsfehler, falsches Protokoll (http statt https) und Zeitüberschreitung. Region optional, "Bucket im Pfad ansprechen" (für MinIO), Zertifikat nicht prüfen (nur für Tests, mit Warnung).
  - NFS: Der Controller startet kurz einen Pod mit der Freigabe (der Kubelet bindet sie ein, Restora braucht keine Rechte auf den Knoten). Der Pod läuft mit dem Controller-Image als Benutzer 65532, schreibt, liest und löscht eine Testdatei und wird danach entfernt. Der Benutzer 65532 muss auf der Freigabe schreiben dürfen. Ein frisch gestarteter NFS-Server hält Schreibzugriffe zunächst zurück, der Test wartet deshalb bis zu 90 Sekunden.
- Der Typ eines Speicherorts lässt sich nicht ändern, dafür einen neuen anlegen.
- Mehrere Speicherorte werden gleichzeitig getestet (vier Tests parallel), damit ein langsamer NFS-Test die anderen nicht aufhält.
- **Wirkung:** Ab Phase 8 landen Sicherungen dort. Ein Speicherort lässt sich nicht löschen, solange eine Richtlinie ihn nutzt oder Kopien darin liegen.
- **Sicherheit:** Bis zum Login (Phase 20) kann jeder, der die Oberfläche erreicht, den Controller auf eine beliebige Adresse zeigen lassen. Die Oberfläche gehört bis dahin nicht ins Netz.
- Testumgebung: `hack/minio.yaml` (MinIO, Zugangsdaten stehen in der Datei) und `hack/nfs-server.yaml` (NFS-Server). MinIO veröffentlicht keine Images mehr auf Docker Hub und Quay, die Testumgebung nutzt deshalb das Chainguard-Image.

**Manueller Test**
1. MinIO im Test-Cluster starten:
   `kubectl --context restora-dev apply -f hack/minio.yaml`
2. Unter "Speicherorte" anlegen: Name "MinIO", Adresse `http://minio.minio:9000`, Bucket `restora`, Access Key `restora-test`, Secret Key `restora-test-secret`. Nach dem Speichern wird der Test grün: vier Schritte, alle erledigt.
3. Falsches Passwort: den Speicherort bearbeiten und beim Secret Key etwas anderes eintragen (beide Schlüsselfelder ausfüllen). Der Test wird rot: "Der Speicher hat den Access Key oder den Secret Key abgelehnt", mit Lösungsvorschlag. Mit dem richtigen Passwort wieder grün.
4. Weitere Fehler zum Ausprobieren: falsches Bucket (`bucket_not_found`), Adresse ohne `http://` (wird als https gelesen: Hinweis auf das Protokoll), falsche Adresse.
5. NFS (optional): `kubectl --context restora-dev apply -f hack/nfs-server.yaml`, dann die IP des Dienstes mit dem Befehl aus der Datei holen und als Server eintragen, Pfad `/`.
6. Die Schlüssel sind nirgends lesbar: `kubectl --context restora-dev -n restora-system get locationprofiles.restora.local -o yaml` enthält keine, die Oberfläche zeigt nur "Gespeichert (nicht sichtbar)".
7. Aufräumen: Speicherorte in der Oberfläche löschen.

### Phase 8 – Export auf externen Speicher
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Backup landet verschlüsselt im Speicherort. *(K10 nutzt Kopia)*
- Verschlüsselt, dedupliziert, inkrementell
- Umsetzung: [Kopia](https://kopia.io) 0.23.1 (statisches Binary im Controller-Image) legt pro Speicherort ein Archiv an, auf S3 oder NFS. Kopiert wird in kurzlebigen Pods, die das Controller-Image mit `mover backup` starten. Pods laufen nur während der Kopie und werden danach über das Label `restora.local/mover` entfernt.
- **Richtlinie:** Jede Richtlinie hat das Feld "Speicherorte" (`spec.exportTo`, Liste von Speicherorten). Manuelle Sicherungen kopieren nicht. Neuer Endpunkt `POST /api/v1/policies/{id}/run` startet eine Richtlinie sofort, auch der Assistent nutzt ihn.
- **Ablauf:** Die lokale Sicherung wird zuerst fertig und bleibt gültig. Danach kommt der Schritt "In Speicherorte kopieren": erst die Ressourcen (als Paket mit Manifest, bis 900 KiB), dann jedes Volume aus dem VolumeSnapshot, schreibgeschützt in einem temporären Volume neben der Anwendung. Schlägt eine Kopie fehl, bleibt die Sicherung "Fertig", die Kopie zeigt ihren eigenen Fehler (`status.exports[]`) mit Klartext und Lösungsvorschlag. Pro Speicherort und Volume wird nacheinander kopiert, verschiedene Speicherorte laufen parallel.
- **Schlüssel:** Beim ersten Kopieren erzeugt Restora ein Passwort und legt es als Secret `restora-repo-<speicherort>` ab. Es wird nie ersetzt, nie von der API gelesen und gehört nicht dem Speicherort. **Sichere dieses Secret außerhalb des Clusters**: ohne den Schlüssel sind die Daten im Speicher unlesbar. Liegt am Ziel schon ein Archiv mit anderem Schlüssel, bricht die Kopie mit `repository_password_invalid` ab, statt etwas zu überschreiben. Dann hilft ein leerer Ordner im Bucket (Feld "Ordner im Bucket").
- **Export-Eintrag:** Pro Kopie schreibt der Controller eine Ressource `Export` (`restora.local/v1alpha1`). Sie überlebt Snapshot und Namespace und enthält Zeitpunkt, Speicherort, Volumes, Größen und die IDs im Archiv. Lesen: `GET /api/v1/exports`, `/{id}`, `/api/v1/export-applications`.
- **Speicherort löschen:** wird verweigert (`location_in_use`, 409), solange eine Richtlinie ihn nutzt oder Kopien darin liegen.
- **Oberfläche:** Richtlinien-Formular mit Auswahl der Speicherorte, Karte "Kopien in Speicherorten" in der Aktivität (Fortschritt "Volumes kopiert: 1 von 2", neu im Speicher), Zähler "Kopien" in der Speicherort-Liste.
- Testumgebung: `hack/volumesnapshotclass.yaml`, `hack/minio.yaml`, `hack/nfs-server.yaml`.

**Manueller Test**
1. Speicherort "MinIO" wie in Phase 7 anlegen (Test grün). Unter "Richtlinien" eine Richtlinie für `demo` anlegen oder bearbeiten und bei "Speicherorte" MinIO wählen. Auf der Karte "Jetzt ausführen".
2. Unter "Aktivität" läuft die Sicherung, danach "In Speicherorte kopieren". Am Ende zeigt die Karte "Kopien in Speicherorten": MinIO, Fertig, Volumes und "neu im Speicher".
3. Objekte erscheinen im Bucket und sind nicht lesbar: `kubectl --context restora-dev -n minio exec deploy/minio -- ls -R /data/restora | head` zeigt nur Kopia-Blöcke (`p…`, `q…`, `kopia.repository`), keine Dateinamen oder Klartext.
4. Die Richtlinie noch einmal ausführen, ohne Daten zu ändern: die zweite Kopie dauert kürzer und "neu im Speicher" ist nur wenige KiB groß.
5. Fehlerfall: `kubectl --context restora-dev -n restora-system delete secret restora-repo-<speicherort>` und erneut ausführen. Die Sicherung bleibt "Fertig", die Kopie zeigt "Der Speicherort ist mit einem anderen Schlüssel verschlüsselt …" mit Lösungsvorschlag.
6. Den Speicherort löschen: wird mit Hinweis auf die Kopien verweigert.

### Phase 9 – Wiederherstellung aus dem Export
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Restore, wenn der lokale Snapshot weg ist. *(K10, Trilio)*
- Umsetzung: Jede gespeicherte Kopie ist ein `Export` und lässt sich wiederherstellen, auch wenn Snapshot, VolumeSnapshots und Namespace weg sind. Endpunkt `POST /api/v1/exports/{id}/restores`, der Ablauf nutzt dieselbe `RestoreRequest` wie Phase 3 (`spec.export`).
- **Ablauf:** Der Controller holt das Ressourcenpaket aus dem Speicherort (Pod `mover fetch-bundle`), legt den Namespace aus dem Manifest neu an, wendet die Ressourcen an und erzeugt für jedes Volume einen leeren Claim mit Annotation `restora.local/restored-from=export:<id>`. Je Volume füllt ein Pod (`mover restore`, root) den Claim aus dem Archiv. Danach werden die gestoppten Programme wieder gestartet. Zeitlimit 6 Stunden (Phase 3: 15 Minuten).
- **Schlüssel:** Es wird nur der vorhandene Schlüssel `restora-repo-<speicherort>` benutzt, nie ein neuer erzeugt. Fehlt er, scheitert der Restore mit Klartext.
- **Oberfläche:** Auf der Detailseite einer Anwendung liegen die Kopien in der Karte "Sicherungen in Speicherorten", jede mit "Aus Speicherort wiederherstellen" und dem gleichen Bestätigungsdialog wie in Phase 3 (Volumes, Ressourcen, Dauer, Ausfall). Unter "Anwendungen" erscheint die Karte "Gelöscht, aber noch im Speicherort": Anwendungen, die es im Cluster nicht mehr gibt, aber Kopien haben. Ihre Detailseite trägt das Abzeichen "Gelöscht" und bietet die Wiederherstellung an.
- **Grenzen:** Kopien werden bisher nicht automatisch aufgeräumt (Aufbewahrung gilt nur für lokale Sicherungen). Ein Restore in einen anderen Namespace kommt mit Phase 12.

**Manueller Test**
1. Nach Phase 8 ist eine Kopie von `demo` im Speicherort. Schreibe vorher Testdaten, z. B. in der Datenbank von `demo`.
2. Totalausfall: Snapshots und Namespace löschen:
   `kubectl --context restora-dev delete snapshots.restora.local --all -n restora-system` und `kubectl --context restora-dev delete namespace demo`.
3. Unter "Anwendungen" erscheint "Gelöscht, aber noch im Speicherort" mit `demo`. Auf "Öffnen" klicken, dort "Aus Speicherort wiederherstellen", Dialog bestätigen.
4. In der Aktivität läuft die Wiederherstellung (Namespace, Ressourcen, Volumes, Programme starten) und endet mit "Fertig".
5. Die Daten sind wieder vollständig: `kubectl --context restora-dev -n demo get pods,pvc` und der Inhalt des Volumes.

### Phase 10 – Einrichtungsassistent
**Status:** umgesetzt, wartet auf deinen manuellen Test und deinen Commit.
**Neu:** Erststart in 4 Schritten: Speicherort, erste Policy, Probelauf, Testrestore. *(K10 wird für seine einfache Einrichtung gelobt)*
- Umsetzung: Seite `/setup`. Es gibt keinen eigenen Zustand im Cluster: der Fortschritt wird bei jedem Öffnen aus den Fakten abgeleitet (Speicherort mit grünem Test? Richtlinie mit Kopie? Sicherung mit Kopie erfolgreich? Wiederherstellung aus dem Speicherort erfolgreich?) und alle 3 Sekunden aktualisiert. Nur "gesehen", "ausgeblendet" und "Wiederherstellungstest übersprungen" liegen im Browser (`restora.setup.*` in localStorage).
- **Schritt 1 – Speicherort:** das Speicherort-Formular (S3 oder NFS) eingebettet, mit Test. Weiter geht es erst bei grünem Test.
- **Schritt 2 – Richtlinie:** das Richtlinien-Formular mit Vorgaben, die Kopie in den neuen Speicherort ist vorausgewählt. Der Assistent geht automatisch weiter, sobald die Richtlinie gespeichert ist.
- **Schritt 3 – Probelauf:** startet die Richtlinie einmal (`POST /policies/{id}/run`), zeigt Schritte und Fehler live und kopiert in den Speicherort. Bei einem Fehler zeigt der Schritt Klartext und Lösungsvorschlag, nach der Korrektur (z. B. unter "Speicherorte") hilft "Erneut versuchen".
- **Schritt 4 – Wiederherstellungstest:** holt eine Anwendung aus dem Speicherort zurück, mit klarer Warnung, dass die Volumes ersetzt werden, und dem Bestätigungsdialog. Der Schritt lässt sich überspringen.
- **Erststart:** Ist die Installation frisch (keine Speicherorte, Richtlinien, Sicherungen), leitet die Übersicht einmalig auf den Assistenten. "Später fortsetzen" bringt zurück; auf der Übersicht erinnert ein Banner ("Einrichtung fortsetzen"), das sich mit "Nicht mehr anzeigen" ausblenden lässt. Unter "Einstellungen" öffnet "Assistent öffnen" den Assistenten wieder.
- Die Oberfläche ist für alle Breiten, DE/EN/AR (RTL) und Hell/Dunkel gebaut.

**Manueller Test**
1. Frische Installation (keine Speicherorte, Richtlinien, Sicherungen): Beim Öffnen der Übersicht landest du im Assistenten.
2. Mit dem Assistenten kommt man ohne Doku bis zum ersten erfolgreichen Backup: MinIO eintragen (Test grün), Richtlinie speichern, Probelauf starten.
3. Abbrechen und später fortsetzen: "Später fortsetzen", Seite neu laden, Banner auf der Übersicht, der Assistent macht beim nächsten offenen Schritt weiter.
4. Wiederherstellungstest durchführen oder überspringen, am Ende steht "Alles bereit".
5. Fehlerfall: ein Speicherort mit Archiv eines anderen Schlüssels (Ordner im Bucket mit altem Archiv, Secret `restora-repo-…` gelöscht): der Probelauf zeigt die Meldung verständlich, mit Lösungsvorschlag.

### Phase 11 – Zweite Kopie (3-2-1)
**Neu:** Backup wird auf einen zweiten Speicherort kopiert.

**Manueller Test**
1. Zwei MinIO-Buckets anlegen.
2. Policy mit beiden Zielen.
3. Ersten Bucket leeren: Restore läuft vom zweiten.

## Block D – Wiederherstellung+

### Phase 12 – Vorschau und anderer Namespace
**Neu:** Restore in neuen Namespace, davor Vorschau der Änderungen. *(K10)*

**Manueller Test**
1. Restore von `demo` nach `demo-copy`.
2. Vorschau listet alle Ressourcen mit Status "neu".
3. Beide Namespaces laufen parallel.

### Phase 13 – Transformationen
**Neu:** Ressourcen beim Restore ändern (StorageClass, Labels, Replikate, Image-Tags). *(K10)*

**Manueller Test**
1. Restore mit "StorageClass von csi-hostpath-sc nach standard".
2. PVC im Ziel nutzt `standard`.
3. Vorschau zeigt die Änderung vorher an.

### Phase 14 – Datenbank-Hooks
**Neu:** Anwendungskonsistente Sicherung mit Vorlagen für Postgres, MySQL, MongoDB. *(K10 Blueprints, Stash)*

**Manueller Test**
1. Hook für Postgres wählen (Vorlage, kein YAML nötig).
2. Während einer Schreiblast sichern.
3. Restore: Datenbank startet ohne Wiederherstellungsfehler.

### Phase 15 – Cluster-Migration
**Neu:** Anwendung in einen anderen Cluster verschieben. *(K10 Application Mobility, Trilio, CloudCasa)*

**Manueller Test**
1. Zweiten minikube-Cluster `restora-dev2` starten.
2. Cluster verbinden, Speicherort teilen.
3. `demo` im zweiten Cluster wiederherstellen.

## Block E – Überblick

### Phase 16 – Dashboard
**Neu:** Startseite mit Gesundheit auf einen Blick: ungeschützte Anwendungen, letzte Fehler, Speicherverbrauch. *(K10)*

**Manueller Test**
1. Neue Anwendung ohne Policy: sie steht als "ungeschützt" oben.
2. Policy zuweisen: Eintrag verschwindet.
3. Auf dem Handy bleibt jede Karte lesbar.

### Phase 17 – Berichte
**Neu:** Berichte als Seite und als PDF. *(K10)*

**Manueller Test**
1. Wochenbericht erzeugen.
2. PDF herunterladen: Erfolgsquote und Speicherverbrauch stimmen mit der Aktivitätsliste überein.

### Phase 18 – Benachrichtigungen
**Neu:** Meldungen bei Erfolg und Fehler: E-Mail, Slack, Teams, Telegram, Webhook.

**Manueller Test**
1. Kanal anlegen, "Testnachricht senden".
2. Einen Snapshot absichtlich fehlschlagen lassen: Nachricht kommt an und enthält den Klartext-Fehler.

### Phase 19 – Metriken
**Neu:** Prometheus-Metriken und fertiges Grafana-Dashboard. *(K10)*

**Manueller Test**
1. `/metrics` aufrufen: Zähler für Backups, Fehler, Dauer sind da.
2. Nach einem weiteren Backup steigt der Zähler.

## Block F – Zugriff und Sicherheit

### Phase 20 – Login
**Neu:** Anmeldung lokal und per OIDC/LDAP. *(K10)*

**Manueller Test**
1. Ohne Login: Weiterleitung auf Anmeldeseite.
2. Falsches Passwort: klare Meldung, keine Details über vorhandene Konten.
3. Abmelden beendet die Sitzung.

### Phase 21 – Rollen und Teams
**Neu:** Rollen (Admin, Operator, Betrachter) und Teams mit eigenen Namespaces. *(K10 Multi-Tenancy)*

**Manueller Test**
1. Betrachter anlegen: Buttons zum Sichern und Löschen fehlen.
2. Team-Nutzer sieht nur Anwendungen der eigenen Namespaces.

### Phase 22 – Audit-Log
**Neu:** Wer hat wann was getan, nicht änderbar und exportierbar.

**Manueller Test**
1. Policy löschen, danach Audit-Log öffnen: Eintrag mit Nutzer, Zeit, Aktion.
2. Export als CSV.

### Phase 23 – Unveränderbare Backups
**Neu:** Object Lock gegen Löschen und Ransomware. *(K10, Commvault, Trilio)*

**Manueller Test**
1. Bucket mit Object Lock, Aufbewahrung 1 Tag.
2. Backup im Bucket manuell löschen: wird abgelehnt.

### Phase 24 – Schlüsselverwaltung
**Neu:** Verschlüsselungsschlüssel anzeigen, wechseln, sichern.

**Manueller Test**
1. Schlüssel wechseln, neues Backup läuft.
2. Alte Backups sind weiter wiederherstellbar.

## Block G – Betrieb

### Phase 25 – Backup von Restora selbst
**Neu:** Konfiguration (Policies, Speicherorte, Rollen) sichern und wiederherstellen. *(K10 DR)*

**Manueller Test**
1. Restora-Namespace löschen.
2. Neu installieren, Konfiguration aus dem Backup laden.
3. Policies und Speicherorte sind wieder da.

### Phase 26 – Multi-Cluster-Ansicht
**Neu:** Mehrere Cluster in einer Oberfläche mit einheitlicher Sicht auf Jobs, Policies und Warnungen. *(K10; Nutzer wünschen sich genau diese Ansicht)*

**Manueller Test**
1. Zweiten Cluster verbinden.
2. Ein fehlgeschlagener Job im zweiten Cluster erscheint in der gemeinsamen Liste.
3. Filter nach Cluster funktioniert.

### Phase 27 – GitOps
**Neu:** Policies als YAML exportieren und per Git/ArgoCD verwalten; die UI zeigt den Stand an. *(K10)*

**Manueller Test**
1. Policy in der UI anlegen, YAML exportieren.
2. Policy löschen, YAML anwenden: Policy ist zurück.

### Phase 28 – Lizenzverwaltung
**Neu:** Offline-Lizenzschlüssel, Ablaufwarnung, Limits (zum Beispiel Nodes oder Anwendungen).

**Manueller Test**
1. Ohne Lizenz: Testmodus mit sichtbarem Limit.
2. Gültigen Schlüssel eingeben: Limit fällt weg.
3. Abgelaufener Schlüssel: verständliche Warnung, bestehende Backups bleiben wiederherstellbar.

## Block H – Mehrwert gegenüber K10

### Phase 29 – Automatische Restore-Tests
**Neu:** Das Backup wird regelmäßig in einer Sandbox wiederhergestellt und geprüft; Ergebnis als Bericht. *(bei Velero muss man das selbst bauen)*

**Manueller Test**
1. Restore-Test für `demo` planen.
2. Sandbox-Namespace entsteht, Postgres startet, Prüfabfrage läuft.
3. Bericht zeigt "bestanden", Sandbox wird aufgeräumt.
4. Kaputtes Backup absichtlich erzeugen: Bericht zeigt "fehlgeschlagen".

### Phase 30 – Anomalie-Erkennung
**Neu:** Warnung bei auffälliger Backup-Größe oder Änderungsrate (Hinweis auf Ransomware).

**Manueller Test**
1. Normale Backups laufen lassen.
2. Viele Daten in der Demo-DB überschreiben, dann sichern.
3. Warnung erscheint mit Begründung.

### Phase 31 – Kosten und Prognose
**Neu:** Speicherverbrauch und Kosten pro Anwendung, Wachstumsprognose.

**Manueller Test**
1. Preis pro GB eintragen.
2. Anwendungen sind nach Kosten sortierbar.
3. Prognose zeigt Trend für die nächsten 30 Tage.

### Phase 32 – Compliance-Berichte
**Neu:** Berichte für ISO 27001, DSGVO, SOC 2: Aufbewahrung, Verschlüsselung, Zugriffe, Restore-Tests.

**Manueller Test**
1. Bericht erzeugen.
2. Jede Aussage verweist auf einen echten Eintrag (Policy, Audit, Test).

### Phase 33 – Feingranulare Wiederherstellung
**Neu:** Einzelne Dateien oder Tabellen wiederherstellen. *(Commvault nennt granulare Restores)*

**Manueller Test**
1. Eine Datei im Volume löschen.
2. Nur diese Datei aus dem Backup holen, der Rest bleibt unverändert.

### Phase 34 – Schutz vor Cluster-Upgrades
**Neu:** Automatisches Backup vor Upgrade-Fenstern, mit Ein-Klick-Rückweg.

**Manueller Test**
1. Wartungsfenster anlegen.
2. Kurz davor läuft ein Backup, Aktivitätsliste zeigt "Vor Upgrade".

### Phase 35 – KI-Assistent
**Neu:** Fragen in normalen Worten ("Warum ist das Backup gestern fehlgeschlagen?", "Stelle `demo` wie gestern 15 Uhr wieder her").
- Der Assistent schlägt vor, der Mensch bestätigt. Er führt nichts selbständig aus.

**Manueller Test**
1. Frage zu einem Fehler stellen: Antwort nennt den echten Eintrag und die Ursache.
2. Restore-Wunsch äußern: Assistent zeigt Vorschau, es passiert erst nach deiner Bestätigung.

---

## Offene Entscheidungen
- Lizenzmodell: pro Node, pro Cluster oder pro Anwendung
- Marke: Prüfung bei DPMA / EUIPO vor dem Verkauf
- Go-Modulpfad (hängt von GitHub-Organisation und Domain ab)
- Datenbank: SQLite für Start, Postgres für große Kunden
