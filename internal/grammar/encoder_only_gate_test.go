package grammar

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Wave 0 / invariant I7 (the forcing-function cure applied to the cockpit): the cell-grammar encoder —
// EncodeCell + RenderFacetRow — is the ONE legal row renderer. The typed datom kinds (Command/Event/Trace/
// Turn/Task/Dispatch) still carry bespoke Render*Row strips; those are TRACKED MIGRATION DEBT — each migrates
// to the encoder and is removed from legacyRenderRowDebt. This gate FAILS THE BUILD if a NEW bespoke
// Render*Row appears (regression), so the set can only SHRINK toward zero (the hard gate: no Render*Row
// outside the encoder at all). A pane that renders rows outside the encoder is structurally impossible once
// the debt reaches zero — enforce-not-exhort, not a docs convention.

// canonicalRenderRowFuncs are the encoder's own row composer(s) — allowed, not debt.
var canonicalRenderRowFuncs = map[string]bool{
	"RenderFacetRow": true, // the universal row composer over EncodeCell cells
}

// specialRenderRowFuncs render NON-datom rows (a pane-contract row, not a typed datom) — allowed, not debt.
var specialRenderRowFuncs = map[string]bool{
	"RenderAxisRow": true, // renders an Axis five-tuple contract row (a pane-contract, not a datom)
}

// legacyRenderRowDebt is the tracked set of bespoke TYPED-DATOM row renderers that pre-date the encoder and
// must migrate to EncodeCell/RenderFacetRow. Each migration removes a name here; when this reaches zero the
// gate below becomes the hard gate (no Render*Row outside the encoder survives).
var legacyRenderRowDebt = map[string]bool{
	"RenderCommandRow":      true,
	"RenderEventRow":        true,
	"RenderTraceRow":        true,
	"RenderTurnRow":         true,
	"RenderTaskRow":         true,
	"RenderDispatchRow":     true,
	"RenderSessionRow":      true,
	"RenderIdentityRow":     true,
	"RenderConsentFacetRow": true,
}

func TestEncodeCellIsTheOnlyLegalRowRenderer(t *testing.T) {
	// collect every Render*Row func declared in this package (non-test .go files)
	fset := token.NewFileSet()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	found := map[string]bool{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil {
				continue
			}
			name := fd.Name.Name
			if strings.HasPrefix(name, "Render") && strings.HasSuffix(name, "Row") {
				found[name] = true
			}
		}
		return nil
	})

	// 1. REGRESSION GATE: a Render*Row that is neither canonical, special, nor tracked debt is a NEW bespoke
	//    renderer — FAIL. The debt set may only shrink; it may never silently grow.
	for name := range found {
		if canonicalRenderRowFuncs[name] || specialRenderRowFuncs[name] || legacyRenderRowDebt[name] {
			continue
		}
		t.Errorf(
			"new bespoke row renderer %q appeared — EncodeCell/RenderFacetRow is the only legal typed-datom "+
				"renderer (migrate it to the encoder; do NOT add new Render*Row). If it renders a non-datom "+
				"row, declare it in specialRenderRowFuncs with a rationale.",
			name,
		)
	}

	// 2. DEBT HONESTY: a name in legacyRenderRowDebt that no longer exists in the package has been migrated —
	//    REMOVE it from the debt set (a stale debt entry hides progress + weakens the gate).
	for name := range legacyRenderRowDebt {
		if !found[name] {
			t.Errorf(
				"legacyRenderRowDebt lists %q but it no longer exists in the package — it was migrated; remove "+
					"it from legacyRenderRowDebt (a stale debt entry hides migration progress)",
				name,
			)
		}
	}

	// 3. THE HARD GATE (arms when the migration completes): once legacyRenderRowDebt is empty, NO bespoke
	//    Render*Row may remain (only the canonical encoder + the declared specials).
	if len(legacyRenderRowDebt) == 0 {
		offenders := []string{}
		for name := range found {
			if !canonicalRenderRowFuncs[name] && !specialRenderRowFuncs[name] {
				offenders = append(offenders, name)
			}
		}
		sort.Strings(offenders)
		if len(offenders) > 0 {
			t.Errorf(
				"the typed-datom migration is complete (legacyRenderRowDebt is empty) but bespoke Render*Row "+
					"funcs remain: %v — migrate them to EncodeCell/RenderFacetRow",
				offenders,
			)
		}
	}
}
