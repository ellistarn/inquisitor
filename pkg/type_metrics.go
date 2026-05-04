package inquisitor

import (
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// analyzeTypes computes LCOM4, CBO, method count, and field count for every
// named struct type declared in the analyzed packages.
func AnalyzeTypes(pkgs []*packages.Package, analyzedPaths map[string]bool) []*Type {
	var results []*Type
	for _, pkg := range pkgs {
		if !analyzedPaths[pkg.PkgPath] {
			continue
		}
		results = append(results, analyzePackageTypes(pkg)...)
	}
	return results
}

func analyzePackageTypes(pkg *packages.Package) []*Type {
	var results []*Type

	for _, obj := range pkg.TypesInfo.Defs {
		tn, ok := obj.(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}

		// Collect methods via pointer method set (includes both pointer and value receivers).
		mset := types.NewMethodSet(types.NewPointer(named))
		if mset.Len() == 0 {
			continue // skip struct with no methods (LCOM4 not applicable)
		}

		// Collect method funcs.
		var methods []methodInfo
		for i := 0; i < mset.Len(); i++ {
			sel := mset.At(i)
			fn, ok := sel.Obj().(*types.Func)
			if !ok {
				continue
			}
			// Only include methods directly declared on this type (not promoted).
			if len(sel.Index()) != 1 {
				continue
			}
			methods = append(methods, methodInfo{name: fn.Name(), fn: fn})
		}

		if len(methods) == 0 {
			continue
		}

		// Map method names to indices.
		methodIndex := make(map[string]int)
		for i, m := range methods {
			methodIndex[m.name] = i
		}

		// Find AST func decls for each method.
		methodAST := findMethodASTs(pkg, named, methods)

		// For each method, determine field accesses and method calls on receiver.
		details := make([]MethodDetail, len(methods))
		methodFieldSets := make([]map[string]bool, len(methods))
		methodCallSets := make([]map[string]bool, len(methods))

		for i, m := range methods {
			details[i].Name = m.name
			fieldSet := make(map[string]bool)
			callSet := make(map[string]bool)

			if fd, ok := methodAST[m.name]; ok {
				recvVar := resolveReceiverVar(fd, pkg.TypesInfo)
				if recvVar != nil {
					walkMethodBody(fd.Body, recvVar, pkg.TypesInfo, st, fieldSet, callSet, methodIndex)
				}
			}

			fields := sortedKeys(fieldSet)
			details[i].FieldsUsed = fields
			methodFieldSets[i] = fieldSet
			methodCallSets[i] = callSet
		}

		// Build union-find for LCOM4.
		n := len(methods)
		uf := newUnionFind(n)

		// Union methods sharing fields.
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if shareField(methodFieldSets[i], methodFieldSets[j]) {
					uf.union(i, j)
				}
			}
		}

		// Union methods connected by calls.
		for i := 0; i < n; i++ {
			for callee := range methodCallSets[i] {
				if j, ok := methodIndex[callee]; ok {
					uf.union(i, j)
				}
			}
		}

		// Count connected components.
		components := make(map[int][]string)
		for i := 0; i < n; i++ {
			root := uf.find(i)
			components[root] = append(components[root], methods[i].name)
		}
		lcom4 := len(components)

		// Build clusters when LCOM4 > 1.
		var clusters [][]string
		if lcom4 > 1 {
			for _, members := range components {
				sort.Strings(members)
				clusters = append(clusters, members)
			}
			// Sort clusters by size descending, then by first member name.
			sort.Slice(clusters, func(i, j int) bool {
				if len(clusters[i]) != len(clusters[j]) {
					return len(clusters[i]) > len(clusters[j])
				}
				return clusters[i][0] < clusters[j][0]
			})
		}

		// Compute CBO.
		cbo := computeCBO(pkg, named, st, methods, methodAST)

		results = append(results, &Type{
			Name:          tn.Name(),
			Package:       pkg.PkgPath,
			LCOM4:         lcom4,
			CBO:           cbo,
			Methods:       len(methods),
			Fields:        st.NumFields(),
			MethodDetails: details,
			Clusters:      clusters,
		})
	}

	return results
}

// derefType strips pointer wrappers.
func derefType(t types.Type) types.Type {
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

// resolveReceiverVar returns the *types.Var for the receiver of a FuncDecl
// by looking it up through TypesInfo.
func resolveReceiverVar(fd *ast.FuncDecl, info *types.Info) *types.Var {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return nil
	}
	names := fd.Recv.List[0].Names
	if len(names) == 0 {
		return nil
	}
	obj := info.Defs[names[0]]
	if obj == nil {
		return nil
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return nil
	}
	return v
}

// findMethodASTs locates AST FuncDecls for methods on the given named type.
func findMethodASTs(pkg *packages.Package, named *types.Named, methods []methodInfo) map[string]*ast.FuncDecl {
	result := make(map[string]*ast.FuncDecl)
	want := make(map[string]bool)
	for _, m := range methods {
		want[m.name] = true
	}

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			if !want[fd.Name.Name] {
				continue
			}
			// Check that the receiver type matches our named type.
			recvType := resolveRecvType(fd.Recv.List[0].Type, pkg.TypesInfo)
			if recvType == named {
				result[fd.Name.Name] = fd
			}
		}
	}
	return result
}

// resolveRecvType extracts the *types.Named from a receiver type expression,
// stripping pointer indirection.
func resolveRecvType(expr ast.Expr, info *types.Info) *types.Named {
	t := info.TypeOf(expr)
	if t == nil {
		return nil
	}
	t = derefType(t)
	if named, ok := t.(*types.Named); ok {
		return named
	}
	return nil
}

// walkMethodBody inspects a method body for field accesses and method calls on the receiver.
func walkMethodBody(body *ast.BlockStmt, recvObj *types.Var, info *types.Info, st *types.Struct, fields map[string]bool, calls map[string]bool, methodIndex map[string]int) {
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Check if X is the receiver variable.
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		use := info.Uses[ident]
		if use != recvObj {
			return true
		}

		// Resolve what the selector refers to.
		selObj := info.Selections[sel]
		if selObj != nil {
			switch obj := selObj.Obj().(type) {
			case *types.Var:
				if obj.IsField() {
					fields[obj.Name()] = true
				}
			case *types.Func:
				if _, ok := methodIndex[obj.Name()]; ok {
					calls[obj.Name()] = true
				}
			}
			return true
		}

		// Fallback: check Uses for the selector ident.
		selUse := info.Uses[sel.Sel]
		if selUse != nil {
			switch obj := selUse.(type) {
			case *types.Var:
				if obj.IsField() {
					fields[obj.Name()] = true
				}
			case *types.Func:
				if _, ok := methodIndex[obj.Name()]; ok {
					calls[obj.Name()] = true
				}
			}
		}
		return true
	})
}

// computeCBO counts distinct external types referenced by this type's fields and methods.
func computeCBO(pkg *packages.Package, named *types.Named, st *types.Struct, methods []methodInfo, methodAST map[string]*ast.FuncDecl) int {
	currentPkg := pkg.Types
	seen := make(map[string]bool) // "pkgpath.Name" dedup key

	// Collect from struct fields.
	for i := 0; i < st.NumFields(); i++ {
		collectTypeNames(st.Field(i).Type(), currentPkg, seen)
	}

	// Collect from method signatures and bodies.
	for _, m := range methods {
		sig, ok := m.fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		collectTupleTypeNames(sig.Params(), currentPkg, seen)
		collectTupleTypeNames(sig.Results(), currentPkg, seen)

		if fd, ok := methodAST[m.name]; ok && fd.Body != nil {
			collectBodyTypeNames(fd.Body, pkg.TypesInfo, currentPkg, seen)
		}
	}

	return len(seen)
}

// collectTypeNames recursively extracts named types from a types.Type.
func collectTypeNames(t types.Type, currentPkg *types.Package, seen map[string]bool) {
	switch tt := t.(type) {
	case *types.Named:
		tn := tt.Obj()
		if tn.Pkg() != nil && tn.Pkg() != currentPkg {
			key := tn.Pkg().Path() + "." + tn.Name()
			seen[key] = true
		}
		// Check type arguments.
		if targs := tt.TypeArgs(); targs != nil {
			for i := 0; i < targs.Len(); i++ {
				collectTypeNames(targs.At(i), currentPkg, seen)
			}
		}
	case *types.Pointer:
		collectTypeNames(tt.Elem(), currentPkg, seen)
	case *types.Slice:
		collectTypeNames(tt.Elem(), currentPkg, seen)
	case *types.Array:
		collectTypeNames(tt.Elem(), currentPkg, seen)
	case *types.Map:
		collectTypeNames(tt.Key(), currentPkg, seen)
		collectTypeNames(tt.Elem(), currentPkg, seen)
	case *types.Chan:
		collectTypeNames(tt.Elem(), currentPkg, seen)
	case *types.Signature:
		collectTupleTypeNames(tt.Params(), currentPkg, seen)
		collectTupleTypeNames(tt.Results(), currentPkg, seen)
	case *types.Interface:
		for i := 0; i < tt.NumMethods(); i++ {
			if sig, ok := tt.Method(i).Type().(*types.Signature); ok {
				collectTupleTypeNames(sig.Params(), currentPkg, seen)
				collectTupleTypeNames(sig.Results(), currentPkg, seen)
			}
		}
	case *types.Struct:
		for i := 0; i < tt.NumFields(); i++ {
			collectTypeNames(tt.Field(i).Type(), currentPkg, seen)
		}
	}
}

func collectTupleTypeNames(tup *types.Tuple, currentPkg *types.Package, seen map[string]bool) {
	if tup == nil {
		return
	}
	for i := 0; i < tup.Len(); i++ {
		collectTypeNames(tup.At(i).Type(), currentPkg, seen)
	}
}

// collectBodyTypeNames walks an AST body and collects external type references.
func collectBodyTypeNames(body *ast.BlockStmt, info *types.Info, currentPkg *types.Package, seen map[string]bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[ident]
		if obj == nil {
			return true
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			return true
		}
		if tn.Pkg() != nil && tn.Pkg() != currentPkg {
			key := tn.Pkg().Path() + "." + tn.Name()
			seen[key] = true
		}
		return true
	})
}

// --- Union-Find ---

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	uf := &unionFind{
		parent: make([]int, n),
		rank:   make([]int, n),
	}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (uf *unionFind) find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]] // path splitting
		x = uf.parent[x]
	}
	return x
}

func (uf *unionFind) union(x, y int) {
	rx, ry := uf.find(x), uf.find(y)
	if rx == ry {
		return
	}
	if uf.rank[rx] < uf.rank[ry] {
		rx, ry = ry, rx
	}
	uf.parent[ry] = rx
	if uf.rank[rx] == uf.rank[ry] {
		uf.rank[rx]++
	}
}

// --- helpers ---

func shareField(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type methodInfo struct {
	name string
	fn   *types.Func
}
