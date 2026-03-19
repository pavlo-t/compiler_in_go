package repl

import (
	"bufio"
	"fmt"
	"io"
	"monkey/ast"
	"monkey/compiler"
	"monkey/evaluator"
	"monkey/lexer"
	"monkey/object"
	"monkey/parser"
	"monkey/vm"
)

const PROMPT = ">> "

func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)

	var constants []object.Object
	symbolTable := compiler.NewSymbolTable()
	for i, b := range object.Builtins {
		symbolTable.DefineBuiltin(i, b.Name)
	}
	globals := make([]object.Object, vm.GlobalsSize)

	macroEnv := object.NewEnvironment()

	for {
		fmt.Fprintf(out, PROMPT)
		scanned := scanner.Scan()
		if !scanned {
			return
		}

		line := scanner.Text()
		l := lexer.New(line)
		p := parser.New(l)

		program := p.ParseProgram()
		if len(p.Errors()) != 0 {
			printParserErrors(out, p.Errors())
			continue
		}

		evaluator.DefineMacros(program, macroEnv)
		expanded := evaluator.ExpandMacros(program, macroEnv).(*ast.Program)

		comp := compiler.NewForRepl(constants, symbolTable)
		err := comp.Compile(expanded)
		if err != nil {
			fmt.Fprintf(out, "Compilation failed:\n %s\n", err)
			continue
		}

		constants = comp.Bytecode().Constants

		machine := vm.NewForRepl(comp.Bytecode(), globals)
		err = machine.Run()
		if err != nil {
			fmt.Fprintf(out, "Bytecode execution failed:\n %s\n", err)
		}

		stackTop := machine.LastPoppedStackElem()
		if stackTop != nil {
			io.WriteString(out, stackTop.Inspect())
			io.WriteString(out, "\n")
		}
	}
}

func printParserErrors(out io.Writer, errors []string) {
	io.WriteString(out, "\x1b[31m")
	io.WriteString(out, "Parser errors:\n")
	for _, msg := range errors {
		io.WriteString(out, "\t"+msg+"\n")
	}
	io.WriteString(out, "\x1b[0m")
}
