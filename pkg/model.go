package inquisitor

// Module represents the top-level analyzed Go module.
type Module struct {
	Path     string
	Packages []*Package
	Lines    int
}

// Package represents an analyzed Go package with coupling and abstraction metrics.
type Package struct {
	Name            string
	Path            string // import path
	Functions       []*Function
	Types           []*Type
	Ca              int     // afferent coupling - packages depending on this one
	Ce              int     // efferent coupling - packages this one depends on
	I               float64 // Ce / (Ca + Ce)
	A               float64 // interfaces / total types
	D               float64 // |A + I - 1|
	ExportedSymbols int
	Lines           int
	Imports         []string // internal import paths (within analyzed set)
}

// Type represents an analyzed Go type with cohesion and coupling metrics.
type Type struct {
	Name          string
	Package       string // package import path
	File          string // base filename where type is declared
	LCOM4         int
	CBO           int
	Methods       int
	Fields        int
	methodDetails []methodDetail // for LCOM4 computation
	Clusters      [][]string     // method clusters when LCOM4 > 1
}

// methodDetail records which fields a method accesses, used for LCOM4 computation.
type methodDetail struct {
	Name       string
	FieldsUsed []string // field names accessed
}

// Function represents an analyzed Go function with complexity metrics.
type Function struct {
	Name            string
	Package         string // package import path
	File            string // base filename where function is declared
	Receiver        string // empty for free functions
	PointerReceiver bool
	IsTest          bool // true if name starts with "Test" and file is _test.go
	Cog             int
	Cyc             int
	FanIn           int
	FanOut          int
	Lines           int
	StartLine       int
	EndLine         int
}
