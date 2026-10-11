// Package cli verbindet die Pakete zu den Befehlen von goreadmenator.
//
// Exit-Codes: 0 Erfolg, 1 Laufzeitfehler oder nichts gefunden, 2 Nutzungsfehler,
// mehrdeutiger Name (Kandidaten werden ausgegeben) oder Index nicht aktuell.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"

	"goreadmenator/internal/export"
	"goreadmenator/internal/impact"
	"goreadmenator/internal/langver"
	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/resolve"
	"goreadmenator/internal/store"
	_ "goreadmenator/internal/store/memory"
	_ "goreadmenator/internal/store/snapshot"
	_ "goreadmenator/internal/store/sqlite"
	"goreadmenator/internal/update"
	"goreadmenator/internal/xmlimport"
)

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

const usage = `goreadmenator – Symbol- und Verwendungsindex für C# und TypeScript/JavaScript

Verwendung:
  goreadmenator [Optionen] <Befehl> [Argumente]

Befehle:
  index                    Index aktualisieren (Vollaufbau oder Inkrement)
  symbol <name>            Symbole mit diesem Namen oder FQN anzeigen
  refs <name>              Verwendungen eines Symbols anzeigen
  impact <name>            Verwendungsketten eines Symbols anzeigen
  diff <alt>..<neu>        Geänderte Symbole zwischen zwei Commits und ihre Wirkung
  export <format>          Graph exportieren: json, jsonl, mermaid oder markdown
  import-xml <datei.xml>   Symbole aus einer .NET-XML-Dokumentation importieren (ohne Quelltext)
  mcp                      MCP-Server für KI-Assistenten starten (stdio, nur Lesezugriff)
  project add|list|remove  Projekte für den MCP-Server verwalten (Registry)
  serve                    HTTP-JSON-Schnittstelle starten (nur lesend, Standard 127.0.0.1:7878)

Optionen:
`

// App ist die Ausführungsumgebung der Befehle.
type App struct {
	Stdout io.Writer
	Stderr io.Writer

	RepoDir string
	Backend string
	DBPath  string
	JSON    bool
	Depth   int
	ID      string // exakte Symbol-ID, verhindert Mehrdeutigkeit
	Workers int
	Batch   int
	MaxSize int64
	Out     string
	Scope   string
	Members bool

	CPUProfile string
	MemProfile string
	MemLimitMB int64

	Registry     string // Pfad der Projektregistry (leer = Standard im Benutzerverzeichnis)
	Addr         string // Adresse der HTTP-Schnittstelle
	Token        string // Bearer-Token der HTTP-Schnittstelle (optional, sonst READMENATOR_TOKEN)
	RepoExplicit bool
	DBExplicit   bool
}

// Run führt die Befehlszeile aus und liefert den Exit-Code.
func (a *App) Run(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("goreadmenator", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.Usage = func() {
		fmt.Fprint(a.Stderr, usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&a.RepoDir, "repo", ".", "Pfad zum Git-Repository")
	fs.StringVar(&a.Backend, "store", "snapshot", "Speicher-Backend ("+strings.Join(store.Backends(), ", ")+")")
	fs.StringVar(&a.Addr, "addr", "127.0.0.1:7878", "Adresse der HTTP-Schnittstelle (serve)")
	fs.StringVar(&a.Token, "token", "", "Bearer-Token der HTTP-Schnittstelle (serve)")
	fs.StringVar(&a.Registry, "registry", "", "Pfad der Projektregistry (Standard: ~/.goreadmenator/projects.json)")
	fs.StringVar(&a.DBPath, "db", "", "Pfad zum Index (Standard: <repo>/.goreadmenator/index.snap)")
	fs.BoolVar(&a.JSON, "json", false, "Ausgabe als JSON")
	fs.IntVar(&a.Depth, "depth", 3, "Tiefe der Impact-Analyse")
	fs.StringVar(&a.ID, "id", "", "exakte Symbol-ID statt Name")
	fs.IntVar(&a.Workers, "workers", 0, "Anzahl paralleler Parser (0 = Kerne)")
	fs.IntVar(&a.Batch, "batch", 1000, "Datensätze pro Schreibtransaktion")
	fs.Int64Var(&a.MaxSize, "max-size", 5<<20, "größte Datei in Bytes, die indiziert wird")
	fs.StringVar(&a.Out, "out", "", "Ausgabedatei (Standard: stdout; bei markdown ein Verzeichnis)")
	fs.StringVar(&a.Scope, "scope", "", "Präfix des vollqualifizierten Namens (nur mermaid)")
	fs.BoolVar(&a.Members, "members", false, "Methoden und Properties in den Mermaid-Kästen zeigen")
	fs.Int64Var(&a.MemLimitMB, "mem-limit", 0, "weiches Speicherlimit in MB (0 = keines); der Garbage Collector arbeitet dann häufiger")
	fs.StringVar(&a.CPUProfile, "cpuprofile", "", "CPU-Profil in Datei schreiben (für pprof)")
	fs.StringVar(&a.MemProfile, "memprofile", "", "Speicherprofil nach dem Lauf in Datei schreiben")
	// Optionen dürfen vor und nach Befehl und Argumenten stehen (`refs --id X name`).
	var positional []string
	remaining := args
	for {
		if err := fs.Parse(remaining); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return ExitOK
			}
			return ExitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		remaining = fs.Args()[1:]
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "repo":
			a.RepoExplicit = true
		case "db":
			a.DBExplicit = true
		}
	})
	if a.DBPath == "" {
		a.DBPath = defaultDBPath(a.RepoDir)
	}
	if a.Workers <= 0 {
		a.Workers = defaultWorkers()
	}
	if a.MemLimitMB > 0 {
		debug.SetMemoryLimit(a.MemLimitMB << 20)
	}
	if stop, err := a.startProfiling(); err != nil {
		fmt.Fprintln(a.Stderr, "Profiling:", err)
		return ExitError
	} else {
		defer stop()
	}
	rest := positional
	if len(rest) == 0 {
		fs.Usage()
		return ExitUsage
	}
	cmd, params := rest[0], rest[1:]
	switch cmd {
	case "index":
		return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdIndex(ctx, st) })
	case "symbol":
		return a.needArg(params, func(name string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdSymbol(ctx, st, name) })
		})
	case "refs":
		return a.needArg(params, func(name string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdRefs(ctx, st, name) })
		})
	case "impact":
		return a.needArg(params, func(name string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdImpact(ctx, st, name) })
		})
	case "mcp":
		return a.cmdMCP(ctx, os.Stdin, os.Stdout)
	case "project":
		return a.cmdProject(params)
	case "serve":
		return a.cmdServe(ctx, a.Addr, a.Token)
	case "import-xml":
		return a.needArg(params, func(file string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdImportXML(ctx, st, file) })
		})
	case "export":
		return a.needArg(params, func(format string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdExport(ctx, st, format) })
		})
	case "diff":
		return a.needArg(params, func(spec string) int {
			return a.withStore(ctx, func(st store.Store) (int, error) { return a.cmdDiff(ctx, st, spec) })
		})
	default:
		fmt.Fprintf(a.Stderr, "unbekannter Befehl: %q\n", cmd)
		fs.Usage()
		return ExitUsage
	}
}

// startProfiling startet optionale Profile. Die zurückgegebene Funktion schreibt sie.
func (a *App) startProfiling() (func(), error) {
	var cpuFile *os.File
	if a.CPUProfile != "" {
		f, err := os.Create(a.CPUProfile)
		if err != nil {
			return nil, err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return nil, err
		}
		cpuFile = f
	}
	return func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			cpuFile.Close()
		}
		if a.MemProfile != "" {
			if f, err := os.Create(a.MemProfile); err == nil {
				runtime.GC()
				pprof.WriteHeapProfile(f)
				f.Close()
			}
		}
	}, nil
}

func (a *App) needArg(params []string, fn func(string) int) int {
	if len(params) != 1 {
		fmt.Fprintln(a.Stderr, "genau ein Argument erwartet")
		return ExitUsage
	}
	return fn(params[0])
}

// withStore öffnet den Store, führt den Befehl aus und schließt den Store (speichert ihn).
func (a *App) withStore(ctx context.Context, fn func(store.Store) (int, error)) int {
	st, err := store.Open(ctx, a.Backend, a.DBPath)
	if err != nil {
		fmt.Fprintln(a.Stderr, "Index öffnen:", err)
		return ExitError
	}
	code, runErr := fn(st)
	if cerr := st.Close(); cerr != nil && runErr == nil {
		runErr = fmt.Errorf("Index speichern: %w", cerr)
	}
	if runErr != nil {
		fmt.Fprintln(a.Stderr, "Fehler:", runErr)
		if code == ExitOK {
			code = ExitError
		}
	}
	return code
}

func (a *App) registry() (*parser.Registry, error) {
	return parsers.Registry()
}

// --- index ---

func (a *App) cmdIndex(ctx context.Context, st store.Store) (int, error) {
	reg, err := a.registry()
	if err != nil {
		return ExitError, err
	}
	res, err := update.Run(ctx, st, update.Options{
		RepoDir: a.RepoDir, Registry: reg, Workers: a.Workers, BatchSize: a.Batch, MaxFileSizeBytes: a.MaxSize,
	})
	if err != nil {
		return ExitError, err
	}
	if a.JSON {
		return ExitOK, a.writeJSON(map[string]any{
			"mode": res.Mode, "from": res.From, "to": res.To,
			"parsed": res.Parsed, "removed": res.Removed,
			"symbols": res.Index.Symbols, "references": res.Index.References,
			"resolve": res.Resolve, "errors": len(res.Index.Errors),
		})
	}
	switch res.Mode {
	case "noop":
		fmt.Fprintf(a.Stdout, "Index ist aktuell (%s).\n", short(res.To))
	default:
		fmt.Fprintf(a.Stdout, "Index %s auf %s: %d Dateien neu geparst, %d entfernt, %d Symbole, %d Verwendungen.\n",
			res.Mode, short(res.To), res.Parsed, res.Removed, res.Index.Symbols, res.Index.References)
		scope := "alle Verwendungen"
		if res.Mode == "incremental" {
			scope = "Verwendungen betroffener Namen"
		}
		fmt.Fprintf(a.Stdout, "Auflösung (%s): %d EXACT, %d RESOLVED, %d HEURISTIC, %d unaufgelöst, %d Beziehungen abgeleitet.\n",
			scope, res.Resolve.Exact, res.Resolve.Resolved, res.Resolve.Heuristic, res.Resolve.Unresolved, res.Resolve.Relations)
	}
	a.printWarnings(res)
	if n := len(res.Index.Errors); n > 0 {
		return ExitError, fmt.Errorf("%d Datei(en) nicht indiziert, zuerst %s: %v", n, res.Index.Errors[0].Path, res.Index.Errors[0].Err)
	}
	return ExitOK, nil
}

// --- Symbolauswahl ---

// pick sucht Symbole über ID, Name oder FQN-Endung. Bei mehreren Treffern ohne --id wird abgebrochen.
func (a *App) pick(ctx context.Context, st store.Store, name string) (model.Symbol, bool, error) {
	if a.ID != "" {
		s, ok, err := st.Symbol(ctx, model.SymbolID(a.ID))
		return s, ok, err
	}
	cands, err := a.candidates(ctx, st, name)
	if err != nil {
		return model.Symbol{}, false, err
	}
	switch len(cands) {
	case 0:
		return model.Symbol{}, false, nil
	case 1:
		return cands[0], true, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q ist mehrdeutig. Mit --id eine der Varianten wählen:\n", name)
		for _, c := range cands {
			fmt.Fprintf(&b, "  %s  (%s)\n", c.ID, c.Kind)
		}
		return model.Symbol{}, false, &ambiguousError{msg: b.String()}
	}
}

type ambiguousError struct{ msg string }

func (e *ambiguousError) Error() string { return e.msg }

func (a *App) candidates(ctx context.Context, st store.Store, name string) ([]model.Symbol, error) {
	byName, err := st.SymbolsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(byName) > 0 {
		return byName, nil
	}
	all, err := st.AllSymbols(ctx)
	if err != nil {
		return nil, err
	}
	var out []model.Symbol
	for _, s := range all {
		if s.FQN == name || strings.HasSuffix(s.FQN, "."+name) || strings.HasSuffix(s.FQN, "#"+name) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (a *App) handleAmbiguous(err error) int {
	var amb *ambiguousError
	if errors.As(err, &amb) {
		fmt.Fprint(a.Stderr, amb.msg)
		return ExitUsage
	}
	return -1
}

// --- symbol ---

type locJSON struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	StartCol  int    `json:"startCol"`
	EndLine   int    `json:"endLine"`
	EndCol    int    `json:"endCol"`
	Role      string `json:"role"`
}

func (a *App) cmdSymbol(ctx context.Context, st store.Store, name string) (int, error) {
	cands, err := a.candidatesFor(ctx, st, name)
	if err != nil {
		return ExitError, err
	}
	if len(cands) == 0 {
		fmt.Fprintf(a.Stderr, "kein Symbol %q gefunden\n", name)
		return ExitError, nil
	}
	files := fileNames(ctx, st)
	type symOut struct {
		ID        model.SymbolID   `json:"id"`
		Kind      model.SymbolKind `json:"kind"`
		FQN       string           `json:"fqn"`
		Signature string           `json:"signature,omitempty"`
		Summary   string           `json:"summary,omitempty"`
		Returns   string           `json:"returns,omitempty"`
		Locations []locJSON        `json:"locations"`
	}
	var out []symOut
	for _, s := range cands {
		locs, err := st.Locations(ctx, s.ID)
		if err != nil {
			return ExitError, err
		}
		o := symOut{ID: s.ID, Kind: s.Kind, FQN: s.FQN, Signature: s.Signature, Summary: s.Summary, Returns: s.Returns}
		for _, l := range locs {
			o.Locations = append(o.Locations, locJSON{File: files[l.FileID], StartLine: l.Span.StartLine, StartCol: l.Span.StartCol,
				EndLine: l.Span.EndLine, EndCol: l.Span.EndCol, Role: string(l.Role)})
		}
		out = append(out, o)
	}
	if a.JSON {
		return ExitOK, a.writeJSON(out)
	}
	for _, o := range out {
		fmt.Fprintf(a.Stdout, "%s %s\n", o.Kind, o.FQN)
		fmt.Fprintf(a.Stdout, "  ID: %s\n", o.ID)
		if o.Signature != "" {
			fmt.Fprintf(a.Stdout, "  Signatur: %s\n", o.Signature)
		}
		for _, l := range o.Locations {
			fmt.Fprintf(a.Stdout, "  %s:%d-%d", l.File, l.StartLine, l.EndLine)
			if l.Role == string(model.RolePartial) {
				fmt.Fprint(a.Stdout, " (partial)")
			}
			fmt.Fprintln(a.Stdout)
		}
		if o.Summary != "" {
			fmt.Fprintf(a.Stdout, "  %s\n", o.Summary)
		}
	}
	return ExitOK, nil
}

// candidatesFor liefert alle Treffer ohne Mehrdeutigkeitsprüfung (für symbol).
func (a *App) candidatesFor(ctx context.Context, st store.Store, name string) ([]model.Symbol, error) {
	if a.ID != "" {
		s, ok, err := st.Symbol(ctx, model.SymbolID(a.ID))
		if err != nil || !ok {
			return nil, err
		}
		return []model.Symbol{s}, nil
	}
	return a.candidates(ctx, st, name)
}

func fileNames(ctx context.Context, st store.Store) map[model.FileID]string {
	out := map[model.FileID]string{}
	files, _ := st.Files(ctx)
	for _, f := range files {
		out[f.ID] = f.Path
	}
	return out
}

// --- refs ---

type refJSON struct {
	File       string           `json:"file"`
	Line       int              `json:"line"`
	Col        int              `json:"col"`
	EndLine    int              `json:"endLine"`
	Kind       model.RefKind    `json:"kind"`
	Enclosing  model.SymbolID   `json:"enclosing,omitempty"`
	Confidence model.Confidence `json:"confidence"`
	Name       string           `json:"name"`
}

func (a *App) cmdRefs(ctx context.Context, st store.Store, name string) (int, error) {
	sym, ok, err := a.pick(ctx, st, name)
	if code := a.handleAmbiguous(err); code >= 0 {
		return code, nil
	}
	if err != nil {
		return ExitError, err
	}
	if !ok {
		fmt.Fprintf(a.Stderr, "kein Symbol %q gefunden\n", name)
		return ExitError, nil
	}
	files := fileNames(ctx, st)
	to, err := st.ReferencesTo(ctx, sym.ID)
	if err != nil {
		return ExitError, err
	}
	// Ungelöste Verwendungen mit gleichem Namen: nur als Hinweis, nicht als Treffer.
	byName, err := st.ReferencesByName(ctx, sym.Name)
	if err != nil {
		return ExitError, err
	}
	var unresolved []refJSON
	for _, r := range byName {
		if r.TargetID == "" {
			unresolved = append(unresolved, toRefJSON(files, r))
		}
	}
	var resolved []refJSON
	for _, r := range to {
		resolved = append(resolved, toRefJSON(files, r))
	}
	sortRefs(resolved)
	sortRefs(unresolved)

	if a.JSON {
		return ExitOK, a.writeJSON(map[string]any{"symbol": sym.ID, "references": resolved, "unresolvedSameName": unresolved})
	}
	fmt.Fprintf(a.Stdout, "%s — %d Verwendung(en)\n", sym.ID, len(resolved))
	for _, r := range resolved {
		printRef(a.Stdout, r)
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(a.Stdout, "\nNicht eindeutig zugeordnete Stellen mit gleichem Namen (%d):\n", len(unresolved))
		for _, r := range unresolved {
			printRef(a.Stdout, r)
		}
	}
	return ExitOK, nil
}

func toRefJSON(files map[model.FileID]string, r model.Reference) refJSON {
	return refJSON{File: files[r.FileID], Line: r.Span.StartLine, Col: r.Span.StartCol, EndLine: r.Span.EndLine,
		Kind: r.Kind, Enclosing: r.Enclosing, Confidence: r.Confidence, Name: r.TargetName}
}

func sortRefs(rs []refJSON) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].File != rs[j].File {
			return rs[i].File < rs[j].File
		}
		return rs[i].Line < rs[j].Line
	})
}

func printRef(w io.Writer, r refJSON) {
	encl := string(r.Enclosing)
	if encl == "" {
		encl = "(Datei-Ebene)"
	}
	fmt.Fprintf(w, "  %s:%d:%d  %s  in %s  [%s]\n", r.File, r.Line, r.Col, r.Kind, encl, r.Confidence)
}

// --- impact ---

func (a *App) cmdImpact(ctx context.Context, st store.Store, name string) (int, error) {
	sym, ok, err := a.pick(ctx, st, name)
	if code := a.handleAmbiguous(err); code >= 0 {
		return code, nil
	}
	if err != nil {
		return ExitError, err
	}
	if !ok {
		fmt.Fprintf(a.Stderr, "kein Symbol %q gefunden\n", name)
		return ExitError, nil
	}
	hits, err := impact.Analyze(ctx, st, sym.ID, a.Depth)
	if err != nil {
		return ExitError, err
	}
	if a.JSON {
		return ExitOK, a.writeJSON(map[string]any{"symbol": sym.ID, "depth": a.Depth, "hits": hitsJSON(hits)})
	}
	fmt.Fprintf(a.Stdout, "Wirkung von %s (Tiefe %d): %d Stelle(n)\n", sym.ID, a.Depth, len(hits))
	printHits(a.Stdout, hits)
	return ExitOK, nil
}

type hitJSON struct {
	Depth int            `json:"depth"`
	File  string         `json:"file"`
	Line  int            `json:"line"`
	Col   int            `json:"col"`
	Kind  model.RefKind  `json:"kind"`
	User  model.SymbolID `json:"user,omitempty"`
	Via   model.SymbolID `json:"via"`
	Name  string         `json:"name"`
}

func hitsJSON(hits []impact.Hit) []hitJSON {
	out := make([]hitJSON, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitJSON{Depth: h.Depth, File: h.File, Line: h.Span.StartLine, Col: h.Span.StartCol,
			Kind: h.Kind, User: h.User, Via: h.Target, Name: h.Name})
	}
	return out
}

func printHits(w io.Writer, hits []impact.Hit) {
	depth := -1
	for _, h := range hits {
		if h.Depth != depth {
			depth = h.Depth
			fmt.Fprintf(w, "\nTiefe %d:\n", depth)
		}
		user := string(h.User)
		if user == "" {
			user = "(Datei-Ebene)"
		}
		fmt.Fprintf(w, "  %s:%d:%d  %s  durch %s\n", h.File, h.Span.StartLine, h.Span.StartCol, h.Kind, user)
	}
}

// --- diff ---

type changedJSON struct {
	ID     model.SymbolID   `json:"id"`
	Kind   model.SymbolKind `json:"kind"`
	File   string           `json:"file"`
	Start  int              `json:"startLine"`
	End    int              `json:"endLine"`
	Direct bool             `json:"direct"`
	Impact []hitJSON        `json:"impact,omitempty"`
}

func (a *App) cmdDiff(ctx context.Context, st store.Store, spec string) (int, error) {
	parts := strings.Split(spec, "..")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		fmt.Fprintln(a.Stderr, "Format: <alt>..<neu>")
		return ExitUsage, nil
	}
	from, to := parts[0], parts[1]
	head, err := gitHead(ctx, a.RepoDir)
	if err != nil {
		return ExitError, err
	}
	if to != "HEAD" && !strings.HasPrefix(head, to) && to != head {
		// Der Index zeigt den HEAD. Andere Ziele brauchen einen Index genau an diesem Commit.
		fmt.Fprintf(a.Stderr, "Der Index steht auf %s, nicht auf %s. Bitte zuerst im Ziel-Commit \"index\" ausführen.\n", short(head), to)
		return ExitUsage, nil
	}
	reg, err := a.registry()
	if err != nil {
		return ExitError, err
	}
	if _, err := update.Run(ctx, st, update.Options{RepoDir: a.RepoDir, Registry: reg, Workers: a.Workers,
		BatchSize: a.Batch, MaxFileSizeBytes: a.MaxSize}); err != nil {
		return ExitError, err
	}
	changed, err := impact.Changed(ctx, st, a.RepoDir, from, head)
	if err != nil {
		return ExitError, err
	}
	var out []changedJSON
	for _, c := range changed {
		item := changedJSON{ID: c.Symbol.ID, Kind: c.Symbol.Kind, File: c.File, Start: c.Span.StartLine, End: c.Span.EndLine, Direct: c.Direct}
		if c.Direct {
			hits, err := impact.Analyze(ctx, st, c.Symbol.ID, a.Depth)
			if err != nil {
				return ExitError, err
			}
			item.Impact = hitsJSON(hits)
		}
		out = append(out, item)
	}
	if a.JSON {
		return ExitOK, a.writeJSON(map[string]any{"from": from, "to": head, "changed": out})
	}
	fmt.Fprintf(a.Stdout, "%s..%s: %d geänderte Stelle(n)\n", short(from), short(head), len(out))
	for _, c := range out {
		mark := "  "
		if c.Direct {
			mark = "* "
		}
		fmt.Fprintf(a.Stdout, "%s%s %s  %s:%d-%d\n", mark, c.Kind, c.ID, c.File, c.Start, c.End)
		if c.Direct && len(c.Impact) > 0 {
			var hits []impact.Hit
			for _, h := range c.Impact {
				hits = append(hits, impact.Hit{Depth: h.Depth, Target: h.Via, User: h.User, File: h.File,
					Span: model.Span{StartLine: h.Line, StartCol: h.Col}, Kind: h.Kind, Name: h.Name})
			}
			printHits(a.Stdout, hits)
		}
	}
	fmt.Fprintln(a.Stdout, "\n* = direkt betroffen (innerste Stelle)")
	return ExitOK, nil
}

// --- import-xml ---

func (a *App) cmdImportXML(ctx context.Context, st store.Store, file string) (int, error) {
	res, err := xmlimport.Import(ctx, st, file)
	if err != nil {
		return ExitError, err
	}
	rstats, err := resolve.Run(ctx, st, resolve.Options{RepoDir: a.RepoDir})
	if err != nil {
		return ExitError, err
	}
	if a.JSON {
		return ExitOK, a.writeJSON(map[string]any{"assembly": res.Assembly, "members": res.Members,
			"symbols": res.Symbols, "crefs": res.Crefs, "replaced": res.Replaced, "resolve": rstats})
	}
	verb := "importiert"
	if res.Replaced {
		verb = "ersetzt (vorheriger Import)"
	}
	fmt.Fprintf(a.Stdout, "Assembly %s %s: %d Members, %d Symbole, %d cref-Verweise.\n",
		res.Assembly, verb, res.Members, res.Symbols, res.Crefs)
	return ExitOK, nil
}

// --- export ---

func (a *App) cmdExport(ctx context.Context, st store.Store, format string) (int, error) {
	g, err := export.Load(ctx, st)
	if err != nil {
		return ExitError, err
	}
	if format == "markdown" {
		dir := a.Out
		if dir == "" {
			dir = "goreadmenator-md"
		}
		written, err := export.WriteMarkdown(dir, g)
		if err != nil {
			return ExitError, err
		}
		fmt.Fprintf(a.Stdout, "%d Seite(n) in %s geschrieben.\n", len(written), dir)
		return ExitOK, nil
	}
	var w io.Writer = a.Stdout
	if a.Out != "" {
		f, err := os.Create(a.Out)
		if err != nil {
			return ExitError, err
		}
		defer f.Close()
		w = f
	}
	switch format {
	case "json":
		return ExitOK, export.WriteJSON(w, g)
	case "jsonl":
		return ExitOK, export.WriteJSONL(w, g)
	case "mermaid":
		return ExitOK, export.WriteMermaid(w, g, export.MermaidOptions{Scope: a.Scope, Members: a.Members})
	default:
		fmt.Fprintf(a.Stderr, "unbekanntes Format %q (json, jsonl, mermaid, markdown)\n", format)
		return ExitUsage, nil
	}
}

func (a *App) writeJSON(v any) error {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func short(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// defaultDBPath legt den Index im Git-Verzeichnis ab, damit er nie als unversioniert auftaucht.
func defaultDBPath(repo string) string {
	gitDir := filepath.Join(repo, ".git")
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		return filepath.Join(gitDir, "goreadmenator", "index.snap")
	}
	return filepath.Join(repo, ".goreadmenator", "index.snap")
}

func defaultWorkers() int {
	n := os.Getenv("READMENATOR_WORKERS")
	if n == "" {
		return 4
	}
	var v int
	fmt.Sscanf(n, "%d", &v)
	if v <= 0 {
		return 4
	}
	return v
}

// printWarnings zeigt Warnungen zur Sprachabdeckung: Konstrukte in einzelnen Dateien und
// Projekte, deren Sprachversion neuer ist als der Parser. Es wird nur gewarnt, nichts abgebrochen.
func (a *App) printWarnings(res update.Result) {
	projectWarnings := langver.Warnings(a.RepoDir)
	if res.Index.WarningCount == 0 && len(projectWarnings) == 0 {
		return
	}
	fmt.Fprintf(a.Stdout, "Warnungen: %d Konstrukte nur teilweise erfasst, %d Projekthinweise.\n",
		res.Index.WarningCount, len(projectWarnings))
	for _, w := range projectWarnings {
		fmt.Fprintf(a.Stdout, "  Projekt: %s\n", w)
	}
	const show = 10
	for i, w := range res.Index.Warnings {
		if i >= show {
			fmt.Fprintf(a.Stdout, "  … %d weitere (Liste über die Abfrage status im Projekt)\n", res.Index.WarningCount-show)
			break
		}
		fmt.Fprintf(a.Stdout, "  %s:%d %s\n", w.Path, w.Line, w.Message)
	}
}
