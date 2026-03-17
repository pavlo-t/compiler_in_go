package compiler

type SymbolScope string

const (
	BuiltinScope  SymbolScope = "BUILTIN"
	GlobalScope   SymbolScope = "GLOBAL"
	LocalScope    SymbolScope = "LOCAL"
	FreeScope     SymbolScope = "FREE"
	FunctionScope SymbolScope = "FUNCTION"
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
	FreeSymbols    []Symbol
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
	if !exists && t.Outer != nil {
		symbol, exists = t.Outer.Resolve(name)
		if symbol.Scope != BuiltinScope && symbol.Scope != GlobalScope {
			symbol = t.defineFree(symbol)
		}
	}
	return symbol, exists
}

func (t *SymbolTable) DefineBuiltin(index int, name string) Symbol {
	symbol := Symbol{Name: name, Index: index, Scope: BuiltinScope}
	t.store[name] = symbol
	return symbol
}

func (t *SymbolTable) defineFree(original Symbol) Symbol {
	t.FreeSymbols = append(t.FreeSymbols, original)
	symbol := Symbol{Name: original.Name, Index: len(t.FreeSymbols) - 1}
	symbol.Scope = FreeScope
	t.store[original.Name] = symbol
	return symbol
}

func (t *SymbolTable) DefineFunctionName(name string) Symbol {
	symbol := Symbol{Name: name, Scope: FunctionScope}
	t.store[name] = symbol
	return symbol
}
