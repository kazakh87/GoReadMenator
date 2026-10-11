# GoReadMenator

Ist ein Fork von [grisuno/ReadMenator](https://github.com/grisuno/ReadMenator)
in der Programmiersprache Go. Ziel ist es, die Funktionalität von ReadMenator in
Go zu implementieren und zu erweitern.

GoReadMenator baut einen Symbol- und Verwendungsindex für Quellcode auf. Es
erkennt Klassen, Methoden, Typen und ihre Verwendungen über Dateigrenzen hinweg,
und es zeigt, was eine Änderung betrifft. Der Index wird inkrementell
aktualisiert und lässt sich über Kommandozeile, MCP-Server oder
HTTP-Schnittstelle abfragen. Ein KI-Assistent kann so den Graphen abfragen,
statt den Quellcode zu lesen.

Unterstützte Sprachen: C#, VB.NET, TypeScript und JavaScript, dazu
ASP.NET-Vorlagen (Razor `.cshtml`/`.razor`, ASPX `.aspx`/`.ascx`/`.master`). Ein
Projekt darf mehrere Sprachen enthalten.

## Build

Voraussetzung: Go 1.22 oder neuer.

```
make build            # erzeugt bin/goreadmenator
make test             # Tests
make vet              # go vet
```

oder direkt:

```
go build -o bin/goreadmenator ./cmd/goreadmenator
```

Der Standardbau hat keine Fremdabhängigkeiten. Das SQLite-Backend braucht den
Treiber `modernc.org/sqlite` und wird mit dem Build-Tag `modernc` gebaut, siehe
[docs/SQLITE.md](docs/SQLITE.md):

```
go build -tags modernc -o bin/goreadmenator ./cmd/goreadmenator
```

## Tests

```
go test ./...                 # gesamte Suite ohne SQLite-Treiber
./scripts/test-sqlite.sh      # mit dem SQLite-Treiber (braucht Netz zum Go-Modul-Proxy)
```

Die Tests sind nach Schicht geordnet:

| Schicht   | Prüft                                                  | Ort                                           |
| --------- | ------------------------------------------------------ | --------------------------------------------- |
| Parser    | Symbole, Fundorte, Verwendungen, Diagnosen je Sprache  | `internal/parser/*/testdata` (Golden-Dateien) |
| Auflösung | Verwendungen über Dateigrenzen, Konfidenz, Beziehungen | `internal/resolve/testdata`                   |
| Abfragen  | Suche, Details, Paginierung, Kontext, Endpunkte        | `internal/query/testdata`                     |
| Speicher  | Vertrag des Store-Interface für jedes Backend          | `internal/store/storetest`                    |

Jeder Golden-Fall hat eine Datei `scenario.txt` oder Kommentare am Dateianfang
mit `SZENARIO` und `ABDECKUNG`. Die Übersicht aller Fälle steht in `COVERAGE.md`
des jeweiligen Testordners.

Golden-Dateien aktualisieren, wenn sich das Verhalten bewusst ändert:

```
READMENATOR_UPDATE_GOLDEN=1 go test ./internal/parser/... ./internal/resolve ./internal/query
```

Danach die geänderten `.golden.txt`-Dateien durchsehen, bevor man sie übernimmt.

## Nutzung über die Kommandozeile

```
goreadmenator -repo <pfad> index                  # Vollaufbau oder Inkrement
goreadmenator -repo <pfad> symbol <name>          # Symbole mit diesem Namen oder FQN
goreadmenator -repo <pfad> refs <name>            # Verwendungen
goreadmenator -repo <pfad> impact <name>          # Verwendungsketten bis zur Tiefe -depth
goreadmenator -repo <pfad> diff <alt>..<neu>      # geänderte Symbole zwischen Commits und ihre Wirkung
goreadmenator -repo <pfad> export json            # json, jsonl, mermaid oder markdown
goreadmenator import-xml <datei.xml>              # .NET-XML-Dokumentation ohne Quelltext
```

Wichtige Optionen:

| Option       | Bedeutung                                                  | Standard                               |
| ------------ | ---------------------------------------------------------- | -------------------------------------- |
| `-repo`      | Pfad des Git-Repositories                                  | `.`                                    |
| `-store`     | Speicher-Backend: `snapshot`, `memory`, `sqlite` (mit Tag) | `snapshot`                             |
| `-db`        | Pfad der Indexdatei                                        | `<repo>/.git/goreadmenator/index.snap` |
| `-json`      | Ausgabe als JSON                                           | aus                                    |
| `-depth`     | Tiefe der Impact-Analyse                                   | `3`                                    |
| `-id`        | exakte Symbol-ID statt Name, verhindert Mehrdeutigkeit     |                                        |
| `-workers`   | parallele Parser                                           | Anzahl CPUs                            |
| `-max-size`  | größte Datei in Bytes, die indiziert wird                  | 5 MB                                   |
| `-scope`     | Präfix des FQN, nur bei `export mermaid`                   |                                        |
| `-out`       | Ausgabedatei, bei `markdown` ein Verzeichnis               | stdout                                 |
| `-mem-limit` | weiches Speicherlimit in MB                                | kein                                   |

Beispiele:

```
goreadmenator -repo ~/projekte/shop index
goreadmenator -repo ~/projekte/shop -json symbol OrderService
goreadmenator -repo ~/projekte/shop -id 'csharp:Shop.Services.OrderService.Save(Shop.Order)' refs
goreadmenator -repo ~/projekte/shop export mermaid -scope Shop.Services -out graph.md
```

Nach jedem Commit genügt `goreadmenator -repo <pfad> index`. Es werden nur
geänderte Dateien neu geparst.

## Projekte verwalten

Für MCP und HTTP werden Projekte in einer Registry geführt, standardmäßig
`~/.goreadmenator/projects.json`:

```
goreadmenator project add shop ~/projekte/shop
goreadmenator project add api ~/projekte/api
goreadmenator project list
goreadmenator project remove api
```

Jedes Projekt hat seinen eigenen Index. Die Registry lässt sich mit
`-registry <datei>` umlenken.

## Nutzung über MCP (für KI-Assistenten wie Claude Code)

goreadmenator spricht das Model Context Protocol über stdio und ist nur lesend.
Der Server bedient alle Projekte der Registry, jedes Werkzeug akzeptiert einen
Parameter `project`:

```
goreadmenator mcp
```

Einbindung in Claude Code (`.mcp.json` im Projektordner oder über
`claude mcp add`):

```json
{
  "mcpServers": {
    "goreadmenator": {
      "command": "/pfad/zu/goreadmenator",
      "args": ["mcp"]
    }
  }
}
```

Verfügbare Werkzeuge:

| Werkzeug       | Zweck                                                    |
| -------------- | -------------------------------------------------------- |
| `projects`     | verfügbare Projekte                                      |
| `find_symbols` | Symbole nach Namensteil suchen                           |
| `get_symbol`   | Signatur, Dokumentation, Fundorte                        |
| `api`          | öffentliche API unter einem Namensraum                   |
| `neighborhood` | Beziehungen bis Tiefe 3                                  |
| `usages`       | Verwendungen bis Tiefe 3                                 |
| `context`      | Symbol, Beziehungen und Verwendungen in einem Paket      |
| `endpoints`    | HTTP-Routen des Backends mit ihren Aufrufern im Frontend |
| `source`       | Quelltext-Ausschnitt, höchstens 80 Zeilen                |
| `changes`      | geänderte Symbole seit einem Commit                      |
| `status`       | Zustand des Index und Versionshinweise                   |

Jede Antwort ist auf ein Token-Budget begrenzt (Parameter `budget_tokens`,
Standard 2500). Größere Ergebnisse liefern ein `nextCursor`, mit dem man die
nächste Seite abruft.

## Nutzung über die HTTP-Schnittstelle

```
goreadmenator serve                                  # 127.0.0.1:7878
goreadmenator serve -addr 0.0.0.0:7878 -token geheim # nur mit Token über das Netz
```

Alle Endpunkte sind lesend und liegen unter `/v1/{projekt}/...`:

```
curl localhost:7878/v1/projects
curl "localhost:7878/v1/shop/symbols?q=OrderService&budget=500"
curl "localhost:7878/v1/shop/context?ref=csharp:Shop.Services.OrderService&budget=2500"
curl "localhost:7878/v1/shop/endpoints"
```

Die Beschreibung im OpenAPI-Format liegt unter `/v1/openapi.json`. Mit `-token`
oder `READMENATOR_TOKEN` verlangt der Server den Header
`Authorization: Bearer <token>`. Die Schnittstelle hat keine
TLS-Verschlüsselung. Für den Zugriff über das Netz gehört sie hinter einen
Reverse-Proxy mit HTTPS.

## Speicher-Backends

| Backend               | Eigenschaft                                                                            |
| --------------------- | -------------------------------------------------------------------------------------- |
| `snapshot` (Standard) | alles im Speicher, beim Schreiben als Datei gesichert. Für Projekte bis mittlere Größe |
| `memory`              | nur im Speicher, für Tests                                                             |
| `sqlite`              | Datenbankdatei, Abfragen über SQL, nur mit Build-Tag `modernc`                         |

## Projektstruktur

```
cmd/goreadmenator/       Einstiegspunkt
internal/cli/          Befehle und Optionen
internal/parser/       Parser je Sprache (csharp, vbnet, tsjs, razor)
internal/resolve/      Auflösung der Verwendungen und Beziehungen
internal/index/        Pipeline: Lesen, Parsen, Schreiben
internal/update/       Vollaufbau und Inkrement über git diff
internal/query/        Abfrageschicht mit Token-Budget
internal/mcp/          MCP-Server
internal/httpapi/      HTTP-Schnittstelle
internal/store/        Speicher-Backends und Vertragstests
internal/langver/      Prüfung der Sprachversion (Warnungen)
docs/                  Anleitungen (Parser hinzufügen, SQLite)
```

Der Umsetzungsplan mit Stand und offenen Punkten liegt separat in
[docs/UMSETZUNGSPLAN.md](docs/UMSETZUNGSPLAN.md).

## License

AGPL-3.0 (geerbt von grisuno/ReadMenator)

## Umsetzung

Die Umsetzung wird in [docs/UMSETZUNGSPLAN.md](docs/UMSETZUNGSPLAN.md)
dokumentiert.
