# Testabdeckung C#

Automatisch erzeugt. Jeder Fall ist eine Eingabedatei mit Golden-Datei.

| Datei | Szenario | Abgedeckt |
|---|---|---|
| `01_namespaces_usings.cs` | Namensräume (Block- und Datei-Form), verschachtelte Namensräume und alle using-Varianten | namespace-block, file-scoped namespace, verschachtelter namespace, using, using static, using-Alias, global using |
| `02_types.cs` | Alle Typarten mit Modifikatoren, Generics (Arity im Namen), verschachtelten Typen und Record-Varianten | class, struct, interface, record, record struct, enum mit Werten, generische Typen mit Arity, verschachtelte Typen, abstract, sealed, static |
| `03_members.cs` | Alle Member-Arten: Felder (mehrere Deklaratoren), Properties (auto, Ausdruck, init), Indexer, Events, Methoden (Ausdruckskörper, generisch, mehrzeilig), Konstruktoren, Operatoren, Destruktor | field, multiple declarators, auto-property, expression-bodied property, init-accessor, indexer, event, method, generic method, expression-bodied method, constructor, static constructor, operator, conversion operator, destructor |
| `04_inheritance_attributes.cs` | Basisklassen und Interfaces (Erkennung über das I-Präfix), mehrere Attribute, Attribute mit Ziel-Präfix und Attribute am Member | extends, implements, I-Präfix-Heuristik, Basisliste mit Generics, Attribute am Typ, Attribute mit Argumenten, Attribut am Member, assembly-Attribut (ohne Ziel-Symbol, daher kein Verweis), Basis object wird übersprungen |
| `05_partial.cs` | partial-Klassen und partial-Member in einer Datei | partial class, partial method, Rolle partial am Fundort |
| `06_xmldoc.cs` | XML-Dokumentation: Summary, Remarks, Returns, Parameter, Typparameter, see/seealso mit cref, Entities und mehrzeilige Texte | summary, remarks, returns, param, typeparam, see cref, seealso cref, XML-Entities, mehrzeilige Doku, Doku nur an Member |
| `07_masking.cs` | Texte, die wie Code aussehen, dürfen keine Symbole erzeugen. Präprozessor-Zweige mit gleichen Deklarationen werden zusammengeführt. | Zeilenkommentar, Blockkommentar, normaler String mit Escapes, verbatim-String mit "", Rohstring """, Zeichenliteral mit Klammer, #if/#else-Duplikate |
| `08_references.cs` | Verwendungen im Körper: Aufrufe mit unterschiedlicher Argumentzahl, Konstruktoraufrufe, Generic-Aufrufe, Schreibzugriffe, Member-Zugriff über Feld, Typverweise in Signaturen | call mit arity, construct, generischer Aufruf, write, this.x =, type-ref in Parameter und Rückgabe, Attribut-Verweis, Qualifier-Aufruf |
| `09_broken.cs` | Kaputter Quelltext darf nicht abstürzen. Es entstehen Diagnosen, die übrigen Deklarationen bleiben erhalten. | fehlende Klammer am Ende, unvollständige Deklaration, Parameterliste nicht geschlossen, gute Deklaration danach |
| `10_bom_crlf.cs` | UTF-8-BOM am Dateianfang und Windows-Zeilenenden (CRLF). Positionen müssen trotzdem stimmen. | BOM, CRLF-Zeilenenden, Umlaute in Bezeichnern und Kommentaren |
| `11_version_warnings.cs` | Konstrukte neuerer C#-Versionen, die der Parser nur teilweise erfasst. Er warnt, statt still falsch zu arbeiten. | primärer Konstruktor (C# 12), Positionsparameter eines Records (C# 9), file-lokaler Typ (C# 11), Kommentar mit Muster bleibt ohne Warnung |
| `12_http_routes.cs` | ASP.NET-Controller mit HTTP-Attributen und Präfix [controller], absolute Routen, Minimal-API-Aufrufe mit Parameter | Route auf Klasse mit [controller], HttpGet ohne und mit Parameter, HttpPost, absoluter Pfad mit führendem /, Route mit Constraint {id:int}, MapGet mit Literal, Kommentar mit Route bleibt ohne Verweis |
