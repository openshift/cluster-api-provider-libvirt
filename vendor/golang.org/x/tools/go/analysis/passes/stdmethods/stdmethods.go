// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdmethods

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name: "stdmethods",
	Doc: `check signature of methods of well-known interfaces

Sometimes a type may be intended to satisfy an interface but may fail to
do so because of a mistake in its method signature.
For example, the result of this WriteTo method should be (int64, error),
not error, to satisfy io.WriterTo:

	type myWriterTo struct{...}
        func (myWriterTo) WriteTo(w io.Writer) error { ... }

This check ensures that each method whose name matches one of several
well-known interface methods from the standard library has the correct
signature for that interface.

Checked method names include:
	Format GobEncode GobDecode MarshalJSON MarshalXML
	Peek ReadByte ReadFrom ReadRune Scan Seek
	UnmarshalJSON UnreadByte UnreadRune WriteByte
	WriteTo
`,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// canonicalMethods lists the input and output types for Go methods
// that are checked using dynamic interface checks. Because the
// checks are dynamic, such methods would not cause a compile error
// if they have the wrong signature: instead the dynamic check would
// fail, sometimes mysteriously. If a method is found with a name listed
// here but not the input/output types listed here, vet complains.
//
// A few of the canonical methods have very common names.
// For example, a type might implement a Scan method that
// has nothing to do with fmt.Scanner, but we still want to check
// the methods that are intended to implement fmt.Scanner.
// To do that, the arguments that have a = prefix are treated as
// signals that the canonical meaning is intended: if a Scan
// method doesn't have a fmt.ScanState as its first argument,
// we let it go. But if it does have a fmt.ScanState, then the
// rest has to match.
var canonicalMethods = map[string]struct{ args, results []string }{
	// "Flush": {{}, {"error"}}, // http.Flusher and jpeg.writer conflict
	"Format":        {[]string{"=fmt.State", "rune"}, []string{}},                      // fmt.Formatter
	"GobDecode":     {[]string{"[]byte"}, []string{"error"}},                           // gob.GobDecoder
	"GobEncode":     {[]string{}, []string{"[]byte", "error"}},                         // gob.GobEncoder
	"MarshalJSON":   {[]string{}, []string{"[]byte", "error"}},                         // json.Marshaler
	"MarshalXML":    {[]string{"*xml.Encoder", "xml.StartElement"}, []string{"error"}}, // xml.Marshaler
	"ReadByte":      {[]string{}, []string{"byte", "error"}},                           // io.ByteReader
	"ReadFrom":      {[]string{"=io.Reader"}, []string{"int64", "error"}},              // io.ReaderFrom
	"ReadRune":      {[]string{}, []string{"rune", "int", "error"}},                    // io.RuneReader
	"Scan":          {[]string{"=fmt.ScanState", "rune"}, []string{"error"}},           // fmt.Scanner
	"Seek":          {[]string{"=int64", "int"}, []string{"int64", "error"}},           // io.Seeker
	"UnmarshalJSON": {[]string{"[]byte"}, []string{"error"}},                           // json.Unmarshaler
	"UnmarshalXML":  {[]string{"*xml.Decoder", "xml.StartElement"}, []string{"error"}}, // xml.Unmarshaler
	"UnreadByte":    {[]string{}, []string{"error"}},
	"UnreadRune":    {[]string{}, []string{"error"}},
	"WriteByte":     {[]string{"byte"}, []string{"error"}},                // jpeg.writer (matching bufio.Writer)
	"WriteTo":       {[]string{"=io.Writer"}, []string{"int64", "error"}}, // io.WriterTo
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.InterfaceType)(nil),
	}
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Recv != nil {
				canonicalMethod(pass, n.Name, n.Type)
			}
		case *ast.InterfaceType:
			for _, field := range n.Methods.List {
				for _, id := range field.Names {
					canonicalMethod(pass, id, field.Type.(*ast.FuncType))
				}
			}
		}
	})
	return nil, nil
}

func canonicalMethod(pass *analysis.Pass, id *ast.Ident, t *ast.FuncType) {
	// Expected input/output.
	expect, ok := canonicalMethods[id.Name]
	if !ok {
		return
	}

	// Actual input/output
	args := typeFlatten(t.Params.List)
	var results []ast.Expr
	if t.Results != nil {
		results = typeFlatten(t.Results.List)
	}

	// Do the =s (if any) all match?
	if !matchParams(pass, expect.args, args, "=") || !matchParams(pass, expect.results, results, "=") {
		return
	}

	// Everything must match.
	if !matchParams(pass, expect.args, args, "") || !matchParams(pass, expect.results, results, "") {
		expectFmt := id.Name + "(" + argjoin(expect.args) + ")"
		if len(expect.results) == 1 {
			expectFmt += " " + argjoin(expect.results)
		} else if len(expect.results) > 1 {
			expectFmt += " (" + argjoin(expect.results) + ")"
		}

		var buf bytes.Buffer
		if err := printer.Fprint(&buf, pass.Fset, t); err != nil {
			fmt.Fprintf(&buf, "<%s>", err)
		}
		actual := buf.String()
		actual = strings.TrimPrefix(actual, "func")
		actual = id.Name + actual

		pass.Reportf(id.Pos(), "method %s should have signature %s", actual, expectFmt)
	}
}

func argjoin(x []string) string {
	y := make([]string, len(x))
	for i, s := range x {
		if s[0] == '=' {
			s = s[1:]
		}
		y[i] = s
	}
	return strings.Join(y, ", ")
}

// Turn parameter list into slice of types
// (in the ast, types are Exprs).
// Have to handle f(int, bool) and f(x, y, z int)
// so not a simple 1-to-1 conversion.
func typeFlatten(l []*ast.Field) []ast.Expr {
	var t []ast.Expr
	for _, f := range l {
		if len(f.Names) == 0 {
			t = append(t, f.Type)
			continue
		}
		for range f.Names {
			t = append(t, f.Type)
		}
	}
	return t
}

// Does each type in expect with the given prefix match the corresponding type in actual?
func matchParams(pass *analysis.Pass, expect []string, actual []ast.Expr, prefix string) bool {
	for i, x := range expect {
		if !strings.HasPrefix(x, prefix) {
			continue
		}
		if i >= len(actual) {
			return false
		}
		if !matchParamType(pass.Fset, pass.Pkg, x, actual[i]) {
			return false
		}
	}
	if prefix == "" && len(actual) > len(expect) {
		return false
	}
	return true
}

// Does this one type match?
func matchParamType(fset *token.FileSet, pkg *types.Package, expect string, actual ast.Expr) bool {
	expect = strings.TrimPrefix(expect, "=")
	// Strip package name if we're in that package.
	if n := len(pkg.Name()); len(expect) > n && expect[:n] == pkg.Name() && expect[n] == '.' {
		expect = expect[n+1:]
	}

	// Overkill but easy.
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, actual)
	return buf.String() == expect
}
