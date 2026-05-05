package inquisitor

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/packages"
)

// AnalyzeTypes computes LCOM4, CBO, method count, and field count for every
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

		methods := collectStructMethods(named)
		if len(methods) == 0 {
			continue
		}

		methodIndex := make(map[string]int)
		for i, m := range methods {
			methodIndex[m.name] = i
		}

		methodAST := findMethodASTs(pkg, named, methods)
		details, methodFieldSets, methodCallSets := buildFieldAccessSets(pkg, st, methods, methodAST, methodIndex)
		lcom4, components := computeLCOM4(methods, methodFieldSets, methodCallSets, methodIndex)
		clusters := extractClusters(lcom4, components)
		cbo := computeCBO(pkg, st, methods, methodAST)

		results = append(results, &Type{
			Name:          tn.Name(),
			Package:       pkg.PkgPath,
			File:          filepath.Base(pkg.Fset.Position(tn.Pos()).Filename),
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

// collectStructMethods returns the direct (non-promoted) methods on the named
// struct type via its pointer method set. Returns nil if no methods exist.
func collectStructMethods(named *types.Named) []methodInfo {
	mset := types.NewMethodSet(types.NewPointer(named))
	if mset.Len() == 0 {
		return nil
	}

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
	return methods
}

// buildFieldAccessSets walks each method's AST body to determine which fields
// it accesses and which sibling methods it calls on the receiver.
func buildFieldAccessSets(pkg *packages.Package, st *types.Struct, methods []methodInfo, methodAST map[string]*ast.FuncDecl, methodIndex map[string]int) ([]MethodDetail, []map[string]bool, []map[string]bool) {
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
				w := &methodBodyWalker{
					recvObj:     recvVar,
					info:        pkg.TypesInfo,
					methodIndex: methodIndex,
				}
				w.walk(fd.Body, fieldSet, callSet)
			}
		}

		details[i].FieldsUsed = sortedKeys(fieldSet)
		methodFieldSets[i] = fieldSet
		methodCallSets[i] = callSet
	}

	return details, methodFieldSets, methodCallSets
}

// computeLCOM4 uses union-find to group methods into connected components based
// on shared field accesses and intra-type method calls. Returns the LCOM4 value
// and the component map (root index -> method names).
func computeLCOM4(methods []methodInfo, methodFieldSets []map[string]bool, methodCallSets []map[string]bool, methodIndex map[string]int) (int, map[int][]string) {
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

	return len(components), components
}

// extractClusters converts the component map into sorted clusters for display.
// Returns nil when LCOM4 <= 1 (type is cohesive).
func extractClusters(lcom4 int, components map[int][]string) [][]string {
	if lcom4 <= 1 {
		return nil
	}

	var clusters [][]string
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

	return clusters
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

// methodBodyWalker holds stable context for walking method bodies to detect
// field accesses and sibling method calls on the receiver.
type methodBodyWalker struct {
	recvObj     *types.Var
	info        *types.Info
	methodIndex map[string]int
}

// walk inspects a method body for field accesses and method calls on the receiver.
func (w *methodBodyWalker) walk(body *ast.BlockStmt, fields map[string]bool, calls map[string]bool) {
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		return w.inspectNode(n, fields, calls)
	})
}

// inspectNode handles a single AST node during method body inspection,
// recording field accesses and sibling method calls on the receiver.
func (w *methodBodyWalker) inspectNode(n ast.Node, fields map[string]bool, calls map[string]bool) bool {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return true
	}

	// Check if X is the receiver variable.
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return true
	}
	if w.info.Uses[ident] != w.recvObj {
		return true
	}

	w.recordSelector(sel, fields, calls)
	return true
}

// recordSelector classifies a selector expression on the receiver as either
// a field access or a sibling method call, and records it accordingly.
func (w *methodBodyWalker) recordSelector(sel *ast.SelectorExpr, fields map[string]bool, calls map[string]bool) {
	var obj types.Object
	if s := w.info.Selections[sel]; s != nil {
		obj = s.Obj()
	} else {
		obj = w.info.Uses[sel.Sel]
	}
	if obj == nil {
		return
	}
	switch o := obj.(type) {
	case *types.Var:
		if o.IsField() {
			fields[o.Name()] = true
		}
	case *types.Func:
		if w.isSiblingMethod(o.Name()) {
			calls[o.Name()] = true
		}
	}
}

// isSiblingMethod reports whether name is a method on the same receiver type.
func (w *methodBodyWalker) isSiblingMethod(name string) bool {
	_, ok := w.methodIndex[name]
	return ok
}

// computeCBO counts distinct external types referenced by this type's fields and methods.
func computeCBO(pkg *packages.Package, st *types.Struct, methods []methodInfo, methodAST map[string]*ast.FuncDecl) int {
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

// collectTypeNames iteratively extracts named types from a types.Type using a
// work queue, recording external named types found along the way.
func collectTypeNames(t types.Type, currentPkg *types.Package, seen map[string]bool) {
	queue := []types.Type{t}
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if named, ok := cur.(*types.Named); ok {
			tn := named.Obj()
			if tn.Pkg() != nil && tn.Pkg() != currentPkg {
				key := tn.Pkg().Path() + "." + tn.Name()
				seen[key] = true
			}
		}
		queue = appendChildTypes(queue, cur)
	}
}

// appendChildTypes returns the child types to explore within t, appending them
// to the provided queue. This handles unwrapping pointers, containers,
// signatures, interfaces, structs, and generic type arguments.
func appendChildTypes(queue []types.Type, t types.Type) []types.Type {
	switch tt := t.(type) {
	case *types.Named:
		if targs := tt.TypeArgs(); targs != nil {
			for i := 0; i < targs.Len(); i++ {
				queue = append(queue, targs.At(i))
			}
		}
	case *types.Pointer:
		queue = append(queue, tt.Elem())
	case *types.Slice:
		queue = append(queue, tt.Elem())
	case *types.Array:
		queue = append(queue, tt.Elem())
	case *types.Map:
		queue = append(queue, tt.Key(), tt.Elem())
	case *types.Chan:
		queue = append(queue, tt.Elem())
	case *types.Signature:
		queue = appendTupleTypes(queue, tt.Params())
		queue = appendTupleTypes(queue, tt.Results())
	case *types.Interface:
		for i := 0; i < tt.NumMethods(); i++ {
			if sig, ok := tt.Method(i).Type().(*types.Signature); ok {
				queue = appendTupleTypes(queue, sig.Params())
				queue = appendTupleTypes(queue, sig.Results())
			}
		}
	case *types.Struct:
		for i := 0; i < tt.NumFields(); i++ {
			queue = append(queue, tt.Field(i).Type())
		}
	}
	return queue
}

// appendTupleTypes appends all types from a tuple to the work queue.
func appendTupleTypes(queue []types.Type, tup *types.Tuple) []types.Type {
	if tup == nil {
		return queue
	}
	for i := 0; i < tup.Len(); i++ {
		queue = append(queue, tup.At(i).Type())
	}
	return queue
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
