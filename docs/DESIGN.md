# Restora – Design-Regeln

Ziel: modern, ruhig, sofort verständlich. Auf dem Handy genauso gut bedienbar wie am Desktop.

## Harte Regeln

1. **Keine Farbverläufe.** Nirgends: nicht im Hintergrund, nicht auf Buttons, nicht in Diagrammen, nicht in Skeleton-Ladeanimationen. Nur flache Vollfarben.
2. **Viel Freifläche.** Lieber weniger pro Bildschirm als gedrängt. Abstände folgen einem 8-Pixel-Raster (8, 16, 24, 32, 48, 64).
3. **Mobile first.** Erst das schmale Layout bauen, dann erweitern. Kein horizontales Scrollen der Seite.
4. **Jede Seite hat vier Zustände:** leer, lädt, Fehler, normal. Der leere Zustand erklärt, was als Nächstes zu tun ist, und hat einen Button dafür.
5. **Keine Farbe allein als Information.** Status immer zusätzlich mit Symbol und Text.
6. **Links-nach-rechts und Rechts-nach-links gleichwertig.** Nur logische CSS-Eigenschaften (`margin-inline-start`, `padding-inline`, `inset-inline-end`), nie `left`/`right`.

## Farben (Tokens, flach)

Alle Farben als Variablen. Kein Wert steht direkt im Bauteil. Hell und Dunkel haben dieselben Namen.

| Token | Zweck |
|---|---|
| `--bg` | Seitenhintergrund |
| `--surface` | Karten, Menüs |
| `--surface-muted` | Zeilen bei Hover, Tabellenkopf |
| `--border` | Linien und Rahmen |
| `--text` | Haupttext |
| `--text-muted` | Zweittext |
| `--accent` | Hauptaktion, aktive Navigation |
| `--success` · `--warning` · `--danger` · `--info` | Status |

Statusfarben immer als Paar: Vollfarbe für Symbol und Text, **flache** helle Variante (`--success-soft` usw.) für Flächen.

Akzent: ein ruhiges Blau oder Petrol. Endgültige Werte werden in Phase 0 festgelegt und danach nicht mehr verändert.

## Tiefe ohne Verlauf

Trennung durch Linien und Flächenfarbe. Schatten nur dezent und einfarbig bei schwebenden Elementen (Menüs, Dialoge). Keine Schatten auf normalen Karten.

## Typografie

- Systemschrift oder eine einzelne Schrift mit guter arabischer Unterstützung. Vorschlag: **Inter** für Latein, **IBM Plex Sans Arabic** für Arabisch.
- Größen: 12 · 14 · 16 · 20 · 24 · 32. Fließtext 16 auf dem Handy (verhindert Zoom auf iOS), 14 in dichten Tabellen am Desktop.
- Zeilenhöhe 1,5 im Text, 1,25 in Überschriften.
- Zahlen in Tabellen mit gleich breiten Ziffern.

## Layout

| Breite | Verhalten |
|---|---|
| bis 640 px | Eine Spalte, untere Navigationsleiste mit 4 Einträgen plus "Mehr" |
| 641–1024 px | Seitenleiste eingeklappt (nur Symbole) |
| ab 1025 px | Seitenleiste mit Text, Inhaltsbreite maximal 1280 px |

- Die untere Leiste zeigt 4 Einträge plus "Mehr". Der Eintrag "Mehr" leuchtet auf, wenn die aktuelle Seite darin liegt. Lange Namen bekommen ein Kurzlabel (zum Beispiel "Apps" statt "Anwendungen"), damit nichts abgeschnitten wird.
- Tabellen werden auf dem Handy zu **Karten** (eine Karte pro Zeile), nicht zu Scroll-Tabellen.
- Trefferflächen mindestens 44 × 44 px.
- Hauptaktion einer Seite: ein deutlicher Button, bei schmaler Breite unten fixiert.

## Bauteile

- **Button:** primär (flache Akzentfarbe), sekundär (Rahmen), gefährlich (flache Rot-Fläche). Pro Bereich höchstens ein primärer.
- **Status-Badge:** Symbol, Text, flache `-soft`-Fläche.
- **Dialog:** auf dem Handy als Bottom-Sheet, am Desktop zentriert. Gefährliche Aktionen nennen im Dialog genau, was passiert (zum Beispiel "12 Ressourcen in `demo` werden überschrieben").
- **Formulare:** Beschriftung über dem Feld, Fehlermeldung direkt darunter, Hilfetext in `--text-muted`. Speichern deaktiviert nie ohne Erklärung.
- **Fortschritt:** flache Balken, bei unbekannter Dauer ein rotierendes Symbol. Kein Schimmer-Verlauf.
- **Leere Zustände:** kleines Symbol, ein Satz, ein Button.

## Sprache und Ton

- Deutsch spricht den Nutzer mit "du" an. Englisch und Arabisch bleiben ebenfalls persönlich und freundlich.
- Der Markenname "Restora" steht nicht am Anfang oder Ende eines arabischen Satzes, weil lateinische Wörter in rechts-nach-links-Sätzen sonst leicht falsch einsortiert werden.
- Kurze, direkte Sätze. Kein Fachwort, wenn es ein normales gibt ("Sicherung" statt "Snapshot-Artefakt"). Fachbegriffe stehen in Klammern oder im Detailbereich.
- Fehler: **Was ist passiert, warum, was kann ich tun.** Technische Details sind eingeklappt verfügbar.
- Alle Texte kommen aus Übersetzungsdateien (DE, EN, AR). Keine Texte im Code.

## Bewegung

- Übergänge 150–200 ms, nur Deckkraft und Position.
- `prefers-reduced-motion` wird respektiert.

## Barrierefreiheit

- Kontrast mindestens 4,5 : 1 für Text.
- Alles per Tastatur bedienbar, sichtbarer Fokusrahmen.
- Sinnvolle Beschriftungen für Bildschirmleser.

## Prüfliste vor jeder Phase

- [ ] Auf 375 px Breite benutzbar
- [ ] Auf Arabisch (RTL) geprüft
- [ ] Im Dunkel-Modus geprüft
- [ ] Alle vier Zustände vorhanden
- [ ] Kein Verlauf im Code (`grep -ri "gradient" ui/src` ist leer)
