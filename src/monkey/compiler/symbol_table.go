package compiler

type SymbolScope string

const (
	GlobalScope SymbolScope = "GLOBAL"
	LocalScope  SymbolScope = "LOCAL"
)

type Symbol struct {
	Name  string
	Scope SymbolScope
	Index int
}

type SymbolTable struct {
	Outer          *SymbolTable
	store          map[string]Symbol
	numDefinitions int
}

func NewSymbolTable() *SymbolTable {
	s := make(map[string]Symbol)
	return &SymbolTable{store: s}
}

func NewEnclosedSymbolTable(outer *SymbolTable) *SymbolTable {
	t := NewSymbolTable()
	t.Outer = outer
	return t
}

func (t *SymbolTable) Define(name string) Symbol {
	symbol := Symbol{Name: name, Index: t.numDefinitions}
	if t.Outer == nil {
		symbol.Scope = GlobalScope
	} else {
		symbol.Scope = LocalScope
	}
	t.store[name] = symbol
	t.numDefinitions++
	return symbol
}

func (t *SymbolTable) Resolve(name string) (Symbol, bool) {
	symbol, exists := t.store[name]
	if !exists {
		symbol, exists = t.Outer.Resolve(name)
	}
	return symbol, exists
}
