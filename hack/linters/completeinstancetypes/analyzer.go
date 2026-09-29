// Package completeinstancetypes enforces "unavailable, not absent": functions that
// build the GetInstanceTypes result must return every instance type. Instead of
// dropping an unusable one, set Available=false on its affected offerings.
//
// Scope is opt-in via a directive on the function doc comment:
//
//	// +karpenter:complete-instance-types
//
// Inside a marked function, the analyzer reports:
//   - a range loop that conditionally skips (continue / guarded append) while
//     appending *cloudprovider.InstanceType
//   - lo.Filter / lo.Reject over []*cloudprovider.InstanceType
//
// Intentional exceptions: //nolint:completeinstancetypes // <why>
package completeinstancetypes

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const (
	directive = "+karpenter:complete-instance-types"
	nolint    = "nolint:completeinstancetypes"
	cpPkg     = "sigs.k8s.io/karpenter/pkg/cloudprovider"
	msg       = "instance type dropped from a complete-instance-types result; set Available=false on its offerings instead"
)

var Analyzer = &analysis.Analyzer{
	Name:     "completeinstancetypes",
	Doc:      "reports instance types dropped (instead of marked unavailable) in GetInstanceTypes paths",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Body == nil || fn.Doc == nil || !strings.Contains(fn.Doc.Text(), directive) {
			return
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.RangeStmt:
				checkLoop(pass, n)
			case *ast.CallExpr:
				checkFilter(pass, n)
			}
			return true
		})
	})
	return nil, nil
}

// checkLoop flags a loop that appends *InstanceType but can skip iterations.
func checkLoop(pass *analysis.Pass, loop *ast.RangeStmt) {
	var appends []*ast.CallExpr
	guarded, skips := false, token.NoPos
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		ast.Inspect(n, func(c ast.Node) bool {
			switch c := c.(type) {
			case *ast.FuncLit, *ast.RangeStmt, *ast.ForStmt:
				return c == n // don't descend into nested loops/closures
			case *ast.IfStmt:
				if c != n {
					before := len(appends)
					walk(c.Body, depth+1)
					if c.Else != nil {
						walk(c.Else, depth+1)
					}
					if len(appends) > before {
						guarded = true
					}
					return false
				}
			case *ast.BranchStmt:
				if c.Tok == token.CONTINUE && c.Label == nil && skips == token.NoPos {
					skips = c.Pos()
				}
			case *ast.CallExpr:
				if isAppendOfInstanceType(pass, c) {
					appends = append(appends, c)
				}
			}
			return true
		})
	}
	walk(loop.Body, 0)
	if len(appends) == 0 || (skips == token.NoPos && !guarded) {
		return
	}
	pos := skips
	if pos == token.NoPos {
		pos = appends[0].Pos()
	}
	report(pass, pos, msg)
}

func checkFilter(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "lo" || (sel.Sel.Name != "Filter" && sel.Sel.Name != "Reject") {
		return
	}
	if isInstanceTypeSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
		report(pass, call.Pos(), "lo."+sel.Sel.Name+" over instance types in a complete-instance-types result; set Available=false on offerings instead")
	}
}

func isAppendOfInstanceType(pass *analysis.Pass, c *ast.CallExpr) bool {
	id, ok := c.Fun.(*ast.Ident)
	if !ok || id.Name != "append" || len(c.Args) < 2 {
		return false
	}
	return isInstanceTypeSlice(pass.TypesInfo.TypeOf(c.Args[0]))
}

func isInstanceTypeSlice(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	p, ok := s.Elem().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := p.Elem().(*types.Named)
	return ok && named.Obj().Name() == "InstanceType" && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == cpPkg
}

// report honours a trailing or preceding //nolint:completeinstancetypes comment.
func report(pass *analysis.Pass, pos token.Pos, m string) {
	line := pass.Fset.Position(pos).Line
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) != pass.Fset.File(pos) {
			continue
		}
		for _, cg := range f.Comments {
			// Use raw comment text: CommentGroup.Text() strips directive-style comments like //nolint:x.
			for _, c := range cg.List {
				l := pass.Fset.Position(c.Pos()).Line
				if (l == line || l == line-1) && strings.Contains(c.Text, nolint) {
					return
				}
			}
		}
	}
	pass.Reportf(pos, "%s", m)
}
