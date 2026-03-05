package vm

import (
	"fmt"
	"monkey/code"
	"monkey/compiler"
	"monkey/object"
)

const StackSize = 2048
const GlobalsSize = 1 << 16

var True = &object.Boolean{Value: true}
var False = &object.Boolean{Value: false}
var Null = &object.Null{}

type VM struct {
	instructions code.Instructions
	constants    []object.Object
	globals      []object.Object
	stack        []object.Object
	sp           int // Always points to the next value. Top of stack is stack[sp-1]
}

func New(bytecode *compiler.Bytecode) *VM {
	return &VM{
		instructions: bytecode.Instructions,
		constants:    bytecode.Constants,
		globals:      make([]object.Object, GlobalsSize),
		stack:        make([]object.Object, StackSize),
		sp:           0,
	}
}

func NewForRepl(
	bytecode *compiler.Bytecode,
	globals []object.Object,
) *VM {
	vm := New(bytecode)
	vm.globals = globals
	return vm
}

func (vm *VM) LastPoppedStackElem() object.Object {
	return vm.stack[vm.sp]
}

func (vm *VM) Run() error {
	for ip := 0; ip < len(vm.instructions); ip++ {
		op := code.Opcode(vm.instructions[ip])

		switch op {
		case code.OpConstant:
			constIndex := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2
			err := vm.push(vm.constants[constIndex])
			if err != nil {
				return err
			}
		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv:
			err := vm.executeBinaryOperation(op)
			if err != nil {
				return err
			}
		case code.OpTrue:
			err := vm.push(True)
			if err != nil {
				return err
			}
		case code.OpFalse:
			err := vm.push(False)
			if err != nil {
				return err
			}
		case code.OpEqual, code.OpNotEqual, code.OpGreaterThan:
			err := vm.executeComparisonOperation(op)
			if err != nil {
				return err
			}
		case code.OpMinus:
			o := vm.pop()
			if o.Type() != object.INTEGER_OBJ {
				return fmt.Errorf("unsupported type for '-' operation: %s, value: %s", o.Type(), o)
			}
			err := vm.push(&object.Integer{Value: -o.(*object.Integer).Value})
			if err != nil {
				return err
			}
		case code.OpBang:
			err := vm.push(nativeBoolToBooleanObject(!isTruthy(vm.pop())))
			if err != nil {
				return err
			}
		case code.OpPop:
			vm.pop()
		case code.OpJumpNotTruthy:
			if isTruthy(vm.pop()) {
				ip += 2
			} else {
				ip = int(code.ReadUint16(vm.instructions[ip+1:])) - 1
			}
		case code.OpJump:
			ip = int(code.ReadUint16(vm.instructions[ip+1:])) - 1
		case code.OpNull:
			err := vm.push(Null)
			if err != nil {
				return err
			}
		case code.OpSetGlobal:
			idx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2
			vm.globals[idx] = vm.pop()
		case code.OpGetGlobal:
			idx := code.ReadUint16(vm.instructions[ip+1:])
			ip += 2
			err := vm.push(vm.globals[idx])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (vm *VM) executeBinaryOperation(op code.Opcode) error {
	r := vm.pop()
	l := vm.pop()

	if l.Type() == object.INTEGER_OBJ && r.Type() == object.INTEGER_OBJ {
		return vm.executeBinaryIntegerOperation(op, l, r)
	}

	return fmt.Errorf("unsupported types for binary operation: %s %s", l.Type(), r.Type())
}

func (vm *VM) executeBinaryIntegerOperation(op code.Opcode, left, right object.Object) error {
	l := left.(*object.Integer).Value
	r := right.(*object.Integer).Value
	var result int64

	switch op {
	case code.OpAdd:
		result = l + r
	case code.OpSub:
		result = l - r
	case code.OpMul:
		result = l * r
	case code.OpDiv:
		result = l / r
	default:
		return fmt.Errorf("unsupported operator: %d", op)
	}

	return vm.push(&object.Integer{Value: result})
}

func (vm *VM) executeComparisonOperation(op code.Opcode) error {
	r := vm.pop()
	l := vm.pop()

	if l.Type() == object.INTEGER_OBJ && r.Type() == object.INTEGER_OBJ {
		return vm.executeIntComparisonOperation(op, l, r)
	}

	if l.Type() == object.BOOLEAN_OBJ && r.Type() == object.BOOLEAN_OBJ {
		return vm.executeBoolComparisonOperation(op, l, r)
	}

	return fmt.Errorf("unsupported types for comparison operation: %s %s", l.Type(), r.Type())
}

func (vm *VM) executeIntComparisonOperation(op code.Opcode, left, right object.Object) error {
	l := left.(*object.Integer).Value
	r := right.(*object.Integer).Value
	var result object.Object

	switch op {
	case code.OpEqual:
		result = nativeBoolToBooleanObject(l == r)
	case code.OpNotEqual:
		result = nativeBoolToBooleanObject(l != r)
	case code.OpGreaterThan:
		result = nativeBoolToBooleanObject(l > r)

	default:
		return fmt.Errorf("unsupported operator: %d", op)
	}

	return vm.push(result)
}

func (vm *VM) executeBoolComparisonOperation(op code.Opcode, l, r object.Object) error {
	var result object.Object

	switch op {
	case code.OpEqual:
		result = nativeBoolToBooleanObject(l == r)
	case code.OpNotEqual:
		result = nativeBoolToBooleanObject(l != r)

	default:
		return fmt.Errorf("unsupported operator: %d", op)
	}

	return vm.push(result)
}

func nativeBoolToBooleanObject(b bool) *object.Boolean {
	if b {
		return True
	}
	return False
}

func isTruthy(o object.Object) bool {
	return o != False && o != Null
}

func (vm *VM) push(o object.Object) error {
	if vm.sp >= StackSize {
		return fmt.Errorf("stack overflow")
	}

	vm.stack[vm.sp] = o
	vm.sp++

	return nil
}

func (vm *VM) pop() object.Object {
	o := vm.stack[vm.sp-1]
	vm.sp--
	return o
}
