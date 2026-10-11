package model

import "strings"

// RefHTTPConvention ist ein Verweis auf konventionsbasiertes Routing. TargetName ist entweder
// "PATTERN <muster>" (aus MapControllerRoute) oder "ACTION <controller>/<aktion>" (eine öffentliche
// Aktion ohne Attributroute). Enclosing ist bei ACTION die Aktion, bei PATTERN der Aufrufer.
const (
	ConventionPattern = "PATTERN "
	ConventionAction  = "ACTION "
)

// ExpandConvention erzeugt die Pfade, unter denen eine Aktion mit einem Konventionsmuster erreichbar ist.
// Platzhalter {controller} und {action} werden ersetzt, weitere Parameter werden zu {}. Optionale Teile
// (mit ? oder =) am Ende erzeugen zusätzlich die Variante ohne sie, weil der Pfad dann auch kürzer sein kann.
func ExpandConvention(pattern, controller, action string) []string {
	segs := strings.Split(strings.Trim(pattern, "/"), "/")
	var full []string
	firstOptional := -1
	for i, seg := range segs {
		if seg == "" {
			continue
		}
		if !strings.HasPrefix(seg, "{") {
			full = append(full, seg)
			firstOptional = -1
			continue
		}
		inner := strings.Trim(seg, "{}")
		base := inner
		optional := false
		if k := strings.IndexAny(inner, "?="); k >= 0 {
			base = inner[:k]
			optional = true
		}
		switch strings.ToLower(base) {
		case "controller":
			// Der Controller ist durch die Aktion festgelegt, sein Segment ist nie optional.
			full = append(full, controller)
			firstOptional = -1
		case "action":
			full = append(full, action)
			firstOptional = -1
		default:
			full = append(full, "{}")
			if optional {
				if firstOptional < 0 {
					firstOptional = i
				}
			} else {
				firstOptional = -1
			}
		}
	}
	out := []string{strings.Join(full, "/")}
	if firstOptional >= 0 {
		// Variante ohne die trailing optionalen Segmente: Anzahl Pfadteile vor dem ersten optionalen Segment
		n := 0
		for _, seg := range segs[:firstOptional] {
			if seg != "" {
				n++
			}
		}
		out = append(out, strings.Join(full[:n], "/"))
	}
	return out
}

// ExpandConventionRoutes wandelt Konventionsverweise in Routen um. Ausgabe sind Verweise vom Typ
// RefHTTPRoute mit dem Verb ANY, die wie Attributrouten behandelt werden. Die Eingabe bleibt unverändert.
func ExpandConventionRoutes(refs []Reference) []Reference {
	var patterns, actions []Reference
	for _, r := range refs {
		if r.Kind != RefHTTPConvention {
			continue
		}
		switch {
		case strings.HasPrefix(r.TargetName, ConventionPattern):
			patterns = append(patterns, r)
		case strings.HasPrefix(r.TargetName, ConventionAction):
			actions = append(actions, r)
		}
	}
	var out []Reference
	seen := map[string]bool{}
	for _, a := range actions {
		ctrl, act, _ := strings.Cut(strings.TrimPrefix(a.TargetName, ConventionAction), "/")
		for _, p := range patterns {
			pattern := strings.TrimPrefix(p.TargetName, ConventionPattern)
			for _, path := range ExpandConvention(pattern, ctrl, act) {
				route := "ANY " + NormalizeRoute(path)
				key := route + "|" + string(a.Enclosing)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, Reference{Kind: RefHTTPRoute, TargetName: route, Enclosing: a.Enclosing,
					FileID: a.FileID, Span: a.Span, Confidence: ConfExact})
			}
		}
	}
	return out
}
