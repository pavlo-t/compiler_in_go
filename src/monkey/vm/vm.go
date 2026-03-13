package vm

import (
	"fmt"
	"monkey/code"
	"monkey/compiler"
	"monkey/object"
)

const StackSize = 2048
const GlobalsSize = 1 << 16
const MaxFrames = 1024

var True = &object.Boolean{Value: true}
var False = &object.Boolean{Value: false}
var Null = &object.Null{}

type VM struct {
	constants   []object.Object
	globals     []object.Object
	stack       []object.Object
	sp          int // Always points to the next value. Top of stack is stack[sp-1]
	frames      []*Frame
	framesIndex int
}

func New(bytecode *compiler.Bytecode) *VM {
	vm := &VM{
		constants:   bytecode.Constants,
		globals:     make([]object.Object, GlobalsSize),
		stack:       make([]object.Object, StackSize),
		sp:          0,
		frames:      make([]*Frame, MaxFrames),
		framesIndex: 0,
	}
	vm.pushFrame(NewFrame(&object.CompiledFunction{Instructions: bytecode.Instructions}, 0))
	return vm
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
	var ip int
	var ins code.Instructions
	var op code.Opcode

	for vm.currentFrame().ip < len(vm.currentFrame().Instructions())-1 {
		vm.currentFrame().ip++

		ip = vm.currentFrame().ip
		ins = vm.currentFrame().Instructions()
		op = code.Opcode(ins[ip])

		switch op {
		case code.OpConstant:
			constIndex := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
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
				vm.currentFrame().ip += 2
			} else {
				vm.currentFrame().ip = int(code.ReadUint16(ins[ip+1:])) - 1
			}

		case code.OpJump:
			vm.currentFrame().ip = int(code.ReadUint16(ins[ip+1:])) - 1

		case code.OpNull:
			err := vm.push(Null)
			if err != nil {
				return err
			}

		case code.OpSetGlobal:
			idx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			vm.globals[idx] = vm.pop()

		case code.OpGetGlobal:
			idx := code.ReadUint16(ins[ip+1:])
			vm.currentFrame().ip += 2
			err := vm.push(vm.globals[idx])
			if err != nil {
				return err
			}

		case code.OpArray:
			size := int(code.ReadUint16(ins[ip+1:]))
			vm.currentFrame().ip += 2
			elements := make([]object.Object, size)
			for i := size - 1; i >= 0; i-- {
				elements[i] = vm.pop()
			}
			err := vm.push(&object.Array{Elements: elements})
			if err != nil {
				return err
			}

		case code.OpHash:
			size := int(code.ReadUint16(ins[ip+1:]))
			vm.currentFrame().ip += 2
			pairs := make(map[object.HashKey]object.HashPair, size/2)
			for i := 0; i < size; i += 2 {
				value := vm.pop()
				key := vm.pop()
				hashable, ok := key.(object.Hashable)
				if !ok {
					return fmt.Errorf("unusable as hash key: %s", key.Type())
				}
				pairs[hashable.HashKey()] = object.HashPair{Key: key, Value: value}
			}
			err := vm.push(&object.Hash{Pairs: pairs})
			if err != nil {
				return err
			}

		case code.OpIndex:
			err := vm.executeIndexOperation()
			if err != nil {
				return err
			}

		case code.OpCall:
			numArgs := int(code.ReadUint8(ins[ip+1:]))
			vm.currentFrame().ip += 1
			obj := vm.stack[vm.sp-1-numArgs]

			switch fn := obj.(type) {
			case *object.CompiledFunction:
				if numArgs != fn.NumParameters {
					return fmt.Errorf("wrong number of arguments: want=%d, got=%d", fn.NumParameters, numArgs)
				}
				vm.pushFrame(NewFrame(fn, vm.sp-numArgs))
				vm.sp = vm.currentFrame().basePointer + fn.NumLocals

			case *object.Builtin:
				result := fn.Fn(vm.stack[vm.sp-numArgs : vm.sp]...)
				if result == nil {
					result = Null
				}
				vm.sp = vm.sp - 1 - numArgs
				err := vm.push(result)
				if err != nil {
					return err
				}

			default:
				return fmt.Errorf("call on non-function: %T", obj)
			}

		case code.OpReturnValue:
			returnValue := vm.pop()
			vm.sp = vm.popFrame().basePointer - 1
			err := vm.push(returnValue)
			if err != nil {
				return err
			}

		case code.OpReturn:
			vm.sp = vm.popFrame().basePointer - 1
			err := vm.push(Null)
			if err != nil {
				return err
			}

		case code.OpSetLocal:
			idx := int(code.ReadUint8(ins[ip+1:]))
			vm.currentFrame().ip += 1
			vm.stack[vm.currentFrame().basePointer+idx] = vm.pop()

		case code.OpGetLocal:
			idx := int(code.ReadUint8(ins[ip+1:]))
			vm.currentFrame().ip += 1
			err := vm.push(vm.stack[vm.currentFrame().basePointer+idx])
			if err != nil {
				return err
			}

		case code.OpGetBuiltin:
			idx := int(code.ReadUint8(ins[ip+1:]))
			vm.currentFrame().ip += 1
			err := vm.push(object.Builtins[idx].Builtin)
			if err != nil {
				return err
			}

		default:
			definition, err := code.Lookup(ins[ip])
			if err != nil {
				return err
			}
			return fmt.Errorf("unsupported code: %+v", definition)
		}
	}

	return nil
}

func (vm *VM) executeIndexOperation() error {
	key := vm.pop()
	obj := vm.pop()

	switch {
	case obj.Type() == object.ARRAY_OBJ && key.Type() == object.INTEGER_OBJ:
		elements := obj.(*object.Array).Elements
		idx := int(key.(*object.Integer).Value)
		if idx >= 0 && idx < len(elements) {
			return vm.push(elements[idx])
		}
		return vm.push(Null)

	case obj.Type() == object.HASH_OBJ:
		hashable, ok := key.(object.Hashable)
		if !ok {
			return fmt.Errorf("index operator not supported: %s[%s]", obj.Type(), key.Type())
		}
		pair, exists := obj.(*object.Hash).Pairs[hashable.HashKey()]
		if exists {
			return vm.push(pair.Value)
		}
		return vm.push(Null)

	default:
		return fmt.Errorf("index operator not supported: %s[%s]", obj.Type(), key.Type())
	}
}

func (vm *VM) executeBinaryOperation(op code.Opcode) error {
	r := vm.pop()
	l := vm.pop()

	if l.Type() == object.INTEGER_OBJ && r.Type() == object.INTEGER_OBJ {
		return vm.executeBinaryIntegerOperation(op, l, r)
	}

	if l.Type() == object.STRING_OBJ && r.Type() == object.STRING_OBJ {
		return vm.executeBinaryStringOperation(op, l, r)
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

func (vm *VM) executeBinaryStringOperation(op code.Opcode, left, right object.Object) error {
	l := left.(*object.String).Value
	r := right.(*object.String).Value
	var result string

	switch op {
	case code.OpAdd:
		result = l + r
	default:
		return fmt.Errorf("unsupported string operator: %d", op)
	}

	return vm.push(&object.String{Value: result})
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

func (vm *VM) currentFrame() *Frame {
	return vm.frames[vm.framesIndex-1]
}

func (vm *VM) pushFrame(f *Frame) {
	vm.frames[vm.framesIndex] = f
	vm.framesIndex++
}

func (vm *VM) popFrame() *Frame {
	vm.framesIndex--
	return vm.frames[vm.framesIndex]
}
