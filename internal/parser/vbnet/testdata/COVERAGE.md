# Testabdeckung VB.NET

Automatisch erzeugt. Jeder Fall ist eine Eingabedatei mit Golden-Datei.

| Datei | Szenario | Abgedeckt |
|---|---|---|
| `01_namespaces_imports.vb` | Namensräume (verschachtelt), Imports mit und ohne Alias, mehrere Namensräume in einer Datei | Namespace-Block, verschachtelter Namespace, Imports, Imports mit Alias, Namensraum ohne Typen |
| `02_classes_inheritance.vb` | Klassen mit Basisklasse und Interface-Implementierung, Modifikatoren, Partial | Class, Inherits, Implements (mehrere), Public/Private/Friend, MustInherit, Partial Class, verschachtelte Klasse |
| `03_members.vb` | Felder (mehrere Deklaratoren, mit Initialisierer), Konstanten, Auto-Properties, Properties mit Get und Set, Events | Dim und Feld ohne Dim, Const, Feld mit Komma-Liste, Auto-Property, Property mit Get/Set, Event mit Typ, Shared |
| `04_methods_bodies.vb` | Verwendungen in Methodenkörpern: Aufrufe mit Argumentzahl, Erzeugung mit New, Aufruf über Objekt, Zuweisung, verschachtelte If- und For-Blöcke | Sub, Function mit Rückgabe, Aufruf mit 0/1/3 Argumenten, New-Ausdruck, Qualifier-Aufruf, Zuweisung, Me.Feld =, If-Block, For-Block, Return |
| `05_modules_interfaces_enums.vb` | Module, Interface mit Methodensignaturen ohne Körper, Structure, Enum mit Werten | Module mit Sub Main, Interface mit Sub und Property ohne Körper, Structure, Enum mit Wert, Enum ohne Wert |
| `06_attributes_docs.vb` | Attribute an Typ und Member mit Argumenten, XML-Dokumentation mit Summary, Parametern und Rückgabe | Attribut am Typ, mehrere Attribute in einer Liste, Attribut mit Argument am Member, Doku mit summary, param, returns, remarks |
| `07_masking.vb` | Texte und Kommentare dürfen keine Symbole erzeugen. Anführungszeichen werden verdoppelt, Zeilenfortsetzungen und Doppelpunkt-Anweisungen werden richtig gelesen. | Kommentar mit Apostroph, REM-Kommentar, String mit "" Escape, Zeilenfortsetzung mit _, Doppelpunkt trennt Anweisungen, Bezeichner in eckigen Klammern |
| `08_case_insensitive.vb` | VB ist nicht case-sensitiv. Schlüsselwörter und Namen dürfen in beliebiger Schreibweise auftauchen, die Symbol-ID bleibt die Schreibweise der Deklaration. | gemischte Groß- und Kleinschreibung bei Schlüsselwörtern, Aufruf mit anderer Schreibweise als Deklaration, END in Großbuchstaben |
| `09_broken.vb` | Kaputter VB-Code darf nicht abstürzen. Gute Deklarationen nach dem Fehler bleiben erhalten. | Klasse ohne End Class, Sub mit fehlender Klammer, Namespace ohne Namen, gute Funktion danach |
| `10_bom_crlf.vb` | VB mit UTF-8-BOM, Umlauten und Windows-Zeilenenden | BOM, CRLF, Umlaute in Bezeichnern |
| `11_newer_constructs.vb` | Neuere VB-Konstrukte: async und await, Tupel als Rückgabetyp, Iterator mit Yield, Interpolation, If als Ausdruck und NameOf | Async Function mit Task(Of T), Await-Aufruf, benannte Tupel als Rückgabetyp (Namen sind keine Typen), Iterator/Yield, $-Interpolation, If(a, b, c) ohne Aufruf, NameOf ohne Aufruf |
