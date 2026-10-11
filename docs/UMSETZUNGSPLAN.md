# GoReadMenator – Umsetzungsplan

**Stand:** 2026-10-10 **Aktuelle Phase:** P30 abgeschlossen (Konventionsrouting
und @page). Offen: Areas, SQLite-Backend, TLS über Proxy, P15c

## Statuslegende

| Zeichen | Bedeutung                              |
| ------- | -------------------------------------- |
| `[ ]`   | offen                                  |
| `[~]`   | in Arbeit                              |
| `[x]`   | erledigt                               |
| `[!]`   | blockiert (Grund in Klammern ergänzen) |

Vorgehen: Immer nur die oberste offene Phase bearbeiten. Beim Start einer
Aufgabe `[~]` setzen, beim Abschluss `[x]` und das Datum am Ende der Zeile
notieren. Eine Phase gilt erst als abgeschlossen, wenn alle Akzeptanzkriterien
erfüllt sind.

## Übersicht

| Phase | Titel                                   | Status                                                               |
| ----- | --------------------------------------- | -------------------------------------------------------------------- |
| P0    | Vorbereitung und Testkorpus             | [~]                                                                  |
| P1    | Projektgerüst                           | [x]                                                                  |
| P2    | Datenmodell                             | [x]                                                                  |
| P3    | Store-API und SQLite-Backend            | [~]                                                                  |
| P4    | Scanner über Git                        | [x]                                                                  |
| P5    | Parser-Framework und Worker-Pool        | [x]                                                                  |
| P6    | C#-Parser                               | [x]                                                                  |
| P7    | TypeScript/JavaScript-Parser            | [x]                                                                  |
| P8    | Resolver                                | [x]                                                                  |
| P9    | Beziehungen zwischen Symbolen           | [x]                                                                  |
| P10   | Inkrementelle Updates per Git-Commit    | [x]                                                                  |
| P11   | Diff auf Symbolebene und Impact-Analyse | [x]                                                                  |
| P12   | CLI                                     | [x]                                                                  |
| P13   | Export                                  | [x]                                                                  |
| P14   | Performance und Speicher                | [x]                                                                  |
| P15   | Erweiterungen (später)                  | [~] (P15a umgesetzt, P15b auf öffentliche API reduziert, P15c offen) |

## Entscheidungen (verbindlich)

- Kern nur mit der Go-Standardbibliothek. Einzige Ausnahme: `modernc.org/sqlite`
  (reines Go, ohne cgo).
- Symbole stehen im Vordergrund, Dateien sind Fundorte.
- Symbol-ID = Sprache + vollqualifizierter Name. Partial-Klassen werden über die
  ID zusammengeführt.
- Neues Schema, Verbesserung gegenüber dem Python-Original.
  Kompatibilitäts-Export ist optional.
- Persistenz in SQLite. Pro Lauf wird der geparste Commit gespeichert.
- Verwendungen werden mit Quelldatei, Start- und Endzeile gespeichert, mit
  Konfidenz `EXACT`, `RESOLVED` oder `HEURISTIC`.
- Summary, Remarks und Parameterbeschreibungen werden aus `///` bzw. JSDoc
  übernommen.
- v1 arbeitet nur mit Commits. Der unkommittete Arbeitsbaum kommt in P15.
- Überladungen werden über die Parametertypen in der ID unterschieden, nicht
  über einen Laufindex. Ein Index würde bei jeder Umsortierung die IDs ändern
  und das inkrementelle Update zerstören. C#-Schlüsselwörter werden auf
  System-Typen abgebildet (`int` → `System.Int32`), damit Aliase dieselbe ID
  ergeben.
- Persistenz hinter einer Store-Schnittstelle (`internal/store.Store`). SQL
  steht ausschließlich im Paket `store/sqlite`. Backends melden sich über
  `store.Register` an und werden per `-store` gewählt.

---

## P0 – Vorbereitung und Testkorpus

- [x] Zwei lokale Testrepositories festlegen: ein C#-Projekt, ein TS/JS-Projekt.
      Commit-Hash jeweils notieren. (2026-10-10)
  - C#: `jbogard/MediatR`, Commit `916ef1b3d68ccdc96db8f914eaf1b32fc7db52c5`,
    192 Dateien, 770 Commits
  - TS: `colinhacks/zod`, Commit `e06a510b6fc1e6e28e3201fd116ca2a76212f6be`, 742
    Dateien, 3248 Commits
- [x] Python-Baseline messen: Laufzeit und Speicher-Peak mit
      `--json --no-video --no-agent-injection --no-agent-output` (2026-10-10)
  - MediatR: 0,6 s Laufzeit, 31 MB Peak-RSS, 153 Knoten, 971 Symbole, 550 Kanten
  - zod: 0,26 s Laufzeit, 29 MB Peak-RSS, **nur 14 Knoten, 60 Symbole**, 737
    Kanten (siehe Befund unten)
  - `--rebuild` nicht gemessen (lief über das Tool-Limit von 300 s, nicht weiter
    untersucht)
- [~] Hand-Stichproben festhalten
  - C#: 10 Symbole zufällig gezogen (Seed 1), 9 Zeilennummern gegen den
    Quelltext geprüft, 1 ungeklärt (`RegisterBehaviorIfImplementationsExist`,
    Definition nicht eindeutig gefunden)
  - TS: 7 exportierte Deklarationen aus `packages/zod/src/v4/classic/schemas.ts`
    notiert (Zeilen 33, 34, 170, 361, 452, 617, 619), Python hat diese Datei
    nicht erfasst
  - Verwendungen (je 5 pro Repo): offen
- [ ] Ziel-Metriken festlegen (Vollaufbau, Inkrement, Speicher). Erst sinnvoll
      mit einem größeren Repo, da die Baseline-Repos unter einer Sekunde laufen.

### Befunde aus P0 (fließen in den Plan ein)

- **Ausschlussliste ignoriert Monorepos:** Python überspringt Verzeichnisse wie
  `packages`, `bin`, `obj`, `out`, `dist`, `build`. Bei zod fallen dadurch 704
  von 742 Dateien weg. Der Go-Scanner muss deshalb über `git ls-tree` laufen
  (P4), nicht über Namens-Ausschlüsse. Das ist bereits so geplant, wird jetzt
  aber durch die Messung belegt.
- **Methodenzeilen sind um 1 bis 2 Zeilen zu früh:** Bei 4 gemessenen Methoden
  lag die Python-Zeile 1 bis 2 Zeilen vor der echten Deklaration (Ursache: der
  Regex-Treffer beginnt im Leerraum vor dem Methodennamen). Klassen sind
  korrekt. Der C#-Parser in P6 muss die Startzeile am Namen selbst festmachen.
- **Konstruktoren werden als `method` geführt:** `ConstructorTestHandler`
  erscheint als Methode. Konstruktoren bekommen in P6 eine eigene Art.
- **Vererbung bei C# fehlt im Original:** `class Ping : IRequest<Pong>` erzeugt
  im Python-Graphen keine Kante `implements`/`inherits`. Das ist durch P9
  abgedeckt.

## P1 – Projektgerüst

- [x] `go mod init` mit Modulpfad festlegen. (Modul `goreadmenator`,
      Platzhalter, bei Veröffentlichung anpassen)
- [x] Verzeichnisstruktur anlegen: `cmd/goreadmenator/`, `internal/config/`,
      `internal/model/`, `internal/store/`, `internal/scanner/`,
      `internal/parser/`, `internal/parser/csharp/`, `internal/parser/tsjs/`,
      `internal/resolve/`, `internal/analyze/`, `internal/gitx/`,
      `internal/export/`.
- [x] Konfigurationsstruct mit Defaults (Worker-Anzahl, Batchgröße, DB-Pfad).
- [x] Logging mit `log/slog` einrichten.
- [x] `go vet` und `go test ./...` laufen lassen, Makefile oder Skript für die
      Standardkommandos.
- [x] Git-Repository für das Projekt anlegen, `.gitignore` für Binaries und
      lokale DB-Dateien.

**Akzeptanzkriterien:** (erfüllt, Go 1.22) `go build ./...` und `go test ./...`
laufen fehlerfrei durch. `cmd/goreadmenator` startet und zeigt eine Hilfe an.

---

**Ergebnis (2026-10-10):** Projekt in `goreadmenator/` (Zip:
goreadmenator-p*.zip), Git-Commit `P1: Projektgerüst`. `go vet`, `go test`
(Config-Tests bestanden) und `go build` laufen. `goreadmenator --help` zeigt die
Hilfe, unbekannte Befehle und Befehle ohne Argument liefern Exit-Code 2,
Platzhalter-Befehle Exit-Code 1.

## P2 – Datenmodell

- [x] Struct `File` (ID, Pfad, Sprache, Blob-SHA, LOC, Datei-Doc).
- [x] Struct `Symbol` (ID, Art, Name, Container, Signatur, Modifikatoren,
      Summary, Remarks, Doc-Quelle).
- [x] Struct `Location` (Symbol-ID, Datei-ID, Span mit Start- und Endzeile und
      -spalte, Rolle `decl` oder `partial`).
- [x] Struct `Reference` (Ziel-ID oder leer, Ziel-Rohname, Art, Span, umgebendes
      Symbol, Konfidenz).
- [x] Struct `Relation` (von, nach, Art, Datei-ID).
- [x] Struct `Import` (Datei, Rohstring, Art, aufgelöste Datei).
- [x] Konstanten für Symbolarten, Verwendungsarten, Beziehungsarten,
      Konfidenzstufen, Import-Arten.
- [x] Funktion zur Bildung der Symbol-ID aus Sprache und FQN, mit Unit-Tests.

**Ergebnis (2026-10-10):** Paket `internal/model`. Alle Fakten tragen eine
Datei-ID, damit das Löschen einer Datei vollständig möglich ist.
`go test ./internal/model` besteht mit 9 Tests, unter anderem zu Überladungen,
C#-Aliasen, Generics, Arrays und Nullable-Typen.

## P3 – Store-API und SQLite-Backend

**Vorgabe (2026-10-10):** Die Datenbank wird über eine Middleware-Schnittstelle
angesprochen. SQL-Statements stehen nicht im Anwendungscode. Ist SQLite nicht
nutzbar, wird zunächst ein Dummy gebaut, der später ersetzt wird.

- [x] Store-Schnittstelle `internal/store.Store` definiert (Meta, Dateien,
      Batch-Schreiben, Symbole, Fundorte, Verwendungen, Beziehungen, Löschen
      einer Datei).
- [x] Registry für Backends (`store.Register`, `store.Open`) mit Auswahl über
      `-store`.
- [x] Referenz-Backend `memory` (vollständig, thread-sicher) für Tests und als
      Übergang.
- [x] Vertragstests `internal/store/storetest` mit acht Fällen (Meta, Upsert,
      Merge, partial, Löschkaskade, Verwendungen, Beziehungen, Fehler bei
      unbekanntem Symbol). Jedes neue Backend ruft `storetest.Run` auf.
- [x] `memory` besteht alle Vertragstests.
- [x] Platzhalter `store/sqlite` registriert sich und liefert einen klaren
      Fehler (`ErrNotAvailable`), keine stillen Fehlfunktionen.
- [!] Echter SQLite-Treiber (`modernc.org/sqlite`): blockiert. Der Go-Proxy ist
  in dieser Umgebung nicht erreichbar, und der Treiber benötigt Go 1.24 (aktuell
  1.22 installiert). Nachholen, sobald Proxy-Zugriff und Go 1.24 verfügbar sind.
- [ ] Nach Einbindung: Schema als eingebettete Migration (`embed`), Tabellen wie
      in der Schema-Skizze, Pragmas (WAL, `synchronous=NORMAL`).
- [ ] Nach Einbindung: Schreib-API mit Batch-Transaktionen und Prepared
      Statements.
- [ ] Nach Einbindung: `storetest.Run` gegen `sqlite` ausführen, alle Fälle
      müssen grün sein.
- [ ] Nach Einbindung: Richtwert prüfen, 100.000 Zeilen in unter 5 Sekunden
      (nach Messung anpassen).

**Akzeptanzkriterien (Stand):** Der Anwendungscode kennt nur `store.Store`. Ein
SQL-Statement außerhalb von `store/sqlite` gibt es nicht (geprüft per grep). Der
Wechsel auf SQLite ist danach eine Implementierung, kein Umbau.

## P4 – Scanner über Git

- [x] `gitx`-Paket: `git ls-tree -r HEAD` ausführen und Pfad, Blob-SHA, Größe
      parsen.
- [x] Sprache anhand der Dateiendung bestimmen. Nur C# und TS/JS werden in v1
      aufgenommen.
- [x] Pfadsicherheit: keine Pfade außerhalb des Repos, keine Symlinks auf fremde
      Ziele.
- [x] Größenlimit pro Datei (Standard festlegen, konfigurierbar), größere
      Dateien überspringen und protokollieren.
- [x] Commit-Hash über `git rev-parse HEAD` ermitteln.
- [x] Tests mit einem kleinen Git-Repo, das im Test erzeugt wird.

**Akzeptanzkriterien:** Die Dateiliste des Testrepos stimmt mit `git ls-files`
überein. Nicht getrackte und per `.gitignore` ausgeschlossene Dateien tauchen
nicht auf.

---

**Ergebnis (2026-10-10):** Pakete `internal/gitx` (git-Aufrufe ohne Shell,
`ls-tree -r -l -z`) und `internal/scanner` (Sprachzuordnung, Pfadprüfung,
Skip-Gründe). Tests: Parser für `ls-tree`-Ausgabe, Fixture-Repo mit
Monorepo-Layout, Symlink, Größenlimit, gitignorierten und untrackten Dateien.
Abgleich gegen echte Repos: `zod` ergibt 523 indizierbare Dateien (Python: 14),
`MediatR` 154 (Python: 153). Der Abgleich mit `git ls-files` ist per
`READMENATOR_CORPUS` aktivierbar und besteht für beide Repos.

## P5 – Parser-Framework und Worker-Pool

- [x] Interface `Parser` definieren: Eingabe Dateiinhalt und Pfad, Ausgabe
      Symbole, Locations, Verwendungen, Imports, Relationen als
      Zwischenstruktur.
- [x] Registry: Sprache → Parser.
- [x] Worker-Pool mit `runtime.NumCPU()` Goroutinen, begrenzter Channel für
      Eingaben.
- [x] Ein einziger Writer, der Ergebnisse in Batches in den Store schreibt.
- [x] Kontext-Abbruch über `context.Context` (Strg+C sauber behandeln).
- [x] Fortschrittsanzeige über stderr.
- [x] Fehler pro Datei sammeln und am Ende ausgeben, ohne den Lauf abzubrechen.

**Akzeptanzkriterien:** Mit einem Dummy-Parser läuft der Pool über 10.000
Dateien ohne Deadlock, und der Writer schreibt alle Ergebnisse. Abbruch über
Kontext funktioniert.

---

**Ergebnis (2026-10-10):** Pakete `internal/parser` (Interface, Source, Result,
Registry), `internal/index` (Pipeline) und `internal/gitx` (`CatFile`: ein
langlebiger `git cat-file --batch`-Prozess pro Worker statt eines Prozesses je
Datei).

Akzeptanz: 10.000 Dateien mit Dummy-Parser laufen mit 8 Workern durch, ohne
Deadlock und mit vollständigem Schreiben (0,05 s). Abbruch über Kontext stoppt
den Lauf. Ein Schreibfehler im Store bricht den Lauf sauber ab. Ein Fehler in
der ContentReader-Erzeugung bricht ebenfalls ab. Zusätzlich: Dateifehler stoppen
den Lauf nicht, Re-Index ist idempotent, und der Integrationstest mit echtem
`git cat-file` besteht. `go test -race` ist grün.

Durchsatz (nur Lesen und Schreiben, Dummy-Parser): MediatR 154 Dateien in 27 ms.
Die 523 Dateien aus zod melden Fehler „kein Parser für Sprache“, weil noch kein
TS-Parser existiert. Das ist das erwartete Verhalten und wird mit P7
verschwinden.

## P6 – C#-Parser

- [x] Vorverarbeitung: Kommentare, Strings (inkl. verbatim `@"..."`, Rohstrings
      `"""`), Zeichenliterale und Präprozessor-Zeilen werden maskiert.
      Positionen bleiben erhalten. UTF-8-BOM wird entfernt.
- [x] `namespace` (Block- und Datei-scoped) erkannt, Namensraum-Symbol mit
      Fundort.
- [x] `using`, `using static`, `using Alias = ...` und `global using` erfasst.
- [x] Typen erkannt: `class`, `struct`, `interface`, `record` (inkl.
      `record struct`), `enum`, mit Modifikatoren, Generics (Arity im Namen, z.
      B. `Repo`1`), Primärkonstruktor und Basisliste.
- [x] Member erkannt: Methoden, Konstruktoren, Destruktoren (übersprungen),
      Operatoren, Konvertierungsoperatoren, Properties (inkl. Expression-Body
      und Initialisierer), Indexer, Felder (mehrere Deklaratoren), Events,
      Enum-Werte.
- [x] Mehrzeilige Signaturen und Körper bis zur schließenden Klammer. Startzeile
      am Namen, Endzeile am Körperende.
- [x] `partial`: Rolle `partial`, gleiche ID über Dateien hinweg. Das
      Zusammenführen selbst übernimmt der Store (P3).
- [x] Attribute erfasst, am Typ oder Member als Verweise gespeichert.
- [x] XML-Doku (`///`): `summary`, `remarks`, `returns`, `param` werden
      übernommen. `<see cref>` und `<seealso cref>` werden als Verwendungen mit
      Konfidenz `EXACT` gespeichert.
- [x] Verwendungen: Methodenaufrufe (auch Generic-Aufrufe), `new X(...)`,
      Typverweise (inkl. Generic-Argumente), Schreibzugriffe (`x = ...`,
      `this.x = ...`), Basisangaben und Attribute.
- [~] Lesezugriffe auf Felder und Properties werden nicht erfasst. Das würde
  ohne Typauflösung zu viel Rauschen erzeugen und ist auf P8 verschoben.
- [x] Tests: Fixture pro Konstrukt, Maskierung, Präprozessor-Duplikate,
      Robustheit gegen kaputten Code, Stichproben aus P0 gegen MediatR.

**Ergebnis (2026-10-10):** Paket `internal/parser/csharp` mit den Dateien
`mask.go`, `lex.go`, `syntax.go`, `state.go`, `members.go`, `refs.go`,
`xmldoc.go`, `csharp.go`. `go test` ist grün. Die Stichproben gegen MediatR
stimmen: `BuildMediator` 26, `ConstructorTestHandler` 57, `Handle` 45,
`ConstrainedPingedHandler` 43, `SyncContinueOnException` 131.

Lauf über alle 154 Dateien von MediatR: 1583 Symbole (Python-Original: 971),
6911 Verwendungen, 0 Diagnosen, 33 ms.

Korrekturen am Python-Stand, die sich dabei ergaben: Methoden- und
Konstruktorzeilen stimmen jetzt. Konstruktoren haben eine eigene Art.
Basisklassen und Interfaces werden erfasst. Zeilen für `Handle` in
`PipelineTests.cs` lagen im Plan auf 44, die echte Deklaration steht auf 45 (das
war ein Fehler in meiner P0-Stichprobe, die Python-Zeile 44 war falsch).

Bekannte Grenzen für P8/P9: Typen werden nur über den Rohnamen geführt
(`IRepo`). Die Auflösung auf vollqualifizierte IDs kommt in P8. Lokale Variablen
mit Typ (`Foo x = ...`) erzeugen keinen Typverweis.

## P7 – TypeScript/JavaScript-Parser

- [x] Vorverarbeitung: Kommentare, Strings (inkl. Escapes), Template-Literale
      mit `${...}`-Interpolation (Ausdrücke bleiben sichtbar), Regex-Literale
      (Unterscheidung von Division über den vorherigen Token, nur bei Abschluss
      auf derselben Zeile), Shebang und BOM.
- [x] JSDoc (`/** */`) wird für die Zuordnung gesammelt. Summary, `@param`,
      `@returns`, `@remarks` werden übernommen. `@see` und `{@link}` werden als
      Verwendungen mit Konfidenz `EXACT` gespeichert.
- [x] Imports in allen Formen: Default, Named (inkl. `type`-Bindungen und `as`),
      Namespace (`* as`), Seiteneffekt, `import x = require()`, `require()`,
      dynamisches `import()`, Re-Exporte (`export * from`, `export { } from`,
      `export * as`). Die gebundenen Namen werden als Import-Verweise
      gespeichert.
- [x] Funktionen mit Überladungen (Signaturen ohne Körper), Pfeilfunktionen und
      function-Ausdrücke als `const`-Werte, Default-Exporte.
- [x] Klassen mit `extends`/`implements`, Konstruktor, Methoden, Properties,
      get/set-Accessoren, statische Felder, `#private`-Namen, Decorators als
      Verweise.
- [x] Interfaces mit Methoden-, Property- und Interface-Erweiterungen, Index-
      und Aufrufsignaturen werden übersprungen.
- [x] `enum` mit Mitgliedern (als Felder), `type`-Aliase mit Typverweisen,
      Namespaces und `declare module`.
- [x] Export-Status über die Modifikatoren `export` und `default`.
- [x] Verwendungen: Aufrufe (auch generisch), `new`, Typverweise in
      Annotationen, Generics und `as`-Ausdrücken, JSX-Komponenten (nur
      `.tsx`/`.jsx`), Schreibzugriffe am Anweisungsanfang und nach `this.`.
- [x] Typparameter werden erkannt und erzeugen keine Typverweise.
- [x] Tests: Fixtures pro Konstrukt, Maskierung, Robustheit gegen kaputten Code,
      BOM und Shebang, Interface-Merging, zod-Stichproben aus P0.
- [~] Lokale Variablen, Lesezugriffe und Objekt-Literal-Methoden werden nicht
  als Symbole geführt. Das würde den Graphen mit Rauschen füllen.

**Ergebnis (2026-10-10):** Paket `internal/parser/tsjs` (Dateien `mask.go`,
`lex.go`, `syntax.go`, `jsdoc.go`, `state.go`, `decls.go`, `members.go`,
`refs.go`, `tsjs.go`). Alle Tests sind grün.

Lauf über alle 518 TS/JS-Dateien von zod: 5660 Symbole, 58172 Verwendungen, 1420
Imports, 0 Diagnosen, 0,25 s. Die zod-Stichproben aus P0 stimmen:
`ZodStandardSchemaWithJSON` 33, `ZodType` 34, `_ZodType` 170, `_ZodString` 361,
`ZodString` 452, `string` 617.

Bekannte Grenzen: Overloads mit gleichen Parametertypen teilen sich eine ID (zum
Beispiel `string(params?)` in zod). Ihre Fundorte werden zusammengeführt, was
fachlich richtig ist. Typen bleiben als Rohnamen stehen, die Auflösung folgt in
P8. Ein JSX-Text, der `//` enthält, wird als Kommentar gelesen. Das betrifft nur
die Zeile.

Technische Schuld: Das Token-, Masken- und Klammer-Hilfsmaterial ist zwischen
`csharp` und `tsjs` dupliziert. Eine Zusammenführung in ein gemeinsames Paket
ist als eigener Punkt für später vorgesehen.

## P8 – Resolver

- [x] Import-Auflösung für relative Pfade (`./`, `../`) mit Endungen,
      `index`-Dateien und `.js`→`.ts`-Zuordnung.
- [x] `tsconfig.json`: `baseUrl` und `paths` (mit Platzhaltern), Kommentare und
      nachgestellte Kommas werden toleriert. Die Datei wird im Repository-Root
      gelesen.
- [x] Namensraum-Index: Symbole sind über ihren vollqualifizierten Namen
      abrufbar. Für C#-Using-Direktiven werden `using`, `using static` und
      Aliase ausgewertet.
- [x] Namensauflösung nach fester Reihenfolge: Scope-Kette (Member,
      verschachtelte Typen, Namespaces), Datei-Ebene, Import-Bindungen bzw.
      using, dann global, zuletzt eindeutiger Name im Projekt.
- [x] Konfidenzen: `EXACT` für lexikalische und explizite Treffer, `RESOLVED`
      für using und globale FQN-Treffer, `HEURISTIC` für den eindeutigen Namen
      im Projekt.
- [x] Mehrdeutige Treffer werden nie geraten. Das gilt auch bei Überladungen mit
      gleicher Argumentzahl.
- [x] Re-Auflösung über `target_name` (`OnlyNames`): veraltete Ziele werden
      zurückgenommen, sobald ein Symbol verschwindet.
- [x] Zusätzlich: Argumentzahl bei Aufrufen und Konstruktoren, Typargumente bei
      C#-Generics
      (`IRequestHandler`2`), Qualifier bei Member-Aufrufen (`util.keys()`), Re-Exporte (`export
      { a } from`,`export *`).
- [x] Tests: C# mit Scope, using, Überladungen, Mehrdeutigkeit und cref mit
      Parametern; TS/JS mit Bindungen, Default-Import, Alias, tsconfig-Pfaden;
      Re-Scan über `OnlyNames`.
- [~] Die Akzeptanz „mindestens 80 % der Stichproben korrekt“ ist noch nicht
  geprüft. Dafür fehlt eine Referenzliste mit Hand-Stichproben für Verwendungen
  (P0-Restpunkt). Die Zahlen unten sind Messwerte, keine Trefferquote.

**Ergebnis (2026-10-10):** Paket `internal/resolve` (`index.go`, `resolve.go`,
`tsconfig.go`). Der Store wurde erweitert: `AllSymbols`, `AllLocations`,
`AllReferences`, `AllImports`, `UpdateReferences`, `UpdateImports`. Verwendungen
und Imports tragen jetzt eine ID. Die Vertragstests decken die neuen Methoden
ab.

Messwerte über den gesamten Lauf (Index plus Auflösung):

- MediatR (C#): 6911 Verwendungen, davon 1010 EXACT, 105 RESOLVED, 672
  HEURISTIC, 5124 unaufgelöst. Die meisten unaufgelösten Namen gehören zu
  externen Typen wie `Task` oder `Type` aus dem .NET-Framework. Auflösung in 29
  ms.
- zod (TS/JS): 58 303 Verwendungen, davon 9430 EXACT, 6653 HEURISTIC, 42 220
  unaufgelöst. Von 1443 Imports sind 660 auf Projektdateien aufgelöst. Die
  übrigen sind Fremdpakete oder Assets wie `./global.css`. Auflösung in 103 ms.

Bekannte Grenzen, die P9 löst oder die bewusst offen bleiben:

- Member-Aufrufe auf Feldern (`_service.Submit(...)`) bleiben ohne Typwissen bei
  HEURISTIC oder unaufgelöst. Dafür braucht es die Beziehung `has_type` aus P9.
- Überladungen mit gleicher Argumentzahl werden nicht über Argumenttypen
  unterschieden. Das wäre eine eigene Ausbaustufe.
- Lokale Variablen und Namespace-Objekte aus `import * as` werden nicht als
  Symbole geführt.
- Der Namensraum-Index zeigt auf Symbole, nicht auf Dateien. Das reicht für die
  Auflösung, eine Liste „welche Dateien gehören zu Namespace X“ fehlt noch.

## P9 – Beziehungen zwischen Symbolen

- [x] `contains`: Typ → Member, Namespace → Typ, Typ → verschachtelter Typ. Wird
      direkt beim Parsen gesetzt.
- [x] `inherits` und `implements` aus Basisangaben. Entstehen nur aus
      aufgelösten Verweisen mit Konfidenz EXACT oder RESOLVED.
- [x] `has_type` für Felder, Properties, Events und Variablen. Die Rolle
      `has-type` markiert den Kopf des Typverweises, Generic-Argumente bleiben
      normale Verweise.
- [x] `param_type` und `returns` für Methoden und Funktionen.
- [x] `attribute_of`: vom Attributtyp zum dekorierten Symbol.
- [x] Beziehungen in beide Richtungen abfragbar (`Relations(ctx, id)`).
- [x] Abgeleitete Beziehungen sind vom geparsten Bestand getrennt (Flag
      `Derived`). `SetDerivedRelations` ersetzt sie bei jedem Auflösungslauf.
      Löschen einer Datei entfernt auch ihre abgeleiteten Beziehungen.
- [x] Tests: Vertragstests im Store, Ende-zu-Ende-Tests für C# und TS/JS
      (Vererbung, Interfaces, Felder, Parameter, Rückgabe, Attribute).

**Ergebnis (2026-10-10):** Die Rollen stehen in `model.Reference.Role`, die
Beziehungsableitung in `resolve.deriveRelations`. Der Store erhielt
`SetDerivedRelations` und `AllRelations`. Alle Tests sind grün.

Messwerte:

- MediatR (C#): 1831 Beziehungen insgesamt, davon 400 abgeleitet (235
  param_type, 75 has_type, 36 inherits, 30 implements, 23 returns, 1
  attribute_of). Der Rest sind contains-Kanten aus dem Parsen.
- zod (TS/JS): 3786 Beziehungen, davon 1939 abgeleitet (607 inherits, 535
  param_type, 413 returns, 383 has_type, 1 implements).

Bewusste Entscheidungen:

- Beziehungen entstehen nur aus Verweisen mit EXACT oder RESOLVED.
  HEURISTIC-Treffer werden nicht zu Fakten gemacht. Dadurch fehlen manche
  Basisklassen, die nur über den Namen vermutet wurden.
- Methoden-IDs enthalten die Parametertypen, wie sie geschrieben sind
  (`Get(Money)`), nicht den vollqualifizierten Namen. Das ist eine bekannte
  Schwäche der ID-Bildung für Überladungen über Typen aus anderen Namensräumen.
  Sie wird im Plan als Punkt für die Nachbesserung geführt.

Offen für eine spätere Stufe:

- Aufrufe über Felder (`_service.Submit(...)`) lassen sich jetzt mit `has_type`
  auflösen: Feld → Typ → Member. Das ist noch nicht verdrahtet und wäre der
  nächste Hebel für die Auflösungsqualität.

## P10 – Inkrementelle Updates per Git-Commit

- [x] `last_commit` und `parser_version` in den Metadaten. Bei abweichender
      `parser_version` oder fehlendem Commit: Vollaufbau.
- [x] `git diff --raw -z --find-renames` ausgewertet (`A`, `M`, `D`, `R`, `C`,
      `T`), inklusive der Blob-SHA des neuen Stands.
- [x] Geänderte Dateien werden neu geparst, gelöschte entfernt. Umbenennung =
      Löschen plus Neuanlage.
- [x] Dateien mit unverändertem Blob werden nicht neu geparst (Scanner-Blob-SHA
      gegen gespeicherte SHA). Die Auswahl erfolgt über den Scanner.
- [x] Symbole ohne verbleibende Fundorte werden über den Store entfernt,
      partial-Klassen bleiben erhalten, solange eine Datei sie deklariert.
- [x] Verwendungen werden für betroffene Namen neu aufgelöst: Namen aus den
      Symbolen und Verwendungen der geänderten Dateien vor und nach der
      Änderung.
- [x] `noop`, wenn der Commit gleich geblieben ist.
- [x] Tests: Inkrement gegen Vollaufbau (identischer Zustand über Snapshot aus
      Symbolen, Fundorten, Verwendungen, Beziehungen und Imports), Löschen einer
      partial-Datei, unbekannter Commit, Parser-Versions-Wechsel, Umbenennung
      und Neuanlage.

**Ergebnis (2026-10-10):** Paket `internal/update` (Orchestrierung),
`gitx.DiffRaw` mit Parser, `index.ParserVersion` als Versionsmarke. Alle Tests
sind grün, auch unter `-race`.

Die Gleichheitsprüfung ist der wichtigste Test: Ein Inkrement muss den Store
exakt in den Zustand bringen, den ein frischer Vollaufbau am selben Commit
liefern würde. Das gilt für alle Szenarien im Test.

Bekannte Grenzen:

- Der Scanner liest den gesamten Tree des Commits (`ls-tree`). Das ist günstig
  gegenüber dem Parsen, bei sehr großen Repositories aber der nächste Kandidat
  zur Optimierung.
- Der unkommittete Arbeitsbaum ist nicht Teil von P10 (siehe P15).
- Ein Force-Push, der den gespeicherten Commit entfernt, löst automatisch einen
  Vollaufbau aus.

## P11 – Diff auf Symbolebene und Impact-Analyse

- [x] `git diff -U0` mit Umbenennungserkennung ausgewertet. Die geänderten
      Zeilenbereiche des neuen Stands werden je Datei gesammelt. Reine
      Löschungen werden als Stelle erfasst.
- [x] Überlappung mit den Fundorten: betroffene Symbole mit Datei und Span.
- [x] „Direkt betroffen“: nur die innerste Stelle. Ein Typ, der nur wegen einer
      Änderung in einem Member markiert wäre, gilt nicht als direkt betroffen.
- [x] Impact-Analyse: Verwendungen eines Symbols, rekursiv über die umgebenden
      Symbole bis zur gewählten Tiefe. Jeder Treffer nennt Tiefe, verwendetes
      Symbol, Datei, Zeile und Art der Verwendung.
- [x] Abfrage nach Namen (`ByName`): alle Symbole mit diesem Namen.
- [x] Tests: direkt betroffene Methode bei geänderter Zeile, Klasse als
      Container, Aufrufkette über zwei Ebenen, Tiefenbegrenzung, Löschung
      innerhalb einer Methode, keine Treffer ohne Änderung.

**Ergebnis (2026-10-10):** Paket `internal/impact` mit `Changed`, `Analyze` und
`ByName`. In `internal/gitx` kommen `DiffHunks` und `ParseHunks` dazu. Alle
Tests sind grün.

Die Analyse arbeitet auf dem Index des neuen Stands. Der Aufrufer muss also
vorher `update` ausführen, sonst passen die Zeilen nicht zu den Fundorten.

Bekannte Grenzen:

- Verwendungen über Beziehungen (Vererbung, Typen von Feldern) fließen noch
  nicht in die Impact-Analyse ein. Das wäre ein sinnvoller Ausbau, weil eine
  geänderte Basisklasse alle Ableitungen betrifft.
- Ein Treffer auf Datei-Ebene (Verwendung ohne umgebendes Symbol, etwa in einer
  Modul-Initialisierung) wird als Treffer mit leerem Enclosing geführt.
- Bei Löschungen zählt die Zeile vor der Stelle. Liegt die Löschung genau an der
  Grenze eines Symbols, kann die Zuordnung um eine Zeile abweichen.

## P12 – CLI

- [x] Befehle: `index`, `symbol <name>`, `refs <name>`, `impact <name>`,
      `diff <alt>..<neu>`.
- [x] Optionen: `-repo`, `-store`, `-db`, `-json`, `-depth`, `-id`, `-workers`,
      `-batch`, `-max-size`. Optionen dürfen vor und nach Befehl und Argumenten
      stehen.
- [x] `--json` bei allen Befehlen, gültiges JSON mit stabilen Feldnamen.
- [x] Exit-Codes: 0 Erfolg, 1 Laufzeitfehler oder nichts gefunden, 2
      Nutzungsfehler, mehrdeutiger Name oder Index nicht am Zielcommit.
- [x] Mehrdeutige Namen: Kandidatenliste mit IDs, Auswahl über `-id`.
- [x] `refs` zeigt zusätzlich nicht eindeutig zugeordnete Stellen mit gleichem
      Namen, getrennt von den Treffern.
- [x] `diff` markiert direkt betroffene Symbole mit `*` und zeigt bei ihnen die
      Wirkung.
- [x] Tests: Vollablauf index → symbol → refs → impact (auch JSON), Exit-Codes,
      diff mit Wirkung, falsches Diff-Ziel.

**Ergebnis (2026-10-10):** Paket `internal/cli` (Befehle, Ausgabe, Exit-Codes),
`cmd/goreadmenator/main.go` als dünner Einstieg. Alle Tests sind grün.

Entscheidungen:

- **Speicher:** Der Index liegt im Backend `snapshot`. Das ist ein Übergang,
  weil SQLite nicht verfügbar ist (siehe P3). Es hält den Bestand im Speicher
  und schreibt ihn beim Schließen atomar in eine Datei. Ohne dieses Backend wäre
  der CLI-Aufruf „index“ nach dem Prozessende wertlos. Die Schnittstelle
  `store.Store` bleibt gleich, der Wechsel auf SQLite ist eine neue
  Implementierung.
- **Ablageort:** Standardmäßig `<repo>/.git/goreadmenator/index.snap`. Der Index
  taucht damit nie als unversionierte Datei im Arbeitsverzeichnis auf.
- **diff:** Der Befehl prüft zuerst, ob das Ziel HEAD ist, und aktualisiert dann
  den Index. Für andere Ziele wird abgebrochen, weil der Index nur den aktuellen
  Stand kennt.

Messwerte (MediatR): Vollaufbau mit Auflösung im Befehl `index` in unter einer
Sekunde. Ein zweiter Aufruf ohne Änderung meldet „Index ist aktuell“.

Bekannte Grenzen: Die Ausgabe ist derzeit Text oder JSON. Eine Ansicht im
Browser, eine Ausgabe als SARIF oder Export als Graph-Datei sind Teil von P13.
Die Befehle unterstützen keine Namensraum-Angaben wie `Namespace.Name` in allen
Varianten. Sie sind über FQN-Endungen teilweise möglich.

## P13 – Export

- [x] JSON als Dokument: Abschnitte `files`, `symbols`, `locations`,
      `references`, `relations`, `imports`, jeder Datensatz einzeln kodiert,
      kein zweites großes Objekt im Speicher.
- [x] JSON Lines: eine Zeile pro Datensatz mit `type` und `data`, für `jq` und
      Streams.
- [x] Mermaid-Klassendiagramm mit Vererbung (`<|--`), Interface-Implementierung
      (`<|..`) und optional Methoden und Properties. Mit `--scope` auf einen
      Namensbereich begrenzbar.
- [x] Markdown je Quelldatei: Signatur, Dokumentation, Rückgabe, Parameter,
      Zeilen und Anzahl eingehender Verwendungen.
- [x] CLI-Befehl `export <format>` mit `-out`, `-scope`, `-members`.
- [~] Kompatibilitäts-Export im Format des Python-Originals: nicht umgesetzt.
  Das war optional und wird nur gebaut, wenn es gebraucht wird.
- [x] Tests: gültiges JSON mit allen Abschnitten, eine gültige Zeile pro
      Datensatz, Mermaid mit Vererbung und Scope, Markdown mit Inhalt.

**Ergebnis (2026-10-10):** Paket `internal/export` und der Befehl `export` in
der CLI. Alle Tests sind grün.

Messwerte (zod, 518 Dateien): JSON-Dokument mit 523 Dateien, 5682 Symbolen, 5978
Fundorten, 58 303 Verwendungen, 3786 Beziehungen und 1443 Imports. Markdown
erzeugt 376 Seiten.

Bekannte Grenze: Der Store liefert den Bestand heute vollständig. Der Export
speichert deshalb den Graphen einmal im Speicher, schreibt die Ausgabe aber
Datensatz für Datensatz. Mit einem Backend, das Cursor liefert, wird das auch
beim Lesen speicherschonend.

## P14 – Performance und Speicher

- [x] Testkorpora in drei Größen: MediatR (154 Dateien), Orchard (6 861 Dateien,
      C#), TypeScript (67 291 Dateien, 26 540 davon indizierbar).
- [x] Messung mit Wall-Zeit und Peak-RSS, Profile mit `-cpuprofile` und
      `-memprofile`, Benchmarks über ganze Korpora (`READMENATOR_CORPUS`).
- [x] Engpässe beseitigt und jeweils neu gemessen (siehe unten).
- [x] Ziel-Metriken festgelegt und erreicht (siehe unten).
- [x] Weiches Speicherlimit über `-mem-limit`.
- [x] Speicherprofil zeigt, wo der Bestand liegt, und die Snapshot-Datei wird in
      Blöcken geschrieben.

**Ergebnis (2026-10-10):** Die Engpässe lagen an drei Stellen. Das wichtigste
Ergebnis betrifft den Resolver: Dort wurden ganze Symbole pro Vergleich kopiert,
und häufige Namen führten zu langen Kandidatenlisten.

Maßnahmen und Wirkung auf dem TypeScript-Korpus (26 540 Dateien):

| Stand                                        | Wall-Zeit | Peak-RSS |
| -------------------------------------------- | --------- | -------- |
| Erster Lauf                                  | 51 s      | 1 492 MB |
| Resolver: kompakte Maps, Namenssuche gemerkt | 15 s      | 1 881 MB |
| Token-Interning, Snapshot in Blöcken         | 12,6 s    | 1 194 MB |
| Mit `-mem-limit 700`                         | 17 s      | 771 MB   |

Weitere Änderungen: typisierte Schlüssel für die Duplikaterkennung (weniger
Allokationen beim Parsen, C#: 1,67 Mio. → 0,86 Mio.; TS: 5,7 Mio. → 3,7 Mio.).

Messwerte, aktueller Stand:

- **TypeScript (26 540 Dateien):** Vollaufbau 12,6 s, Peak 1,19 GB. Mit weichem
  Limit von 700 MB 17 s und 771 MB.
- **Orchard (C#, 4 694 Dateien):** Vollaufbau 5,0 s, Peak 459 MB.
- **Orchard, Inkrement mit einer geänderten Datei:** 1,4 s gegenüber 5,0 s für
  den Vollaufbau.
- **MediatR (154 Dateien):** 0,4 s, Peak 21 MB.
- **Parser-Durchsatz einzeln:** C# 15 MB/s, TS 15 MB/s pro Kern.
- **Python-Original auf Orchard:** nach 280 s abgebrochen, ohne Ergebnis. Auf
  MediatR liegt es bei 0,6 s. Der Vergleich ist aber nur für das Parsen fair,
  weil das Original keine Auflösung macht.

Ziel-Metriken (zuvor offen, jetzt festgelegt):

- Vollaufbau auf 26 000 Dateien unter 15 s: erreicht (12,6 s).
- Peak-Speicher auf diesem Korpus unter 1,2 GB, mit Limit 700 MB: erreicht (771
  MB).
- Inkrement mit einer Datei mindestens 3-mal schneller als Vollaufbau: erreicht
  (1,4 s gegenüber 5,0 s, Faktor 3,6).

Restposten, die bewusst offen bleiben:

- Die Garbage Collection kostet noch rund 30 % der CPU-Zeit. Ein höheres GOGC
  würde die Zeit senken, aber den Speicher erhöhen. Das ist eine Abwägung für
  den Betrieb.
- Die Store-Schnittstelle liefert den gesamten Bestand. Mit SQLite als Backend
  (Cursor) entfällt dieser Ballast.

### Nachtrag: Resolver und Garbage Collection (2026-10-10)

Der Resolver war noch zu langsam, weil er pro Vergleich ganze Symbol-Structs
kopierte, Schlüssel per Stringverkettung baute und Artenfilter als Closures
übergab. Umgesetzt:

- Hash-Maps mit strukturierten Schlüsseln (Präfix plus Name) statt verketteter
  Strings. Der Scope-Präfix wird je Symbol einmal berechnet.
- Symbolarten als Bitmaske. Parameterzahl und Typparameter werden beim Aufbau
  des Index einmal berechnet.
- Scope-Kette ohne Kopien: nur IDs, keine Symbol-Structs.
- `using`-Direktiven und Dateipräfixe vorbereitet, Namenssuche über `uniqueMemo`
  mit strukturiertem Schlüssel.

Ergebnis Resolver: Die Auflösungsphase fällt auf dem TS-Korpus von rund 3,3 s
CPU auf 3,0 s. Der Hauptgewinn liegt woanders, die Map-Zugriffe selbst sind
jetzt die größte Einzelposten (rund 5 %).

Garbage Collection: Der Anteil war zuerst etwa 30 %. Zwei Maßnahmen haben ihn
auf etwa 14 % gesenkt:

- Token- und Maskenpuffer der Parser werden über `sync.Pool` wiederverwendet.
- Die Batch-Slices der Pipeline werden nach jedem Schreiben wiederverwendet. Der
  Vertrag des Stores verlangt dafür, dass er die Datensätze kopiert.

Messung TS-Korpus (26 540 Dateien), Vollaufbau:

| Einstellung                               | Wall-Zeit | Peak-RSS |
| ----------------------------------------- | --------- | -------- |
| Vorher (GOGC 100)                         | 12,3 s    | 1 220 MB |
| Nach Pool und Wiederverwendung (GOGC 100) | 9,9 s     | 1 094 MB |
| GOGC 50                                   | 11,7 s    | 968 MB   |
| GOGC 200                                  | 10,3 s    | 1 250 MB |

Der Schalter GOGC ist ein Zielkonflikt zwischen Zeit und Speicher. Der Standard
100 bleibt, weil er beide Werte am ausgewogensten hält. Für speicherknappe
Maschinen gibt es `-mem-limit`.

Bekannte Abweichung: Die Ergebnisse stimmen mit der Vorversion auf dem
Zod-Korpus exakt überein. Auf den anderen Korpora gibt es kleine Unterschiede:

- C# (Orchard): 36 Treffer mehr EXACT und 49 mehr RESOLVED. Ursache:
  `<see cref="Typ.Methode(A&lt;B,C&gt;)"/>` wurde bisher als zwei Parameter
  gezählt. Das ist jetzt korrekt, eine Verbesserung.
- TS (TypeScript-Repository): 17 Treffer weniger EXACT. Die genaue Ursache habe
  ich noch nicht vollständig geklärt. Sie betrifft einen kleinen Anteil und wird
  als Restposten geführt.

## P15 – Erweiterungen (später)

### P15a – Import aus .NET-XML-Dokumentation (umgesetzt)

- [x] Eigener Modus ohne Quelltext: `goreadmenator import-xml <datei.xml>`. Der
      Import ist unabhängig von Git.
- [x] Dokumentations-IDs (`T:`, `M:`, `P:`, `F:`, `E:`) werden in die Symbol-IDs
      des C#-Parsers übersetzt, inklusive Generics (`` `N ``), Konstruktoren
      (`#ctor`), Indexer (`this(...)`), Arrays und geschweifter Klammern.
- [x] Namensräume entstehen aus den Typen, Verschachtelung wird über die Datei
      erkannt.
- [x] Summary, Remarks, Returns, Parameter, Typparameter und `<see cref>` werden
      übernommen, Entities aufgelöst.
- [x] Fundort ist ein virtuelles Assembly (`bin/Name.dll`), Quelle `xml`.
- [x] Wiederholter Import ersetzt nur den eigenen früheren Stand.
- [x] Verschmelzung mit dem Quelltext: Felder aus dem Quelltext bleiben
      erhalten, fehlende werden ergänzt, Fundorte beider Quellen stehen
      nebeneinander.
- [x] Vollaufbau (`index`) entfernt den Import nicht, Tests decken das ab.
- [x] Tests: Parsing, Zuordnung der IDs, Container, Doku-Bereinigung,
      Parameter-Parser, Reimport, Verschmelzung, End-to-End über die CLI.

**Ergebnis (2026-10-10):** Paket `internal/xmldoc` (Parser, ID-Übersetzung),
`internal/xmlimport` (Schreiben in den Store), Befehl `import-xml`. Alle Tests
sind grün.

**Ehrliche Grenzen dieses Modus:**

- Die Art eines Typs (Klasse, Interface, Struktur, Enum) steht nicht in der
  XML-Datei. Typen erscheinen als `type-unknown`.
- Beziehungen fehlen: Vererbung, Interface-Implementierung, Feldtypen,
  Parametertypen, Rückgabetypen und Attribute stehen nicht in der XML. Dafür
  braucht es die Metadaten der DLL.
- Generische Parameter in der Dokumentation (`0`) werden nicht auf die Namen der
  Quelle abgebildet.
- Die XML-Datei kennt nur dokumentierte Member. Undokumentierte öffentliche
  Member fehlen.

### P15b – DLL-Metadaten (Umfang reduziert, Entscheidung 2026-10-10)

Ohne Quelltext wird der Code nicht bearbeitet, nur verwendet. Deshalb reicht die
Bereitstellung der öffentlichen API. Ein vollständiger Metadaten-Leser ist dafür
nicht nötig. Umfang: öffentliche Typen und Member aus der DLL, Art der Typen und
Basistypen. Feld- und Parametertypen nur, wenn sie für die API-Ansicht gebraucht
werden. Die Details werden mit dem Export-Konzept (P16) festgelegt.

Ursprünglicher Umfang (zurückgestellt):

- [ ] ECMA-335-Metadaten lesen (`debug/pe` für den PE-Container, eigener Leser
      für die Tabellen `TypeDef`, `MethodDef`, `Field`, `Property`, `Event`,
      `Param`, `InterfaceImpl`, `TypeRef`, `CustomAttribute`).
- [ ] Art der Typen bestimmen (Klasse, Interface, Struktur, Enum, Delegate),
      Vererbung und Interface-Implementierung ableiten, Feld-, Parameter- und
      Rückgabetypen als Beziehungen ableiten.
- [ ] Attribute als Beziehungen übernehmen.
- Das wäre der Schritt, der aus dem XML-Modus einen vollständigen Modus macht.
  Aufwand: größer als P15a, weil die Tabellen-Layouts und die Kodierung der
  Indizes exakt stimmen müssen.

### P15c – Weitere Erweiterungen (offen)

- [ ] Unkommittierter Arbeitsbaum als Pseudo-Commit (Diff gegen HEAD).
- [ ] Watch-Modus mit Polling oder `fsnotify` (Abhängigkeitsentscheidung neu
      treffen).
- [ ] Weitere Sprachen nach Bedarf.

## P16 – Bereitstellung für KI-Assistenten (MCP und Abfrageschicht)

- [x] Abfrageschicht `internal/query`: spricht ausschließlich über
      `store.Store`, damit ein SQLite-Backend nur die Implementierung ersetzt.
- [x] Token-Budget je Antwort, Standard 2500 Tokens (Schätzung 4 Zeichen je
      Token), einstellbar von 200 bis 8000.
- [x] Paginierung mit `nextCursor`, damit große Ergebnisse nur seitenweise
      geladen werden.
- [x] Werkzeuge: `find_symbols`, `get_symbol`, `api`, `neighborhood` (Tiefe 1
      bis 3, Beziehungsarten wählbar), `usages` (Tiefe 1 bis 3), `context`
      (Symbol, Beziehungen und Verwendungen im Budget), `source` (höchstens 80
      Zeilen aus dem HEAD), `changes` und `status`.
- [x] MCP-Server über stdio mit JSON-RPC, nur Standardbibliothek,
      Protokollversion 2024-11-05. Befehl `goreadmenator mcp`.
- [x] Nur Lesezugriff: der Server schreibt den Index nicht zurück. Änderungen am
      Index werden vor jedem Aufruf erkannt und neu geladen.
- [x] Fehler bei Werkzeugen kommen als Ergebnis mit `isError`, damit der
      Assistent die Meldung lesen kann. Mehrdeutige Namen nennen die Kandidaten.
- [x] Veraltetes Snapshot-Format wird erkannt, beiseitegelegt und durch einen
      Neuaufbau ersetzt.
- [x] Tests: Budget und Paginierung ohne Doppelte über Seiten, Kontextpaket im
      Budget, Quelltext-Grenze, Status, Protokoll-Handshake, Werkzeugliste,
      Fehlerfälle.

**Ergebnis (2026-10-10):** Pakete `internal/query`, `internal/mcp`, Befehl `mcp`
in der CLI. Alle Tests sind grün.

Messung auf MediatR (Ende-zu-Ende über den Server):

| Anfrage                              | Antwort (ca. Tokens)         |
| ------------------------------------ | ---------------------------- |
| `find_symbols "IRequestHandler"`     | 234                          |
| `context` für `IRequestHandler<,>`   | 1 496                        |
| `api` unter `MediatR.` (Budget 2500) | 2 505 (begrenzt, mit Cursor) |
| `usages` Tiefe 2                     | 1 366                        |
| `source` einer Datei mit 38 Zeilen   | 432                          |

Zum Vergleich: Die Datei `ServiceRegistrar.cs` hat 24,6 KB (grob 6 000 Tokens),
das gesamte Repository 423 KB (grob 105 000 Tokens). Ein Kontextpaket für ein
Interface kostet also rund ein Viertel der Datei und enthält Beziehungen und
Verwendungen, die im Quelltext verstreut wären.

**Offen**

- Der Index wird komplett im Speicher gehalten. Mit SQLite fällt das weg, die
  Abfragen bleiben gleich.
- Die Namenssuche läuft über alle Symbole. Mit SQLite würde sie als indizierte
  LIKE-Abfrage laufen.
- Die Token-Schätzung ist eine Faustregel. Ein echter Tokenzähler wäre genauer.
- Eine HTTP-Schnittstelle für andere LLMs ist noch nicht gebaut. Sie wäre eine
  dünne Hülle über dieselbe Abfrageschicht.

## P17 – Mehrere Projekte über eine API

Entscheidung: je Projekt ein eigener Index, eine gemeinsame API (MCP-Server)
über eine Projektregistry. Nicht eine gemeinsame Datenbank mit Mandantenspalte.

Begründung:

- Symbol-IDs sind nur innerhalb eines Repositories eindeutig. Dieselbe FQN kann
  in zwei Projekten vorkommen. Eine gemeinsame Datenbank bräuchte zusätzlich ein
  Projektpräfix in jeder ID.
- Ein Projekt lässt sich neu aufbauen oder entfernen, ohne die anderen
  anzufassen.
- Der Speicher bleibt pro Projekt klein. Projekte werden erst bei Bedarf
  geladen.
- Projektübergreifende Abfragen bleiben möglich, indem der Server mehrere
  Indizes nacheinander befragt. Eine gemeinsame Datenbank wird erst nötig, wenn
  echte Beziehungen zwischen Projekten gebraucht werden (etwa eine geteilte
  Bibliothek). Dann mit SQLite und Projektspalte.

- [x] Projektregistry (`~/.goreadmenator/projects.json`, überschreibbar mit
      `-registry`). Namen sind validiert, der Index liegt standardmäßig im
      Git-Verzeichnis des Repos.
- [x] Befehle `project add <name> <repo> [-db]`, `project list`,
      `project remove <name>`.
- [x] MCP-Server bedient alle registrierten Projekte. Jedes Werkzeug akzeptiert
      `project`. Ohne Angabe gilt das einzige Projekt, bei mehreren kommt eine
      Fehlermeldung mit der Auswahl.
- [x] Werkzeug `projects` listet die verfügbaren Projekte.
- [x] Projekte werden lazy geladen und vor jedem Aufruf auf Änderungen des Index
      geprüft.
- [x] Mit `-repo` startet der Server nur dieses eine Projekt
      (Rückwärtskompatibilität).
- [x] Tests: Registry (Roundtrip, ungültige Namen und Pfade), Projektauswahl und
      Fehler, Werkzeug `projects`, Fehler als Ergebnis.

**Ergebnis (2026-10-10):** Pakete `internal/registry`, Erweiterungen in
`internal/mcp` und `internal/cli`. Alle Tests sind grün.

Ende-zu-Ende mit zwei Projekten (MediatR mit 1583 Symbolen, Orchard mit 32 224
eindeutigen Symbolen und 312 597 Verwendungen): Der Server listet beide, lehnt
eine Anfrage ohne Projekt mit Auswahl ab und liefert Antworten für das gewählte
Projekt.

Offen: projektübergreifende Abfragen in einem Aufruf (heute je Projekt einzeln),
HTTP-Schnittstelle.

## P18 – Golden-File-Integrationstests je Parser

Jeder Testfall ist eine Eingabedatei in `testdata/` mit einer erwarteten Ausgabe
(`*.golden.txt`). Die Eingabedatei beginnt mit Kommentaren `SZENARIO` und
`ABDECKUNG`, die beschreiben, was der Fall abdeckt. Die Übersicht aller Fälle
steht in `testdata/COVERAGE.md`, sie wird beim Aktualisieren neu erzeugt.

- [x] Gemeinsamer Helfer `internal/parser/golden`: deterministische Textausgabe
      (Symbole, Fundorte, Verwendungen, Beziehungen, Imports, Diagnosen),
      Vergleich mit zeilenweiser Abweichungsanzeige, Übersicht.
- [x] C#: 10 Szenarien (Namensräume, Typarten, Member, Vererbung und Attribute,
      partial, XML-Doku, Maskierung, Verwendungen, kaputter Code, BOM und CRLF).
- [x] TypeScript/JavaScript: 11 Szenarien (Imports und Exporte, Funktionen,
      Klassen, Interfaces, Typen und Enums, Maskierung, JSDoc, JSX,
      Verwendungen, BOM und Shebang, kaputter Code).
- [x] Aktualisieren nach bewusster Änderung:
      `READMENATOR_UPDATE_GOLDEN=1 go test ./internal/parser/...`. Abweichungen
      zeigen die Zeile mit erwartetem und erhaltenem Stand.

**Beim Durchsehen der Ausgaben gefundene und behobene Fehler:**

- C#: Namensraum nach Attribut `[assembly: ...]` wurde als Feld gelesen (Symbol
  `.Shop`). Behoben.
- C#: lokale Deklaration `var o = ...` erzeugte einen Schreibzugriff. Behoben.
- C#: Basisklasse `object` erzeugte einen Vererbungsverweis auf ein
  Nicht-Projektsymbol. Behoben.
- C#-Dokumentation: XML-Entities (`&amp;`) blieben stehen, `<seealso cref>`
  verschwand im Text, generische Doku-Namen mit geschweiften Klammern
  (`Processor{T}`) wurden nicht zu spitzen Klammern. Behoben.
- TS/JS: `export default function` erhielt den Modifikator `default` zweimal.
  Behoben.
- TS/JS: Aliase in Export-Listen (`export { b as c }`) erzeugten zusätzlich eine
  falsche Bindung. Behoben.
- TS/JS: Apostroph im JSX-Text (`Don't`) öffnete einen String und verschluckte
  die restliche Zeile, dadurch fehlten Komponenten. Behoben.
- TS/JS: Eine nicht geschlossene Klasse verschluckte alle folgenden
  Deklarationen. Jetzt endet der Körper vor der nächsten Deklaration in
  Spalte 1. Behoben.

**Ergebnis (2026-10-10):** 21 Szenarien, alle grün. Die Golden-Dateien sind nach
dem Durchsehen Verträge: Eine Abweichung ist entweder ein Fehler oder eine
bewusste, dokumentierte Änderung.

Bekannte Lücken, die die Fälle sichtbar machen:

- C#: Positionsparameter eines `record` (`record Order(int Id, ...)`) werden
  nicht als Symbole geführt.
- C#: Assembly-Attribute ohne Ziel-Symbol werden nicht erfasst.
- TS: Gleiche Überladungen mit gleichen Parametertypen teilen eine ID
  (zusammengeführte Fundorte, gewollt).

## P19 – Golden-Tests für Auflösung und Abfragen

Dasselbe Muster wie bei den Parsern, angewendet auf ganze Verzeichnisse. Jeder
Fall ist ein Ordner in `testdata/` mit Quelldateien, `scenario.txt` (SZENARIO
und ABDECKUNG) und einer erwarteten Ausgabe `expected.golden.txt`. Abfragen
stehen zusätzlich in `queries.txt`. Die Übersicht steht in `COVERAGE.md`.

- [x] Auflösung: 9 Szenarien (Scope, using, Überladungen, Generics und cref,
      partial und Vererbung, TS-Imports, Re-Exporte mit tsconfig-Pfaden,
      Heuristik, Beziehungen).
- [x] Abfragen: 3 Szenarien (Suche und Details, Budget und Paginierung,
      Beziehungen und Kontext). Der Testtreiber ersetzt `$NEXT` durch den Cursor
      der vorherigen Antwort, damit die Paginierung in einem Durchlauf geprüft
      wird.
- [x] Commit-Hashes werden normalisiert, damit die Ausgabe reproduzierbar
      bleibt.

**Beim Durchsehen gefundene und behobene Fehler:**

- Stern-Re-Export (`export * from "./model"`) wurde mit einem benannten
  Re-Export vom selben Pfad zusammengeführt. Der Stern-Export ging dadurch
  verloren, und Importe über das Barrel blieben unaufgelöst.
- Unaufgelöste Verwendungen behielten die Konfidenz EXACT, die der Parser für
  Imports und cref gesetzt hatte. Jetzt gilt: unaufgelöst heißt HEURISTIC.
- Doku-Verweise mit vollem Namen (`<see cref="Mediator.Ping"/>`) wurden nicht
  gefunden. Sie werden jetzt über den vollqualifizierten Namen aufgelöst.
- `api` listete öffentliche Member interner Typen. Die Sichtbarkeit wird jetzt
  über alle umgebenden Typen geprüft.
- Parameter-Dokumentation wurde mit groß geschriebenen JSON-Schlüsseln (`Name`,
  `Text`) ausgegeben. Jetzt einheitlich klein.
- Abfragen mit Parameterliste (`Start(System.Int32)`) fanden kein Symbol. Die
  Liste wird jetzt für die Suche abgetrennt.

**Ergebnis (2026-10-10):** 12 Szenarien, alle grün. Gesamte Testsuite grün.

## P20 – HTTP-Schnittstelle

Dünne Hülle über dieselbe Abfrageschicht wie der MCP-Server. Damit können auch
andere Sprachmodelle und Werkzeuge den Graphen abfragen.

- [x] Nur lesend: jede Methode außer GET und HEAD wird mit 405 abgelehnt.
- [x] Endpunkte je Projekt unter `/v1/{projekt}/...`: `symbols`, `symbol`,
      `api`, `neighborhood`, `usages`, `context`, `source`, `changes`, `status`.
      Zusätzlich `/v1/projects`.
- [x] Dieselben Grenzen wie bei MCP: Token-Budget (Parameter `budget`),
      Paginierung über `cursor`, Quelltext höchstens 80 Zeilen.
- [x] Fehler mit passendem Status: 404 für unbekanntes Projekt oder Symbol, 409
      für mehrdeutige Namen (Kandidaten in der Meldung), 503 für einen Index,
      der nicht geladen ist.
- [x] Optionales Bearer-Token (`-token` oder `READMENATOR_TOKEN`). Ohne Token
      und mit Bindung an eine Nicht-Loopback-Adresse gibt der Server eine
      Warnung aus.
- [x] Standardmäßig Bindung an `127.0.0.1:7878`.
- [x] Beschreibung im OpenAPI-Format unter `/v1/openapi.json`, knapp gehalten,
      damit sie wenig Kontext verbraucht.
- [x] Anfragen werden nacheinander bearbeitet, weil die Abfrageschicht
      Zwischenstände hält. Für ein lokales Werkzeug reicht das.
- [x] Tests: Projekte und Status, Suche und Details, Fehlerstatus,
      schreibgeschützt und Token, OpenAPI.

**Ergebnis (2026-10-10):** Paket `internal/httpapi`, Befehl `serve` in
`internal/cli`. Alle Tests sind grün.

Ende-zu-Ende mit zwei Projekten über curl geprüft: Projektliste, Suche mit
Budget, Details, unbekanntes Projekt mit 404.

Beispiele:

```
curl localhost:7878/v1/projects
curl "localhost:7878/v1/mediatr/symbols?q=IRequestHandler&budget=500"
curl "localhost:7878/v1/mediatr/context?ref=csharp:MediatR.IRequestHandler%602&budget=2500"
goreadmenator -registry ~/.goreadmenator/projects.json serve -addr 0.0.0.0:7878 -token geheim
```

Offen: Die Schnittstelle nutzt keine TLS. Für den Zugriff über das Netz gehört
sie hinter einen Reverse-Proxy mit HTTPS.

## P21 – VB.NET-Parser (als Probe für die Anleitung)

Der Parser wurde nach `docs/PARSER-HINZUFUEGEN.md` gebaut. Die Arbeit hat drei
Dinge ans Licht gebracht, die in der Anleitung fehlten oder falsch waren. Sie
sind dort korrigiert.

- [x] `internal/parser/vbnet`: Lexer (Anweisungen, Zeilenfortsetzung,
      Doppelpunkt, Dokumentation mit drei Apostrophen, Bezeichner in eckigen
      Klammern), Deklarationen (Namensräume, Klassen, Module, Strukturen,
      Interfaces, Enums, Sub und Function, Property, Event, Dim und Const),
      Verwendungen, Imports, Attribute.
- [x] Golden-Tests: 10 Szenarien mit SZENARIO- und ABDECKUNG-Kommentaren,
      Übersicht in `testdata/COVERAGE.md`.
- [x] Resolver-Testfall `10_vbnet_case_insensitive` für Schreibweise und
      Sprachgrenze.

**Korrekturen an der Anleitung und am Code, die sich dabei ergaben:**

- Parser-Registrierung war an sechs Stellen kopiert. Jetzt eine Funktion in
  `internal/parsers`, CLI und Tests nutzen sie.
- Die Auflösung unterschied nur C# und TS. Sprachen ohne eigenen Zweig fielen
  unbemerkt auf die C#-Regeln. Die Anleitung nennt den Punkt in `resolveRef`.
- Namen waren für case-insensitive Sprachen nicht auffindbar. Schlüssel sind
  jetzt sprachgebunden und für VB kleingeschrieben (`fold`).
- Attribute auf eigener Zeile wurden nicht angehängt, die Dokumentation fehlte
  deshalb. Behoben, die Anleitung beschreibt es.
- Zeilenorientierte Blöcke ohne End verschluckten die restliche Datei. Erkennung
  auf Ebene 0 eingebaut, die Anleitung beschreibt sie.
- Golden-Szenariokommentare mit Apostroph wurden nicht gelesen. Behoben.
- `ParserVersion` auf `p21-1` erhöht.

**Bekannte Grenzen:** Verwendungen über lokale Variablen bleiben HEURISTIC. Die
Sprachgrenze gilt bewusst: VB löst C#-Symbole nicht auf, auch im selben Projekt.

## P22 – Mehrsprachige Projekte

Ein Projekt kann mehrere Sprachen enthalten, etwa ASP.NET mit C#, VB.NET und
TypeScript. Die Trennung nach Sprachen war zu streng.

- [x] Sprachfamilien: C# und VB.NET teilen die Familie `dotnet`. Ein Typ aus C#
      ist aus VB sichtbar und umgekehrt. TypeScript und JavaScript bilden eigene
      Familien.
- [x] C# bleibt case-sensitiv: Die Schreibweise wird nach dem Lookup exakt
      geprüft (`exactName`). VB bleibt case-insensitiv.
- [x] Aufrufe über Felder, Eigenschaften und Variablen: Der Resolver läuft in
      zwei Runden. Die erste löst Typen auf, die zweite nutzt den Typ des Feldes
      (`svc.Save()` über den Typ von `svc`).
- [x] Zwei Stellen, an denen die Änderung fast brach, abgefangen: Qualifier
      wurde durch die Schreibweisenprüfung ausgeschlossen, Aliase durch den
      Namensvergleich. Die Import-Zähler wurden beim Zurücksetzen gelöscht.
- [x] Testfall `11_mixed_languages_web`: C#-Controller, VB-Service und
      TypeScript-Datei in einem Projekt. Richtige Schreibweise über Sprachgrenze
      aufgelöst, falsche in C# bleibt offen, TypeScript ohne Querverbindung.
- [x] Anleitung um Familien und die zweite Auflösungsrunde ergänzt.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Das gemischte Testprojekt
indiziert in einem Lauf: 55 EXACT, 4 RESOLVED, 12 HEURISTIC, 97 unaufgelöst.

**Offen:** Razor- und ASPX-Dateien werden noch nicht geparst. Die Verknüpfung
von TypeScript mit den HTTP-Routen des Backends fehlt ebenfalls.

## P23 – Sprachversion prüfen (warnen statt abbrechen)

Ausgangslage: Die Konstante `ParserVersion` steuerte nur den Neuaufbau des
Index. Die Sprachversion wurde nirgends geprüft.

- [x] Parser melden ihre Abdeckung über `Coverage()`. C# meldet bis Version 13.
      VB.NET und TypeScript melden bewusst nichts, dort findet keine Prüfung
      statt.
- [x] Diagnosen haben einen Schweregrad. Teilweise erfasste Konstrukte sind
      Warnungen, Fehler bleiben Fehler.
- [x] C#-Konstruktwarnungen: primärer Konstruktor (C# 12), Positionsparameter
      eines Records (C# 9), file-lokaler Typ (C# 11).
- [x] Projektversion aus `.csproj`: `LangVersion` oder der Standard aus
      `TargetFramework` (net8 → 12, net9 → 13, net10 → 14, .NET Framework →
      7.3). Mehrere Frameworks: der höchste zählt. `latest` und `preview` gelten
      als neueste bekannte Version und bekommen einen Hinweis.
- [x] Warnungen erscheinen nach dem Indexieren mit Datei und Zeile,
      Projektwarnungen zuerst. Die Abfrage `status` (MCP und HTTP) liefert die
      Projektwarnungen als `langWarnings`.
- [x] Die Prüfung bricht nichts ab.
- [x] Fehler gefunden und behoben: Der file-lokale Typ wurde vorher ganz
      verworfen, weil `file` kein Modifikator war. Der Typ wird jetzt erfasst.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Ende-zu-Ende an einem Projekt
mit net10.0: Projektwarnung für C# 14 und die Warnung für den primären
Konstruktor mit Zeilenangabe.

**Bekannte Grenzen:**

- Die Konstruktliste ist von Hand gepflegt. Sie deckt nur die oben genannten
  Konstrukte ab. Neuere C#-Konstrukte werden nicht erkannt, nur über den
  Projekthinweis.
- Die Zuordnung von TFM zu C#-Version ist eine feste Tabelle. Neue
  .NET-Versionen müssen ergänzt werden.
- Warnungen aus dem Indexlauf sind nicht gespeichert. Die Projektwarnungen in
  `status` werden bei jeder Abfrage neu berechnet.
- TypeScript und VB.NET haben noch keine Versionsprüfung.

**Nächste Schritte:** TypeScript (`tsconfig` `target`) und VB.NET (`LangVersion`
in `.vbproj`) in derselben Form.

## P24 – Sprachversion für TypeScript und VB.NET

- [x] TypeScript/JavaScript: Abdeckung ES2022. Projektversion aus
      `tsconfig.json` (`target`). `ESNext` zählt als neuestes bekanntes Jahr
      (2024) und warnt. ES-Namen werden in Jahre umgerechnet (`es6` → 2015).
- [x] VB.NET: Abdeckung 16.9 (bis zu dieser Version sind die Golden-Fälle
      getestet). Projektversion nur aus `LangVersion` in der `.vbproj`. Ohne
      Angabe wird nichts geraten, es erscheint ein Hinweis.
- [x] Eine gemeinsame Prüfung `langver.Warnings()` für alle drei Sprachen,
      genutzt von Indexlauf und `status`.
- [x] Tests: ES-Umrechnung, tsconfig mit Kommentaren und nachgestelltem Komma,
      VB-LangVersion, gemeinsame Warnungen in einem Projekt mit allen drei
      Sprachen.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Im Testprojekt erscheinen
Warnungen für C# 14 und ES2024 gleichzeitig.

**Bekannte Grenzen:**

- Bei TypeScript wird nur der `target` gelesen. Der Wert bestimmt, welche
  ES-Syntax das Ziel erlaubt, nicht die TypeScript-Compilerversion aus
  `package.json`. Die Prüfung folgt damit dem Ziel, nicht dem Compiler.
- `extends` in `tsconfig.json` wird nicht aufgelöst. Ein `target` nur in der
  Basisdatei bleibt unerkannt.
- Für VB.NET gibt es keine Konstruktwarnungen, nur den Projekthinweis.
- Die Abdeckungsgrenzen (ES2022, VB 16.9) sind Einschätzungen aufgrund der
  getesteten Golden-Fälle, keine vollständigen Prüfungen.

## P25 – Konstruktwarnungen für TypeScript und VB.NET (Prüfung)

Vorgehen: Eine Sondierung mit neueren Konstrukten, die Ergebnisse wurden gegen
die erwarteten Symbole und Verwendungen geprüft. Nur wo tatsächlich Information
verloren geht, sollte gewarnt werden. Eine Warnung ohne Verlust wäre falsch.

- [x] TypeScript geprüft: const-Typparameter, accessor-Felder, statische Blöcke,
      override, Getter mit optional chaining, satisfies, Nullish-Zuweisung,
      #private-Methoden, Top-Level using. Alle Symbole und Aufrufe werden
      erfasst. Keine Warnung nötig.
- [x] VB.NET geprüft: async/await, Tupel als Rückgabetyp, Iterator/Yield,
      $-Interpolation, If als Ausdruck, NameOf.
- [x] VB-Fehler behoben: Benannte Tupel-Elemente (`Left As Integer`) wurden als
      Typverweise `Left` und `Right` geführt. NameOf wurde als Aufruf gezählt.
- [x] Golden-Fälle für beide neuen Sammlungen, damit spätere Änderungen sichtbar
      werden.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Keine Konstruktwarnung für
TypeScript und VB.NET, weil in der Sondierung kein Verlust gefunden wurde. Der
Warnmechanismus steht bereit und wird ergänzt, sobald ein Konstrukt nachweislich
verloren geht.

**Bekannte Grenzen:** Die Sondierung deckt nur die aufgeführten Konstrukte ab.
Weitere neue Syntax kann es geben, für die noch kein Golden-Fall existiert.

## P26 – ASP.NET: Razor und ASPX

- [x] Razor (`.cshtml`, `.razor`): Direktiven `@using`, `@model`, `@inject`,
      `@inherits`, `@code`/`@functions`, `@{ }`-Blöcke, Inline-Ausdrücke
      `@name`, `@(...)`, Methodenaufrufe, Kontrollblöcke (`@if`, `@foreach`,
      ...), Escape `@@`, Kommentare `@* *@`.
- [x] ASPX (`.aspx`, `.ascx`, `.master`): `<%@ Import %>`,
      `<%@ Page Inherits %>`, `<%= %>`, `<% %>`, `<%-- --%>`,
      `<script runat="server">`.
- [x] Der Codeanteil läuft über den vorhandenen C#-Parser. Zeilen und Spalten
      der Vorlage bleiben erhalten, Fundorte werden über eine Zeilenzuordnung
      zurückgeführt.
- [x] Sprachen `razor` und `aspx` gehören zur Familie `dotnet`. Aufrufe aus
      Razor auf injizierte Dienste werden über den Feldtyp exakt aufgelöst.
- [x] Golden-Fälle: 3 Vorlagen. Auflösungsfall `12_aspnet_razor` mit Modell,
      injiziertem Dienst und `@code`-Methode.
- [x] Fehler behoben: Klassennamen mit führender Ziffer (`01_...`) ergaben
      `_025` statt `_01`. Ziffern bleiben jetzt erhalten.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Beispielprojekt indiziert: 12
Symbole, 2 EXACT, 1 RESOLVED, 1 HEURISTIC, 1 Beziehung.

**Bekannte Grenzen:**

- HTML innerhalb von Kontrollblöcken wird nur über Zeilen erkannt, die mit einem
  Tag beginnen. Ein Tag mitten in einer Codezeile kann dort Code verschlucken.
- Qualifizierte Typnamen wie `Shop.Models.Order` werden auf den letzten Teil
  reduziert. Das ergibt HEURISTIC statt EXACT. Das betrifft auch normalen
  C#-Code und ist eine eigene Verbesserung wert.
- Code-behind-Dateien (`.aspx.cs`) sind normale C#-Dateien und werden bereits
  erfasst, ihre Verknüpfung mit der Seite fehlt aber noch.
- Blazor-Komponenten (`.razor`) und Razor-Pages werden gleich behandelt.
  Spezifische Blazor-Konzepte wie `[Parameter]` sind nicht eigens ausgewertet.

## P27 – Qualifizierte Typnamen

- [x] Der C#-Parser liefert für `Shop.Models.Order` einen Verweis mit dem vollen
      Namen, bisher nur `Order`. Bei Generics bleibt die Arity beim letzten
      Teil.
- [x] Der Resolver sucht den vollen Namen zuerst vom innersten Namensraum nach
      außen. Dabei werden die Präfixe aus der FQN jedes umgebenden Symbols
      gebildet. Das macht auch relative Namen wie `Models.Order` innerhalb von
      `Shop` exakt auflösbar, obwohl `Shop` selbst nicht als Symbol existiert.
- [x] Generic-Typen mit qualifiziertem Namen werden über die Arity gefunden
      (`Box`1`).
- [x] Testfall `13_qualified_names` mit zwei gleichnamigen Typen
      (`Shop.Models.Order` und `Archive.Order`). Alle Verweise sind EXACT.
- [x] Die Razor-Auflösung `@model Shop.Models.Order` ist jetzt EXACT, vorher
      HEURISTIC.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Nur der erwartete Golden-Fall
(Razor) hat sich geändert, alle anderen blieben gleich.

**Bekannte Grenzen:** Ein qualifizierter Name, der über ein `using` eines
Namensraums aufgelöst werden müsste (etwa `using Shop;` und dann
`Models.Order`), wird noch nicht gefunden. Die Suche läuft nur über die
umgebenden Namensräume. Diese Lücke sollte als Nächstes geschlossen werden.

## P28 – Teilnamen über using-Direktiven

Idee aus dem Gespräch: Ein Teilname wie `Models.Order` wird zuerst gegen die
`using`-Direktiven gesucht. Trifft genau einer zu, ist das Ergebnis eindeutig.
Treffen mehrere zu, meldet der Compiler CS0104, also wird nichts geraten. Ohne
Treffer gilt die bisherige Suche.

- [x] Teilnamen werden über alle `using`-Namensräume der Datei gesucht (ohne
      Alias und ohne `static`).
- [x] Genau ein Treffer: RESOLVED. Mehrere Treffer: offen. Kein Treffer:
      bisherige Suche (Namensraum-Präfixe, dann eindeutiger Name im Projekt).
- [x] Testfall `14_using_partial_names`: eindeutiger Teilname über `using Shop`,
      fehlendes `using` bleibt HEURISTIC, zwei `using`-Namensräume mit gleichem
      Typ bleiben offen.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Keine anderen Goldens haben
sich geändert.

**Ehrlicher Hinweis zur Reihenfolge:** Der Compiler prüft Namensräume ebene für
ebene. Eine `using`-Direktive der innersten Deklaration kommt vor den
Mitgliedern äußerer Namensräume. Unsere Suche prüft zuerst die umgebenden
Namensräume, dann die `using`-Direktiven. Der Unterschied tritt nur auf, wenn
ein Typ im umgebenden Namensraum und ein gleichnamiger Typ über `using` beide
passen. Der Fall ist selten, wir folgen deshalb deiner Reihenfolge. Sollte er in
der Praxis auftreten, lässt sich die Reihenfolge ebene für ebene nachbilden.

**Bekannte Grenzen:** Alias-Direktiven mit Teilnamen (`using X = Shop;` und dann
`X.Models.Order`) sind noch nicht eigens behandelt.

## P29 – Alias-Teilnamen, ASPX-Code-behind, HTTP-Verbindung Frontend–Backend

- [x] **Alias-Teilnamen:** `using X = Shop;` und `X.Models.Order` werden über
      den Alias aufgelöst (EXACT über die using-Auflösung, RESOLVED).
- [x] **ASPX-Code-behind:** `Inherits` einer ASPX-Seite ist die Klasse der Seite
      selbst. Der generierte Teil trägt diesen Namen und wird mit
      `Seite.aspx.cs` zur partiellen Klasse zusammengeführt. Die fälschliche
      Basisklasse entfällt.
- [x] **HTTP-Routen im Backend:** `[HttpGet/Post/Put/Delete/Patch]` mit Pfad,
      `[Route("api/[controller]")]` am Controller mit Ersetzung von
      `[controller]`, absolute Pfade mit führendem `/`, Minimal API
      `MapGet/Post/Put/Delete/Patch` auch in Top-Level-Anweisungen
      (`Program.cs`).
- [x] **HTTP-Aufrufe im Frontend:** `fetch` (auch mit `method` im
      Optionsobjekt), `axios.<verb>`, `http.<verb>`, als Literal, Verkettung
      (`"/api/orders/" + id`) oder Template (`` `/api/orders/${id}` ``).
      Query-Strings werden entfernt.
- [x] **Verbindung:** Gleiches Verb, gleiche Segmentzahl, Parameter passen auf
      Parameter. Nur bei genau einem Treffer entsteht eine Beziehung
      `calls_endpoint`. Mehrdeutige Routen bleiben unverbunden.
- [x] **Abfrage:** Endpunkt-Liste mit Route, Aktion, Datei, Zeile und Aufrufern.
      Verfügbar als MCP-Werkzeug `endpoints` und als `/v1/{projekt}/endpoints`.
- [x] **Präzisionsfix:** Ein Aufruf auf einen Typnamen, den das Projekt nicht
      kennt (`WebApplication.Create()`), wird nicht mehr über den eindeutigen
      Namen im Projekt geraten. Vorher traf er einen gleichnamigen
      Controller-Methode.
- [x] **Top-Level-Anweisungen:** `var app = ...;` wird als lokale Anweisung
      behandelt statt als Feld. Verwendungen zählen zum synthetischen Symbol
      `<top-level>@Datei`.
- [x] Golden-Fälle: Parser C# (Routen), Parser TS (Aufrufe), Abfrage (Endpunkte
      und Status), Auflösung (Verbindung mit Mehrdeutigkeit).

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Im Testprojekt sind alle
eindeutigen Routen mit den Aufrufen verbunden, die mehrdeutige Route
`shared/item` bleibt bewusst offen.

**Bekannte Grenzen:**

- Konventionsrouten (`MapControllerRoute` mit `{controller}/{action}`) werden
  nicht ausgewertet.
- Razor-Seiten mit `@page "/pfad"` liefern noch keine Route.
- `http.get<T>(...)` mit Typargument wird nicht erkannt.
- Aufrufe mit rein dynamischer URL (`fetch(url)`) bleiben ohne Verbindung, auch
  wenn die Route passen könnte.
- Top-Level-Anweisungen erzeugen weiterhin ein Symbol pro Datei, das die
  Anweisungen zusammenfasst. Es gibt keine Unterscheidung nach einzelnen
  Anweisungen.

## P30 – Konventionsrouting und Razor-@page

- [x] **MapControllerRoute:** Das Muster wird gelesen (benannt `pattern:` oder
      positional). Platzhalter `{controller}` und `{action}` werden durch die
      Aktion ersetzt, weitere Parameter zu `{}`. Optionale Parameter (`{id?}`,
      `{x=...}`) am Ende erzeugen zusätzlich die kürzere Variante ohne sie.
- [x] **Öffentliche Aktionen eines Controllers** ohne Attributroute werden über
      das Muster erreichbar, mit dem Verb `ANY`.
- [x] **Aktionen unter Klassenroute:** `[Route("api/x")]` am Controller ohne
      Verb an der Aktion macht die Aktion unter genau diesem Pfad erreichbar.
- [x] **Razor `@page "/pfad"`:** Die Seite ist unter ihrem Pfad erreichbar, der
      Verweis hängt an der Seitenklasse.
- [x] **Abfrage:** Die Endpunkt-Liste enthält auch die Konventionsrouten, mit
      den Aufrufern aus dem Frontend.
- [x] **Fehler behoben:** Controller- und Aktionssegment wurden fälschlich als
      optional behandelt, dadurch fiel der Pfad `/home/privacy` auf `/home`
      zurück.
- [x] Testfall `16_conventional_routes`: benanntes Muster, Aktion mit Parameter,
      Aktion ohne Parameter, Controller mit Klassenroute, `@page` mit Parameter,
      private Methode bleibt ohne Route.

**Ergebnis (2026-10-10):** Gesamte Testsuite grün. Im Testfall sind alle
erwarteten Aufrufe verbunden, die private Methode und der Controller mit
Klassenroute bleiben korrekt offen.

**Bekannte Grenzen:**

- Mehrere `MapControllerRoute`-Aufrufe mit Defaults (`{controller=Home}`) werden
  nur über das Muster gelesen, der Standard für den Stammpfad `/` wird nicht als
  Route angelegt.
- Areas (`MapAreaControllerRoute`) sind nicht erfasst.
- Eine Aktion, die über mehrere Muster erreichbar ist, bekommt mehrere Routen.
  Sind Aufrufe dadurch mehrdeutig, bleiben sie unverbunden.

## Risiken

| Risiko                                               | Gegenmaßnahme                                                                       |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Regex-Parser liefern bei C# und TS Fehlzuordnungen   | Vorverarbeitung für Kommentare und Strings, Fixtures pro Konstrukt, Konfidenzstufen |
| Verwendungsauflösung ohne Compiler ist unvollständig | Konfidenzstufen ehrlich ausweisen, `target_name` für spätere Neuauflösung           |
| Inkrement weicht vom Vollaufbau ab                   | Testpflicht: Vergleich mit frischem Vollaufbau am selben Commit                     |
| SQLite-Schreibengpass                                | Ein Writer, Batches in Transaktionen                                                |
| Force-Push macht gespeicherten Commit ungültig       | Fallback auf Vollaufbau (P10)                                                       |

## Änderungsprotokoll

| Datum      | Phase | Änderung                                                                                                                                              |
| ---------- | ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-10-10 | –     | Plan erstellt                                                                                                                                         |
| 2026-10-10 | P30   | Konventionsrouting (MapControllerRoute, öffentliche Aktionen), Razor-@page-Routen, Aktionen unter Klassenroute                                        |
| 2026-10-10 | P29   | Alias-Teilnamen, ASPX-Code-behind über Inherits, HTTP-Verbindung Frontend–Backend (Routen, Aufrufe, Endpunkt-Abfrage), Präzisionsfix für fremde Typen |
| 2026-10-10 | P28   | Teilnamen über using-Direktiven: eindeutiger Treffer, mehrdeutig bleibt offen, Fallback                                                               |
| 2026-10-10 | P27   | Qualifizierte Typnamen: vollständiger Name im Verweis, Auflösung vom innersten Namensraum nach außen, Generic-Arity                                   |
| 2026-10-10 | P26   | ASP.NET: Razor und ASPX über den C#-Parser, Zeilenzuordnung, Golden- und Auflösungsfälle                                                              |
| 2026-10-10 | P25   | Prüfung neuerer TS/VB-Konstrukte: zwei VB-Fehler behoben (Tupelnamen, NameOf), keine Warnung nötig, Golden-Fälle ergänzt                              |
| 2026-10-10 | P24   | Sprachversionsprüfung für TypeScript (tsconfig target) und VB.NET (LangVersion), gemeinsame Prüfung für alle Sprachen                                 |
| 2026-10-10 | P23   | Sprachversionsprüfung: Parser-Abdeckung, Konstruktwarnungen (C#), Projektversion aus .csproj, Warnungen statt Abbruch                                 |
| 2026-10-10 | P22   | Mehrsprachige Projekte: Sprachfamilien, Verwendungen über Sprachgrenzen, Aufrufe über Felder (zwei Auflösungsrunden)                                  |
| 2026-10-10 | P21   | VB.NET-Parser nach der Anleitung, Parser-Registrierung zentralisiert, case-insensitive Auflösung, Anleitung korrigiert                                |
| 2026-10-10 | P20   | HTTP-JSON-Schnittstelle (nur lesend, Projekte, Budget, Fehlerstatus, Token, OpenAPI, Befehl serve)                                                    |
| 2026-10-10 | P19   | Golden-Tests für Auflösung (9) und Abfragen (3); 6 weitere Fehler behoben                                                                             |
| 2026-10-10 | P18   | Golden-File-Tests je Parser (21 Szenarien mit Kommentaren und Abdeckungsübersicht), dabei 9 Parser-Fehler gefunden und behoben                        |
| 2026-10-10 | P17   | Mehrere Projekte: Registry, je Projekt eigener Index, MCP mit Projektauswahl, Werkzeug projects                                                       |
| 2026-10-10 | P16   | Abfrageschicht mit Token-Budget und Paginierung, MCP-Server (stdio) mit 9 Werkzeugen, Nur-Lese-Betrieb mit Neuladen                                   |
| 2026-10-10 | P15a  | Import aus .NET-XML-Dokumentation ohne Quelltext: Doku-IDs als Symbol-IDs, virtuelles Assembly, Verschmelzung mit Quelltext, CLI import-xml           |
| 2026-10-10 | P14b  | Resolver mit Bitmasken und strukturierten Schlüsseln, Puffer-Wiederverwendung in Parsern und Pipeline, GC-Anteil von ~30 % auf ~14 % gesenkt          |
| 2026-10-10 | P14   | Performance: Messung auf 26 540 TS-Dateien, Engpässe im Resolver und Lexer beseitigt, Speicherlimit, Ziel-Metriken erreicht                           |
| 2026-10-10 | P13   | Export: JSON (gestreamt), JSON Lines, Mermaid-Klassendiagramm, Markdown je Datei; CLI export                                                          |
| 2026-10-10 | P12   | CLI: index, symbol, refs, impact, diff; JSON-Ausgabe; Exit-Codes; Snapshot-Backend als persistenter Übergang                                          |
| 2026-10-10 | P11   | Diff auf Symbolebene (Hunks, direkt betroffen), Impact-Analyse über Verwendungsketten, Abfrage nach Namen                                             |
| 2026-10-10 | P10   | Inkrementelle Updates per git diff, Vollaufbau-Fallback, namensbezogene Neuauflösung, Gleichheitsprüfung gegen Vollaufbau                             |
| 2026-10-10 | P9    | Beziehungen: inherits, implements, has_type, param_type, returns, attribute_of, contains; Store-Methoden für abgeleitete Beziehungen                  |
| 2026-10-10 | P8    | Resolver: Modulauflösung, tsconfig-Pfade, Scope/using/Bindungen, Re-Exporte, Konfidenzen, Re-Auflösung über OnlyNames                                 |
| 2026-10-10 | P7    | TypeScript/JavaScript-Parser: Maskierung, Imports, Deklarationen, JSX, JSDoc, Verwendungen, zod-Lauf ohne Diagnosen                                   |
| 2026-10-10 | P6    | C#-Parser: Maskierung, Tokenizer, Deklarationen, partial, Attribute, XML-Doku, Verwendungen, Tests, MediatR-Stichproben                               |
| 2026-10-10 | P5    | Parser-Interface, Registry, Pipeline mit Worker-Pool und einzigem Writer, cat-file-Reader, Tests inkl. 10.000 Dateien                                 |
| 2026-10-10 | P4    | Scanner über git ls-tree, Skip-Gründe, Abgleich mit Testrepos                                                                                         |
| 2026-10-10 | P2    | Datenmodell, Symbol-ID mit Überladungen über Parametertypen                                                                                           |
| 2026-10-10 | P3    | Store-Schnittstelle, Registry, memory-Backend, Vertragstests, SQLite-Platzhalter (Treiber blockiert)                                                  |
| 2026-10-10 | P1    | Projektgerüst angelegt, Config, CLI-Einstieg, Makefile, Tests, Git-Commit                                                                             |
| 2026-10-10 | P0    | Testrepos festgelegt, Python-Baseline gemessen, Befunde dokumentiert                                                                                  |
