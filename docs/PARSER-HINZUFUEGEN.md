# Einen Parser hinzufügen

Diese Anleitung beschreibt, wie man eine neue Sprache in goreadmenator
einbindet, was dabei beachtet werden muss und wie die Tests aufgebaut sind. Als
Vorlage dienen die bestehenden Parser `internal/parser/csharp` und
`internal/parser/tsjs`.

## 1. Überblick

Ein Parser wandelt den Inhalt einer Datei in ein `parser.Result` um. Das
Ergebnis enthält Symbole, Fundorte, Verwendungen, Beziehungen, Imports und
Diagnosen. Alles andere (Lesen aus Git, Speichern, Auflösen über Dateigrenzen,
Abfragen) übernimmt die restliche Pipeline. Der Parser muss deshalb nur **eine
Datei** verstehen und darf keine anderen Dateien öffnen.

Die Pipeline ruft den Parser so auf:

```
scanner (Dateiliste, Sprache über Endung)
  -> index.Run (Worker-Pool, ein Schreiber)
       -> parser.Parse(ctx, parser.Source{Path, BlobSHA, Content})
  -> resolve.Run (Namen auf Symbol-IDs abbilden)
  -> store (Speicherung, Abfragen)
```

## 2. Schritt für Schritt

### 2.1 Sprache anmelden

1. In `internal/model/symbol_id.go` eine Konstante ergänzen:

   ```go
   LangPython Language = "python"
   ```

   Der Wert erscheint als Präfix in jeder Symbol-ID (`python:...`). Er ist Teil
   des Datenformats und darf danach nicht mehr geändert werden.

2. In `internal/scanner/scanner.go` die Dateiendungen in `extLang` eintragen:

   ```go
   ".py": model.LangPython,
   ```

3. Falls die Sprache Namen nach eigenen Regeln schreibt (wie C# mit
   Schlüsselwort-Typen), `NormalizeType` in `symbol_id.go` um einen Zweig
   erweitern. Ohne Eintrag wird nur der Leerraum entfernt.

### 2.2 Parser schreiben

Das Interface steht in `internal/parser/parser.go`:

```go
type Parser interface {
    Language() model.Language
    Parse(ctx context.Context, src Source) (Result, error)
}
```

Empfohlene Dateiaufteilung (wie bei den vorhandenen Parsern):

| Datei                     | Aufgabe                                                                                          |
| ------------------------- | ------------------------------------------------------------------------------------------------ |
| `mask.go`                 | Kommentare, Strings und Literale durch Leerzeichen ersetzen, Zeilen und Spalten bleiben erhalten |
| `lex.go`                  | Maskierten Text in Tokens mit Position zerlegen                                                  |
| `syntax.go`               | Hilfsfunktionen: Klammern finden, Typen lesen, Anweisungsenden finden                            |
| `state.go`                | Zustand der Datei, Ausgabe mit Duplikatschutz, Gliederung (Namensräume, Imports)                 |
| `members.go` / `decls.go` | Deklarationen                                                                                    |
| `refs.go`                 | Verwendungen im Körper                                                                           |
| `doc.go` o. ä.            | Dokumentationskommentare                                                                         |
| `<sprache>.go`            | Typ `Parser`, `New()`, `Parse()` als Einstieg                                                    |

Der Einstieg sieht so aus:

```go
type Parser struct{}

func New() Parser { return Parser{} }

func (Parser) Language() model.Language { return model.LangPython }

func (Parser) Parse(ctx context.Context, src parser.Source) (parser.Result, error) {
    if err := ctx.Err(); err != nil {
        return parser.Result{}, err
    }
    return parseFile(src.Path, src.Content), nil
}
```

Der Parser darf **keinen veränderlichen Zustand über Aufrufe hinweg** halten. Er
wird von mehreren Goroutinen gleichzeitig benutzt.

### 2.3 Parser registrieren

Alle Parser werden an **einer** Stelle eingetragen:
`internal/parsers/parsers.go`, Funktion `Registry()`.

```go
return parser.NewRegistry(csharp.New(), tsjs.New(), vbnet.New(), python.New())
```

CLI und alle Tests rufen `parsers.Registry()` auf. Vor dem Umbau war die Liste
an sechs Stellen kopiert. Wer einen Parser vergisst, bekommt Dateien
stillschweigend übersprungen. Deshalb prüft die Checkliste diesen Punkt.

Außerdem braucht die Auflösung einen Zweig: In `internal/resolve/resolve.go`,
Funktion `resolveRef`, werden Sprachen **ohne** eigene Regeln nach den C#-Regeln
behandelt (`file.Lang != model.LangTSJS`). Eine Sprache mit anderen Regeln (etwa
eigene Modulauflösung) braucht einen eigenen Zweig, sonst gelten unbemerkt die
C#-Regeln.

### 2.4 Parser-Version erhöhen

In `internal/index/pipeline.go` die Konstante `ParserVersion` anheben, z. B. von
`"p9-1"` auf `"p18-1"`. Der nächste Lauf baut den Index dann vollständig neu
auf. Ohne diesen Schritt bleiben alte und neue Ergebnisse nebeneinander
bestehen. **Jede Änderung an einem Parser oder an der Auflösung erfordert das.**

### 2.5 Sprachabdeckung melden (optional, aber empfohlen)

Ein Parser kann melden, bis zu welcher Sprachversion er die Konstrukte erfasst.
Dafür implementiert er `Coverage()`:

```go
func (Parser) Coverage() parser.Coverage {
    return parser.Coverage{Language: model.LangPython, MaxVersion: "3.13"}
}
```

Leer lassen, wenn die Version nicht sinnvoll ist. Dann wird nichts geprüft. Die
Prüfung in `internal/langver` vergleicht die Projektversion mit diesem Wert und
warnt nur, sie bricht nichts ab. Dafür muss der Parser in
`internal/langver/tsvb.go` bzw. `langver.go` einen Projektleser haben und in
`langver.Warnings()` eingebunden sein. Heute sind das:

- C#: `.csproj`, `LangVersion` oder Standard aus `TargetFramework`
- TypeScript/JavaScript: `tsconfig.json`, `target` (ES-Jahr)
- VB.NET: `.vbproj`, nur `LangVersion`. Ohne Angabe wird nichts geraten.

Konstrukte, die der Parser nur teilweise erfasst, meldet er als **Warnung**
(`parser.SeverityWarning`), nicht als Fehler. Ein Beispiel ist `versionWarnings`
in `internal/parser/csharp/version.go`. Dort steht jedes Konstrukt mit der
Version, ab der es existiert, und dem Hinweis, was fehlt. Das Erkennen erfolgt
am Token-Muster, damit Kommentare und Strings keine Warnung auslösen. Jede neue
Sprachversion braucht eine Pflege dieser Liste und einen Golden-Fall dafür.

## 3. Datenmodell: was der Parser liefern muss

### 3.1 Symbol-IDs

Die ID wird immer über `model.NewSymbolID(lang, fqn, paramTypes)` gebildet, nie
von Hand zusammengesetzt.

- **Typen und Namensräume**: `paramTypes = nil`, ergibt `python:pkg.Klasse`.
- **Methoden und Funktionen**: `paramTypes` mit den geschriebenen Typnamen. Ohne
  Typangabe `"any"` verwenden, damit die ID stabil bleibt. Eine Methode ohne
  Parameter bekommt `[]string{}`, also `()`.
- **Member**: FQN des Containers plus `.` plus Name, z. B. `pkg.Klasse.methode`.
- **Generics**: Arity als Backtick-Suffix im Namen (`Name`2`), wie C#. So kann
  der Resolver generische Typen mit der richtigen Anzahl von Typargumenten
  finden.
- **Wichtig**: Die ID hängt nur am FQN, nicht am Fundort. Dadurch funktionieren
  partielle Deklarationen über Dateien hinweg.
- Für Sprachen mit Dateipfad im Namen (wie TS) gilt das Schema `pfad#name`. Der
  Trenner `#` muss vor dem ersten Punkt im Namen stehen, sonst findet die
  Auflösung den Präfix nicht (siehe `splitFQN` in `internal/resolve/index.go`).

### 3.2 Symbole, Fundorte, Verwendungen

| Feld                   | Regel                                                                                                                                                                            |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Symbol.Kind`          | Eine der Konstanten aus `model`. Unbekannte Arten besser als eigene Konstante eintragen, als eine vorhandene missbrauchen (siehe `type-unknown`).                                |
| `Symbol.Container`     | ID des umgebenden Namensraums oder Typs. Leer nur auf oberster Ebene ohne Namensraum.                                                                                            |
| `Symbol.Modifiers`     | Sichtbarkeit und Besonderheiten als Wörter, z. B. `public`, `export`, `partial`, `default`. Der Resolver und `api` prüfen darauf.                                                |
| `Symbol.Signature`     | Lesbare Signatur ohne Körper.                                                                                                                                                    |
| `Symbol.DocSource`     | `"inline"` für Dokumentation aus dem Quelltext, `"xml"` für Importe.                                                                                                             |
| `Location.Span`        | Startzeile und -spalte, Endzeile und -spalte. **Zeilen ab 1, Spalten ab 1 als Byte-Offset, Endspalte inklusiv.** Der Startpunkt ist der Name, der Endpunkt das Ende des Körpers. |
| `Location.Role`        | `RoleDecl`, bei Mehrfachdeklarationen wie partial `RolePartial`.                                                                                                                 |
| `Reference.Kind`       | Aus den Konstanten `Ref*`. Für Aufrufe `RefCall`, Erzeugung `RefConstruct`, Schreibzugriff `RefWrite`.                                                                           |
| `Reference.Confidence` | Beim Parsen fast immer `ConfHeuristic`. Die Auflösung setzt später die echte Konfidenz. Eine Ausnahme sind explizite Angaben wie `cref`, die als `ConfExact` gelten.             |
| `Reference.Enclosing`  | Das umgebende Symbol, in dem die Verwendung steht. Leer nur auf Modulebene.                                                                                                      |
| `Reference.Arity`      | Bei Aufrufen und Konstruktoren die Anzahl der Argumente, sonst 0. Bei Typreferenzen die Anzahl der Typargumente.                                                                 |
| `Reference.Role`       | `has-type`, `param-type` oder `returns` am Kopf eines Typverweises. Daraus entstehen Beziehungen.                                                                                |
| `Reference.Qualifier`  | Name links vom Punkt bei Aufrufen (`util` in `util.keys()`).                                                                                                                     |

### 3.3 Beziehungen und Imports

- **contains** (Typ zu Member, Namensraum zu Typ) setzt der Parser selbst, weil
  beide IDs bekannt sind.
- **inherits, implements, has_type, param_type, returns, attribute_of** setzt
  der Parser **nicht** direkt. Er liefert die Verweise mit der passenden Rolle,
  die Auflösung leitet die Beziehungen ab.
- **Imports** als `model.Import` mit `Raw` (Modulname oder Namensraum), `Kind`
  und `Bindings` (lokaler Name und importierter Name). Die Bindungen sind
  Grundlage der Auflösung über Dateigrenzen.
- **Diagnosen** für Stellen, die nicht verstanden wurden. Kaputter Code darf nie
  zu einem Absturz führen.

### 3.4 Duplikate

Ein Parser darf dieselbe Deklaration mehrfach sehen (etwa in `#if`-Zweigen). Die
Ausgabe-Funktionen (`emitSymbol`, `emitLoc`, `emitRef`, `emitRel`) verwerfen
Duplikate über typisierte Schlüssel. Diese Funktionen immer benutzen und nie
direkt an `res.Symbols` anhängen, sonst entstehen doppelte Einträge.

## 4. Die kritischen Stellen beim Parsen

Diese Punkte haben bei den bisherigen Parsern die meisten Fehler verursacht.

1. **Maskieren vor dem Tokenisieren.** Kommentare, Strings, Zeichenliterale und
   Präprozessor- oder Shebang-Zeilen müssen durch Leerzeichen ersetzt werden,
   bevor Deklarationen gesucht werden. Zeilenumbrüche bleiben erhalten. Sonst
   entstehen Symbole aus Kommentaren (`// class Fake {}`).
2. **Sonderfälle der Sprache sind Pflicht**: verbatim- und Rohstrings (C#),
   Template-Literale mit Interpolation (TS), Regex gegenüber Division (TS). Ein
   Literal, das nicht sauber endet, darf nicht den Rest der Datei verschlucken.
   Deshalb endet ein String an einem Zeilenumbruch.
3. **Byte-Order-Mark und Zeilenenden.** Ein UTF-8-BOM am Dateianfang muss
   entfernt werden, sonst verschmilzt er mit dem ersten Schlüsselwort.
   CRLF-Dateien müssen gleiche Zeilennummern liefern.
4. **Keine Endlosschleifen.** Jede Schleife über Tokens muss den Index erhöhen
   oder abbrechen. Ein Beispiel aus der Praxis: Ein Ersetzungsmuster, das sein
   eigenes Ergebnis wieder findet, lief endlos (`[]` enthält `[`). Bei
   Ersetzungen immer von der Restzeichenkette weiterarbeiten.
5. **Nicht geschlossene Klammern.** Fehlt eine schließende Klammer, darf der
   Parser nicht den Rest der Datei verschlucken. `recoverBodyEnd` in
   `internal/parser/tsjs/syntax.go` zeigt den Ansatz: Ende vor der nächsten
   Deklaration in Spalte 1.
6. **Keine Schlüsselwörter als Namen.** Schlüsselwörter, die wie Aufrufe
   aussehen (`if(`, `return(`), müssen aus den Aufrufen herausgefiltert werden.
   Dafür gibt es `controlKeywords` bzw. `controlWords`.
7. **Fortschritt garantieren.** Die Hauptschleife prüft, ob sich die Position
   geändert hat, und erhöht sie sonst (siehe `parseScope`).
8. **Speicher wiederverwenden.** Token-Slices und maskierte Puffer kommen aus
   einem `sync.Pool` (siehe `getTokBuf` und `getMaskBuf`). Interne Texte über
   eine Map zusammenführen, damit Bezeichner nicht mehrfach allokiert werden.

## 4a. Sprachen, die zeilenorientiert oder nicht case-sensitiv sind

Der VB.NET-Parser (`internal/parser/vbnet`) hat zwei Eigenschaften, die über C#
und TS hinausgehen. Beide gelten für jede ähnliche Sprache (etwa Basic-Dialekte
oder SQL-artige Deklarationen).

**Zeilenorientierung.** Statt Klammern zu zählen, zerlegt der Lexer die Datei in
Anweisungen. Eine Anweisung endet am Zeilenende oder am Doppelpunkt.
Zeilenfortsetzungen (`_` am Zeilenende) erledigt der Lexer selbst, indem er den
Zeilenumbruch nicht als Anweisungsende zählt. Zeilennummern bleiben dabei
korrekt. Blöcke werden mit einem Zähler verfolgt: Blockanfänge wie
`If ... Then`, `For`, `Do` erhöhen, `End ...`, `Next`, `Loop` verringern ihn
(siehe `blockOpener` und `parseMemberBody`).

**Sprachfamilien (mehrere Sprachen in einem Projekt).** Ein Projekt kann mehrere
Sprachen enthalten, etwa ASP.NET mit C#, VB.NET und TypeScript. Der Resolver
fasst Sprachen deshalb zu **Familien** zusammen (`family()` in
`internal/resolve/index.go`): C# und VB.NET bilden die Familie `dotnet`, weil
sie dieselben .NET-Typen teilen. TypeScript und JavaScript bilden je eine eigene
Familie. Ein Typ aus C# ist aus VB sichtbar und umgekehrt, TypeScript bleibt
davon getrennt.

Wer eine neue Sprache anlegt, muss entscheiden, zu welcher Familie sie gehört.
Gehört sie zu einer bestehenden Plattform, die dieselben Namen teilt, kommt sie
in diese Familie. Sonst bekommt sie eine eigene.

**Nicht case-sensitiv.** Die Auflösung vergleicht Namen als Strings. Für solche
Sprachen müssen die Schlüssel kleingeschrieben abgelegt und gesucht werden.
Dafür gibt es in `internal/resolve/index.go` die Funktion `fold(lang, s)`. Alle
Schlüssel (`childKey`, `nameKey`, `fqnKey`) enthalten die **Familie**, damit
Namen verschiedener Familien sich nie treffen. .NET-Namen werden
kleingeschrieben abgelegt. Für C# prüft der Resolver danach die Schreibweise
exakt (`exactName`), weil C# case-sensitiv ist. Die Symbol-ID selbst behält die
Schreibweise der Deklaration.

Zwei Eigenschaften helfen bei Verwendungen über Felder: Der Resolver läuft
**zweimal**. In der ersten Runde werden Typen aufgelöst, in der zweiten Aufrufe
über Felder, Eigenschaften und Variablen (`svc.Save()` über den Typ von `svc`).
Ein neuer Parser muss deshalb `has-type`-Verweise mit dem Typ des Feldes
liefern, sonst greift diese Auflösung nicht.

Stolperfallen, die beim VB-Parser aufgetreten sind:

- **Attribute auf eigener Zeile.** In VB stehen Attribute oft in einer eigenen
  Zeile vor der Deklaration. Der Parser muss sie sammeln (`pendingAttrs`) und an
  die nächste Deklaration hängen. Die Dokumentation steht dann über der ersten
  Attributzeile, daher wird dort gesucht (`pendingLine`).
- **Lambda oder Deklaration.** `Function(x) ...` in einem Ausdruck ist kein
  neuer Member. Eine Deklaration erkennt man am fehlenden `(` direkt nach dem
  Schlüsselwort (`startsDeclaration`).
- **Fehlendes End.** Fehlt das `End Sub`, darf der Körper nicht den Rest der
  Datei verschlucken. Der Parser erkennt die nächste Deklaration auf Ebene 0 und
  bricht dort ab (siehe Golden-Fall `09_broken`).
- **Accessor-Köpfe.** `Set(value As T)` sieht aus wie ein Aufruf. Die Kopfzeile
  eines Accessors darf keine Verwendung erzeugen.
- **Ungültige Eingaben im Test.** Ein Umbruch nach einem Komma ist in VB ohne
  `_` kein Fortsetzung. Die Golden-Eingabe muss gültiger Quelltext sein, sonst
  testet man Fehlerfälle, die man nicht wollte.
- **Konstruktoren.** `Sub New(...)` wird als Konstruktor mit dem Namen `New`
  geführt, die ID enthält `New(...)`.
- **Typen ohne Angabe.** Ein Parameter ohne `As` hat in VB den Typ `Object`. Die
  ID nutzt dann `Object`, nicht `any`.
- **Schlüsselworttypen.** `Integer`, `String` & Co. werden über `vbAliases` in
  `model/symbol_id.go` auf System-Typen abgebildet. Damit stimmen IDs mit
  C#-Schreibweisen überein, wenn man sie vergleicht.

## 4b. Parser für Vorlagensprachen (Razor, ASPX)

Razor und ASPX sind HTML mit eingebettetem C#. Der Parser
`internal/parser/razor` macht daraus eine C#-Datei und lässt den C#-Parser
darauf laufen. Das ist der Weg für jede Vorlagensprache mit Codeanteil:
**nicht** einen eigenen Parser für den Codeanteil schreiben, sondern den
vorhandenen wiederverwenden.

Wichtige Regeln:

- **Zeilen und Spalten bleiben erhalten.** Jede Codezeile wird auf die Spalte
  der Vorlage eingerückt. Erzeugte Hilfszeilen (Klassenkopf, `__Render`) haben
  keine Entsprechung und werden auf Zeile 1 abgebildet. Die Zuordnung ist die
  Tabelle `origLine`, die beim Zusammensetzen entsteht. Nach dem Parsen werden
  alle Fundorte über diese Tabelle zurückgeführt.
- **Der HTML-Teil wird weggelassen.** Markup-Zeilen (eine Zeile, die mit einem
  Tag beginnt) gehören im Codeblock nicht zum Code. Inline-Ausdrücke (`@name`,
  `@(...)`, `<%= %>`) werden als eigene Anweisung ausgegeben, damit ihre Aufrufe
  erfasst werden.
- **Direktiven werden in Mitglieder und Usings übersetzt.** `@using` wird zu
  `using`, `@model T` zu einem Feld `Model`, `@inject T name` zu einem Feld,
  `@inherits` und `Inherits="..."` zu einer Basisklasse.
- **Klasse und Namensraum kommen aus dem Pfad.** `Pages/Sub/Index.cshtml` ergibt
  `Pages.Sub.Index`. Der Pfad muss relativ zum Repository sein, sonst stimmen
  die IDs nicht.
- **Sprachfamilie.** Die Symbole haben die C#-IDs, die Sprache der Datei ist
  `razor` bzw. `aspx`. Beide gehören zur Familie `dotnet`, damit die Auflösung
  über Sprachgrenzen funktioniert.

Bekannte Grenzen dieser Vorlagen-Parser: HTML innerhalb eines Kontrollblocks
(`@if`, `@foreach`) wird nur über Zeilen erkannt, die mit einem Tag beginnen.
Ein Tag mitten in einer Codezeile kann dort einen Teil des Codes verschlucken.
Qualifizierte Typnamen (`Shop.Models.Order`) bekommen als Ganzes einen Verweis.
Der Resolver sucht sie vom innersten Namensraum nach außen (siehe
`qualifiedLookup` in `internal/resolve/resolve.go`), danach über die
`using`-Direktiven der Datei. Ein Treffer über genau einen Namensraum ist
eindeutig. Treffen mehrere zu, bleibt der Verweis offen, wie der Compiler
(CS0104) es verlangt.

## 4c. HTTP-Verbindungen (Routen und Aufrufe)

Frontend und Backend sind über HTTP verbunden, nicht über Namen. Dafür gibt es
zwei Arten von Verweisen, die **keinen Namen zum Auflösen** haben:

- `http-route` (Backend): TargetName ist `VERB pfad`, Enclosing die Aktion.
  Quelle: HTTP-Attribute mit Route (`[HttpGet("{id}")]`, mit `[Route]` am
  Controller), Minimal-API-Aufrufe (`app.MapGet("/x", ...)`).
- `http-call` (Frontend): TargetName ist `VERB pfad`, Enclosing die aufrufende
  Funktion. Quelle: `fetch`, `axios.<verb>`, `http.<verb>` mit einem
  URL-Literal, einer Verkettung oder einem Template. Laufzeitteile werden zu
  `{}`.

Pfade werden mit `model.NormalizeRoute` vereinheitlicht (Parameter, Schema,
Host, Query, Groß- und Kleinschreibung). Der Resolver überspringt diese Verweise
bei der Namensauflösung und verbindet sie in `linkEndpoints` über Verb und
Segmente. Ein Aufruf verbindet sich nur, wenn genau **eine** Route passt. Das
Ergebnis ist die Beziehung `calls_endpoint`.

**Konventionsrouting und Razor-Seiten.** Zwei weitere Formen:

- `http-convention` (Backend): `PATTERN <muster>` aus `MapControllerRoute`, und
  `ACTION <controller>/<aktion>` für jede öffentliche Aktion eines Controllers
  ohne Attributroute. Der Resolver setzt daraus mit
  `model.ExpandConventionRoutes` Routen mit dem Verb `ANY` zusammen. Optionale
  Parameter (`{id?}`) erzeugen zusätzlich die kürzere Variante.
- `@page "/pfad"` in Razor: eine Route `ANY pfad`, die an der Seitenklasse
  hängt.

Wichtig für neue Sprachen: Ein Parser liefert die Verweise mit der Form
`VERB pfad`, nicht mit dem Namen einer Methode. Literale liest der C#-Parser
über `literalAt` aus der Maskierung, der TS-Parser direkt aus den Stringtokens
und dem Rohtext interpolierter Templates (`templateAt`).

## 5. Tests

Es gibt drei Testebenen. Die Golden-Tests sind die wichtigste, weil sie die
Abdeckung sichtbar machen.

### 5.1 Golden-Tests (Pflicht)

Der Ablauf:

1. Testdateien in `internal/parser/<sprache>/testdata/` anlegen, eine Datei pro
   Szenario, nummeriert: `01_module.py`, `02_klassen.py`, ...
2. Jede Datei beginnt mit zwei Kommentarzeilen. Ohne sie fehlt der Titel in der
   Übersicht:

   ```python
   # SZENARIO: Klassen mit Basisklasse, Methoden, Properties und Decorators
   # ABDECKUNG: class, extends, method, property, decorator, staticmethod
   ```

   `SZENARIO` beschreibt den Zweck in einem Satz. `ABDECKUNG` listet die
   Konstrukte als kommagetrennte Stichworte. Wenn ein Konstrukt bewusst nicht
   abgedeckt wird, steht das im Kommentar im Code, mit Begründung.
3. Den Testtreiber kopieren und die Endung anpassen. Vorlage:
   `internal/parser/tsjs/golden_test.go`. Er ruft `golden.ReadCase` für die
   Übersicht und `golden.Dump` für die Ausgabe auf.
4. Die erwartete Ausgabe erzeugen:
   `READMENATOR_UPDATE_GOLDEN=1 go test ./internal/parser/<sprache> -run TestGolden`.
   Dabei entstehen `*.golden.txt` und `testdata/COVERAGE.md`.
5. **Die erzeugten Dateien von Hand durchsehen.** Die Golden-Datei ist ein
   Vertrag. Sie hält auch Fehler fest, wenn man sie ungeprüft übernimmt.
   Typische Fehler: falsche Zeilennummern, Symbole aus Kommentaren, fehlende
   Kinder, Verwendungen mit falscher Enclosing-ID.
6. Ohne Update-Modus läuft der Test gegen die gespeicherte Datei. Jede
   Abweichung wird mit Zeile und erwartetem und erhaltenem Stand gemeldet.

Die Ausgabe ist so sortiert, dass Abweichungen lokal bleiben. Ein neuer Verweis
verschiebt nicht alle folgenden Zeilen.

### 5.2 Mindestumfang der Fälle

Für eine neue Sprache sollten mindestens diese Szenarien existieren. Jedes
erzeugt eine eigene Datei:

| Szenario                | Prüft                                                               |
| ----------------------- | ------------------------------------------------------------------- |
| Module und Imports      | alle Importformen, Re-Exporte, dynamische Imports                   |
| Funktionen              | Signaturen, Überladungen, Anonyme und Lambda-Funktionen             |
| Klassen                 | Vererbung, Methoden, Felder, Konstruktor, statische Member          |
| Typen                   | Interfaces, Aufzählungen, Aliase, Generics                          |
| Literale und Kommentare | Texte, die wie Code aussehen, erzeugen keine Symbole                |
| Dokumentation           | Zusammenfassung, Parameter, Rückgabe, Verweise                      |
| Verwendungen            | Aufrufe mit Argumentzahl, Konstruktoren, Schreibzugriffe, Qualifier |
| Kaputter Code           | Diagnosen, keine Abstürze, nachfolgende Deklarationen bleiben       |
| Kodierung               | BOM, Zeilenenden, Umlaute                                           |

### 5.3 Unit-Tests (ergänzend)

Für Einzelheiten, die sich schlecht in Golden-Dateien ausdrücken lassen, eine
`*_test.go` mit gezielten Prüfungen schreiben, z. B. Zeilennummern einzelner
Methoden. Vorlage: `internal/parser/csharp/csharp_test.go`.

### 5.4 Korpustests (optional)

Ein echtes Projekt über `READMENATOR_CORPUS` prüfen. Die Tests sind
übersprungen, wenn die Variable fehlt. Zusätzlich gibt es Benchmarks
(`bench_test.go`) für den Durchsatz.

### 5.5 Auswirkungen auf andere Pakete

Nach dem Hinzufügen eines Parsers immer die gesamte Suite laufen lassen.
Besonders wichtig:

- `internal/update` (Gleichheit Inkrement und Vollaufbau)
- `internal/resolve` (Auflösung und Beziehungen)
- `internal/cli` (End-to-End)

## 6. Checkliste

- [ ] Konstante `model.Lang...` angelegt
- [ ] Endungen in `scanner.extLang` eingetragen
- [ ] Parser implementiert, `Language()` und `Parse()` vorhanden
- [ ] Parser in `internal/parsers/parsers.go` eingetragen (einzige Stelle)
- [ ] Auflösungszweig in `resolveRef` geprüft (eigene Regeln nötig?)
- [ ] Sprachabdeckung gemeldet (`Coverage()`), oder bewusst leer
- [ ] Teilweise erfasste Konstrukte als Warnung gemeldet und mit Golden-Fall
      abgesichert
- [ ] Familie festgelegt (`family()`): eigene oder gemeinsame mit bestehender
      Sprache
- [ ] Falls nicht case-sensitiv: `fold` greift für die Familie
- [ ] Bei Vorlagensprachen: Codeanteil über den C#-Parser laufen lassen (siehe
      4b), keine eigene Codeanalyse
- [ ] Qualifizierte Namen: der Parser liefert den vollen Namen in `TargetName`,
      nicht nur den letzten Teil
- [ ] HTTP-Fakten (Routen, Aufrufe) als `VERB pfad` mit `NormalizeRoute`
      liefern, falls die Sprache Endpunkte hat
- [ ] Mindestens ein Auflösungs-Testfall (Verzeichnis in
      `internal/resolve/testdata`) mit Sprachgrenzen
- [ ] `ParserVersion` erhöht
- [ ] Symbol-IDs über `model.NewSymbolID`, Arity für Generics
- [ ] Spans: Zeilen ab 1, Spalten ab 1, Endspalte inklusiv
- [ ] Maskieren: Kommentare, Strings, Literale, Präprozessor
- [ ] BOM und CRLF berücksichtigt
- [ ] Schleifen garantieren Fortschritt, kein Endlosloop
- [ ] Kaputter Code erzeugt Diagnosen, keinen Absturz
- [ ] Golden-Szenarien mit `SZENARIO` und `ABDECKUNG` angelegt
- [ ] Golden-Dateien von Hand geprüft
- [ ] `COVERAGE.md` erzeugt und gesichtet
- [ ] Gesamte Testsuite grün, auch mit `-race`

## 7. Häufige Fehler

- **Symbol aus einem String oder Kommentar.** Die Maskierung fehlt oder ist
  unvollständig. Abhilfe: ein Golden-Szenario mit solchen Texten anlegen.
- **Zeilen um eins verschoben.** Der Startpunkt liegt auf dem Anfang eines
  Leerraums oder einer Zeile davor. Der Startpunkt muss der Name sein.
- **Doppelte Symbole bei `#if`.** Nicht über `res.Symbols` anhängen, sondern
  `emitSymbol` verwenden.
- **Auflösung findet Dateiübergreifendes nicht.** Die Import-Bindungen fehlen
  oder der Modulpfad ist nicht normalisiert.
- **Alte Ergebnisse nach einer Änderung.** `ParserVersion` wurde nicht erhöht.
- **Test läuft nicht, obwohl der Parser fertig ist.** Der Parser fehlt in
  `internal/parsers/parsers.go`. Die Dateien werden dann stillschweigend
  übersprungen.
- **Verwendungen bleiben trotz Import offen.** Entweder fehlt der
  Auflösungszweig, oder die Sprache wird nicht case-insensitiv behandelt.
- **Symbole aus Blöcken ohne End.** Der Körper verschluckt die folgenden
  Deklarationen. Erkennung auf Ebene 0 einbauen (siehe 4a).
- **Golden-Datei einfach übernommen.** Sie hält Fehler fest. Immer von Hand
  prüfen.
