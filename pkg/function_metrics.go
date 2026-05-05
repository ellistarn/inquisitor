package inquisitor

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// fnResult pairs a function with the set of callees it invokes.
type fnResult struct {
	fn      *Function
	callees map[string]bool // set of callee keys (pkg.Path().Name)
}

// AnalyzeFunctions computes per-function complexity metrics across all packages.
// analyzedPaths scopes fan-in counting to internal callers only.
func AnalyzeFunctions(pkgs []*packages.Package, analyzedPaths map[string]bool) []*Function {
	results := computeFunctionMetrics(pkgs)
	return computeFanIn(results, analyzedPaths)
}

// computeFunctionMetrics iterates all packages and functions, computing
// per-function metrics (cognitive complexity, cyclomatic complexity, fan-out, lines).
func computeFunctionMetrics(pkgs []*packages.Package) []fnResult {
	var results []fnResult

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				f := &Function{
					Name:    fd.Name.Name,
					Package: pkg.PkgPath,
				}

				// Receiver
				if fd.Recv != nil && len(fd.Recv.List) > 0 {
					f.Receiver = receiverTypeName(fd.Recv.List[0].Type)
					_, f.PointerReceiver = fd.Recv.List[0].Type.(*ast.StarExpr)
				}

				// Lines
				pos := pkg.Fset.Position(fd.Pos())
				f.StartLine = pos.Line
				f.File = filepath.Base(pos.Filename)
				f.EndLine = pkg.Fset.Position(fd.End()).Line
				f.Lines = f.EndLine - f.StartLine + 1

				// Cognitive complexity
				f.Cognitive = cognitiveComplexity(fd)

				// Cyclomatic complexity
				f.Cyclomatic = cyclomaticComplexity(fd)

			// Fan-out: distinct functions called
			callees := fanOut(fd, pkg.TypesInfo)
			// Exclude recursive self-calls from fan-out
			selfKey := funcKey(f.Package, f.Receiver, f.Name)
			delete(callees, selfKey)
			f.FanOut = len(callees)

			results = append(results, fnResult{fn: f, callees: callees})
			}
		}
	}

	return results
}

// computeFanIn aggregates fan-in by scanning each function's callees,
// counting only callers from analyzedPaths as internal callers.
func computeFanIn(results []fnResult, analyzedPaths map[string]bool) []*Function {
	fanInCount := map[string]int{}
	for _, r := range results {
		if !analyzedPaths[r.fn.Package] {
			continue
		}
		callerKey := funcKey(r.fn.Package, r.fn.Receiver, r.fn.Name)
		for key := range r.callees {
			// Exclude recursive self-calls from fan-in
			if key == callerKey {
				continue
			}
			fanInCount[key]++
		}
	}

	functions := make([]*Function, len(results))
	for i, r := range results {
		key := funcKey(r.fn.Package, r.fn.Receiver, r.fn.Name)
		r.fn.FanIn = fanInCount[key]
		functions[i] = r.fn
	}

	return functions
}

// funcKey produces a unique identity string for a function.
func funcKey(pkgPath, receiver, name string) string {
	if receiver != "" {
		return pkgPath + ".(" + receiver + ")." + name
	}
	return pkgPath + "." + name
}

// funcKeyFromObj produces the same identity string from a types.Func.
func funcKeyFromObj(fn *types.Func) string {
	sig := fn.Type().(*types.Signature)
	pkg := fn.Pkg()
	pkgPath := ""
	if pkg != nil {
		pkgPath = pkg.Path()
	}
	recv := sig.Recv()
	if recv != nil {
		typeName := receiverTypeNameFromType(recv.Type())
		return pkgPath + ".(" + typeName + ")." + fn.Name()
	}
	return pkgPath + "." + fn.Name()
}

// receiverTypeName extracts the type name from a receiver AST expression,
// stripping any pointer indirection.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic with single type param
		return receiverTypeName(t.X)
	case *ast.IndexListExpr: // generic with multiple type params
		return receiverTypeName(t.X)
	default:
		return ""
	}
}

// receiverTypeNameFromType extracts the base type name from a receiver's type,
// stripping pointers.
func receiverTypeNameFromType(t types.Type) string {
	switch v := t.(type) {
	case *types.Pointer:
		return receiverTypeNameFromType(v.Elem())
	case *types.Named:
		return v.Obj().Name()
	default:
		return ""
	}
}

// --- Cognitive Complexity ---

func cognitiveComplexity(fd *ast.FuncDecl) int {
	if fd.Body == nil {
		return 0
	}
	c := &cognitiveVisitor{}
	c.walkStmtList(fd.Body.List, 0)
	return c.complexity
}

type cognitiveVisitor struct {
	complexity int
}

func (c *cognitiveVisitor) walkStmtList(stmts []ast.Stmt, nesting int) {
	for _, stmt := range stmts {
		c.walkStmt(stmt, nesting)
	}
}

func (c *cognitiveVisitor) walkStmt(stmt ast.Stmt, nesting int) {
	switch s := stmt.(type) {
	// --- Control flow: increment + nesting penalty + recurse into body ---
	case *ast.IfStmt:
		c.walkNested(s.Body.List, nesting)
		c.walkStmtOpt(s.Init, nesting)
		c.walkExpr(s.Cond, nesting)
		if s.Else != nil {
			c.walkElse(s.Else, nesting)
		}
	case *ast.ForStmt:
		c.walkNested(s.Body.List, nesting)
		c.walkStmtOpt(s.Init, nesting)
		c.walkExprOpt(s.Cond, nesting)
		c.walkStmtOpt(s.Post, nesting)
	case *ast.RangeStmt:
		c.walkNested(s.Body.List, nesting)
	case *ast.SwitchStmt:
		c.walkNested(s.Body.List, nesting)
		c.walkStmtOpt(s.Init, nesting)
		c.walkExprOpt(s.Tag, nesting)
	case *ast.TypeSwitchStmt:
		c.walkNested(s.Body.List, nesting)
		c.walkStmtOpt(s.Init, nesting)
		c.walkStmtOpt(s.Assign, nesting)
	case *ast.SelectStmt:
		c.walkNested(s.Body.List, nesting)

	// --- Branch statements: goto, labeled break/continue ---
	case *ast.BranchStmt:
		c.walkBranchStmt(s)

	// --- Clauses and structural wrappers ---
	case *ast.BlockStmt:
		c.walkStmtList(s.List, nesting)
	case *ast.CaseClause:
		c.walkExprs(s.List, nesting)
		c.walkStmtList(s.Body, nesting)
	case *ast.CommClause:
		c.walkStmtOpt(s.Comm, nesting)
		c.walkStmtList(s.Body, nesting)
	case *ast.LabeledStmt:
		c.walkStmt(s.Stmt, nesting)

	// --- Expressions, assignments, declarations: just recurse ---
	case *ast.ExprStmt:
		c.walkExpr(s.X, nesting)
	case *ast.AssignStmt:
		c.walkExprs(s.Rhs, nesting)
		c.walkExprs(s.Lhs, nesting)
	case *ast.ReturnStmt:
		c.walkExprs(s.Results, nesting)
	case *ast.SendStmt:
		c.walkExpr(s.Chan, nesting)
		c.walkExpr(s.Value, nesting)
	case *ast.IncDecStmt:
		c.walkExpr(s.X, nesting)
	case *ast.GoStmt:
		c.walkExpr(s.Call, nesting)
	case *ast.DeferStmt:
		c.walkExpr(s.Call, nesting)
	case *ast.DeclStmt:
		c.walkDeclStmt(s, nesting)
	}
}

// walkNested handles the common control-flow pattern: increment complexity by
// 1 (structural) + nesting penalty, then recurse into the body at nesting+1.
func (c *cognitiveVisitor) walkNested(body []ast.Stmt, nesting int) {
	c.complexity += 1 + nesting
	c.walkStmtList(body, nesting+1)
}

// walkBranchStmt increments complexity for goto, labeled break, or labeled continue.
func (c *cognitiveVisitor) walkBranchStmt(s *ast.BranchStmt) {
	if s.Tok == token.GOTO || (s.Label != nil && (s.Tok == token.BREAK || s.Tok == token.CONTINUE)) {
		c.complexity++
	}
}

// walkDeclStmt walks value initializers in a declaration statement.
func (c *cognitiveVisitor) walkDeclStmt(s *ast.DeclStmt, nesting int) {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		c.walkExprs(vs.Values, nesting)
	}
}

// walkStmtOpt walks a statement if non-nil.
func (c *cognitiveVisitor) walkStmtOpt(stmt ast.Stmt, nesting int) {
	if stmt != nil {
		c.walkStmt(stmt, nesting)
	}
}

// walkExprOpt walks an expression if non-nil.
func (c *cognitiveVisitor) walkExprOpt(expr ast.Expr, nesting int) {
	if expr != nil {
		c.walkExpr(expr, nesting)
	}
}

// walkExprs walks a slice of expressions.
func (c *cognitiveVisitor) walkExprs(exprs []ast.Expr, nesting int) {
	for _, expr := range exprs {
		c.walkExpr(expr, nesting)
	}
}

func (c *cognitiveVisitor) walkElse(stmt ast.Stmt, nesting int) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		// else-if: +1 structural, NO nesting increase
		c.complexity++
		if s.Init != nil {
			c.walkStmt(s.Init, nesting)
		}
		c.walkExpr(s.Cond, nesting)
		c.walkStmtList(s.Body.List, nesting+1)
		if s.Else != nil {
			c.walkElse(s.Else, nesting)
		}
	case *ast.BlockStmt:
		// else: +1 structural
		c.complexity++
		c.walkStmtList(s.List, nesting+1)
	}
}

func (c *cognitiveVisitor) walkExpr(expr ast.Expr, nesting int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if e.Op == token.LAND || e.Op == token.LOR {
			c.walkBooleanExpr(e, token.ILLEGAL, nesting) // ILLEGAL = no parent op
		} else {
			c.walkExpr(e.X, nesting)
			c.walkExpr(e.Y, nesting)
		}
	case *ast.FuncLit:
		// Closure increases nesting but no structural penalty
		if e.Body != nil {
			c.walkStmtList(e.Body.List, nesting+1)
		}
	case *ast.CallExpr:
		c.walkExpr(e.Fun, nesting)
		for _, arg := range e.Args {
			c.walkExpr(arg, nesting)
		}
	case *ast.UnaryExpr:
		c.walkExpr(e.X, nesting)
	case *ast.ParenExpr:
		c.walkExpr(e.X, nesting)
	case *ast.IndexExpr:
		c.walkExpr(e.X, nesting)
		c.walkExpr(e.Index, nesting)
	case *ast.SliceExpr:
		c.walkExpr(e.X, nesting)
		c.walkExpr(e.Low, nesting)
		c.walkExpr(e.High, nesting)
		c.walkExpr(e.Max, nesting)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			c.walkExpr(elt, nesting)
		}
	case *ast.KeyValueExpr:
		c.walkExpr(e.Value, nesting)
	case *ast.SelectorExpr:
		c.walkExpr(e.X, nesting)
	case *ast.TypeAssertExpr:
		c.walkExpr(e.X, nesting)
	case *ast.StarExpr:
		c.walkExpr(e.X, nesting)
	}
}

// walkBooleanExpr traverses a chain of && / || binary expressions,
// incrementing complexity when the operator changes or appears for the first time.
func (c *cognitiveVisitor) walkBooleanExpr(expr *ast.BinaryExpr, parentOp token.Token, nesting int) {
	// Process left operand
	if left, ok := expr.X.(*ast.BinaryExpr); ok && (left.Op == token.LAND || left.Op == token.LOR) {
		c.walkBooleanExpr(left, expr.Op, nesting)
	} else {
		c.walkExpr(expr.X, nesting)
	}

	// +1 if this operator differs from parent or is the first in the chain
	if expr.Op != parentOp {
		c.complexity++
	}

	// Process right operand
	if right, ok := expr.Y.(*ast.BinaryExpr); ok && (right.Op == token.LAND || right.Op == token.LOR) {
		c.walkBooleanExpr(right, expr.Op, nesting)
	} else {
		c.walkExpr(expr.Y, nesting)
	}
}

// --- Cyclomatic Complexity ---

func cyclomaticComplexity(fd *ast.FuncDecl) int {
	if fd.Body == nil {
		return 1
	}
	complexity := 1
	ast.Inspect(fd, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncLit:
			return false // don't count closure bodies in enclosing function's cyclomatic
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			// Each case except default adds a decision path
			if s.List != nil {
				complexity++
			}
		case *ast.CommClause:
			// Each comm case except default
			if s.Comm != nil {
				complexity++
			}
		case *ast.BinaryExpr:
			if s.Op == token.LAND || s.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// --- Fan-out ---

// fanOut returns the set of distinct function keys called from the function body.
func fanOut(fd *ast.FuncDecl, info *types.Info) map[string]bool {
	if fd.Body == nil || info == nil {
		return nil
	}
	callees := map[string]bool{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := resolveCallee(call, info)
		if fn == nil {
			return true
		}
		callees[funcKeyFromObj(fn)] = true
		return true
	})
	return callees
}

// resolveCallee resolves a CallExpr to the *types.Func it calls, if possible.
// Returns nil for type conversions, builtin calls, and indirect/interface calls.
func resolveCallee(call *ast.CallExpr, info *types.Info) *types.Func {
	var ident *ast.Ident

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil
	}

	obj := info.Uses[ident]
	if obj == nil {
		// Try ObjectOf for cases where Uses doesn't have it
		obj = info.ObjectOf(ident)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	// Interface methods can't be statically resolved to a concrete implementation.
	if sig, ok := fn.Type().(*types.Signature); ok {
		if recv := sig.Recv(); recv != nil {
			if _, isIface := recv.Type().Underlying().(*types.Interface); isIface {
				return nil
			}
		}
	}
	return fn
}
