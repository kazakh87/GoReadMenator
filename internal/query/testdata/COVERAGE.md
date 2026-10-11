# Testabdeckung Abfragen

Automatisch erzeugt. Jeder Fall ist eine Eingabedatei mit Golden-Datei.

| Datei | Szenario | Abgedeckt |
|---|---|---|
| `01_suche_und_details/` | Suche, Details und Eindeutigkeit: Treffer nach Namensteil, genauer Treffer zuerst, Art-Filter, mehrdeutiger Name mit Auswahl, Status des Index | find_symbols, Art-Filter, get_symbol mit Dokumentation und Fundorten, mehrdeutiger Name als Fehler mit Kandidaten, api mit Sichtbarkeit, status |
| `02_budget_und_paginierung/` | Das Token-Budget begrenzt jede Antwort. Über den Cursor lassen sich alle Einträge seitenweise abrufen, ohne Doppelte und ohne Lücken. | api mit kleinem Budget, nextCursor, Fortsetzung über mehrere Seiten, Budget-Untergrenze, find_symbols mit Limit |
| `03_beziehungen_und_kontext/` | Beziehungen, Verwendungen und das Kontextpaket. Quelltext nur ausschnittsweise und begrenzt. Unbekannte Eingaben liefern verständliche Fehler. | neighborhood mit Tiefe und Beziehungsart, usages mit Tiefe 2, context im Budget, source mit Zeilenlimit, source für unbekannte Datei, unbekanntes Symbol |
| `04_http_endpoints/` | Routen des Backends mit den Frontend-Aufrufen, die sie erreichen. Abgefragt über die Endpunkt-Liste: Route, Aktion, Datei und Aufrufer. | Liste der Routen mit Aktion und Zeile, Aufrufer aus Frontend per Verbindung, Route ohne Aufrufer, Minimal-API, Status |
