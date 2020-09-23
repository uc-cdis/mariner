package mariner

import (
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for generating commands for CommandLineTools

/*
Notes on generating commands for CLTs
- baseCommand contains leading arguments
- inputs and arguments mix together and are ordered via position specified in each binding
- the rules for sorting when no position is specified are truly ambiguous, so
---- presently only supporting sorting inputs/arguments via position and no other key
---- later can implement sorting based on additional keys, but not in first iteration here

Sketch of Steps:
0. cmdElts := make([]CommandElement, 0)
1. Per argument, construct CommandElement -> cmdElts = append(cmdElts, cmdElt)
2. Per input, construct CommandElement -> cmdElts = append(cmdElts, cmdElt)
3. Sort(cmdElts) -> using Position field values (or whatever sorting key we want to use later on)
4. Use baseCommand as initial arguments for command -> cmd := BaseCommand
5. Iterate through sorted cmdElts and append each to command -> cmd = append(cmd, cmdElt.Value...)
6. return cmd
*/

// CommandElement represents an input/argument on the commandline for commandlinetools
type CommandElement struct {
	Position    int      // position from binding
	ArgPosition int      // index from arguments list, if argument
	Value       []string // representation of this input/arg on the commandline (after any/all valueFrom, eval, prefix, separators, shellQuote, etc. has been resolved)
}

// CommandElements is an array of CommandElements
// we define this type and methods for sort.Interface so these CommandElements can be sorted by position
type CommandElements []*CommandElement

// from first example at: https://golang.org/pkg/sort/
func (cmdElts CommandElements) Len() int           { return len(cmdElts) }
func (cmdElts CommandElements) Swap(i, j int)      { cmdElts[i], cmdElts[j] = cmdElts[j], cmdElts[i] }
func (cmdElts CommandElements) Less(i, j int) bool { return cmdElts[i].Position < cmdElts[j].Position }

// GenerateCommand ..
func (tool *Tool) generateCommand() (err error) {
	tool.Task.infof("begin generate command")
	cmdElts, err := tool.cmdElts()
	if err != nil {
		return tool.Task.errorf("%v", err)
	}

	// Sort the command elements by position
	sort.Sort(cmdElts)

	// capture stdout if specified - tool.Task.Root.Stdout resolves to a file name where stdout will be redirected
	if tool.Task.Root.Stdout != "" {
		// append "1> stdout_file" to end of command
		stdoutElts, err := tool.stdElts(1)
		if err != nil {
			return tool.Task.errorf("%v", err)
		}
		cmdElts = append(cmdElts, stdoutElts...)
	}

	// capture stderr if specified - tool.Task.Root.Stderr resolves to a file name where stderr will be redirected
	if tool.Task.Root.Stderr != "" {
		// append "2> stderr_file" to end of command
		stderrElts, err := tool.stdElts(2)
		if err != nil {
			return tool.Task.errorf("%v", err)
		}
		cmdElts = append(cmdElts, stderrElts...)
	}

	cmd := tool.Task.Root.BaseCommands // BaseCommands is []string - empty array if no BaseCommand specified
	for _, cmdElt := range cmdElts {
		cmd = append(cmd, cmdElt.Value...)
	}
	tool.Command = exec.Command(cmd[0], cmd[1:]...)
	tool.Task.infof("end generate command")
	return nil
}

// i==1 --> stdout; i==2 --> stderr
func (tool *Tool) stdElts(i int) (cmdElts CommandElements, err error) {
	tool.Task.infof("begin handle stdout and stderr destinations")
	cmdElts = make([]*CommandElement, 0)
	var f string
	switch i {
	case 1:
		f, _, err = tool.resolveExpressions(tool.Task.Root.Stdout)
	case 2:
		f, _, err = tool.resolveExpressions(tool.Task.Root.Stderr)
	}
	if err != nil {
		return nil, tool.Task.errorf("%v", err)
	}

	prefix := tool.WorkingDir

	// 2>> for stderr, 1>> for stdout
	// NOTE: presently using ">>" (append) and not ">" (write) in case multiple steps/tools redirect stdout/stderr to the same file
	// ----- need to decide implementation for scattered tasks
	// ----- if all scattered tasks redirect their output to the same file if a fixed filename specified
	// ----- or if each scattered subtask will have its own individual dir and stdout/stderr file
	stream := fmt.Sprintf("%v>>", i)

	cmdElt := &CommandElement{
		Value: []string{stream, prefix + f},
	}
	cmdElts = append(cmdElts, cmdElt)
	tool.Task.infof("end handle stdout and stderr destinations")
	return cmdElts, nil
}

func (tool *Tool) cmdElts() (cmdElts CommandElements, err error) {
	tool.Task.infof("begin process command elements")
	cmdElts = make([]*CommandElement, 0)

	// handle arguments
	argElts, err := tool.argElts()
	if err != nil {
		return nil, tool.Task.errorf("%v", err)
	}
	cmdElts = append(cmdElts, argElts...)

	// handle inputs
	inputElts, err := tool.inputElts()
	if err != nil {
		return nil, tool.Task.errorf("%v", err)
	}
	cmdElts = append(cmdElts, inputElts...)

	tool.Task.infof("end process command elements")
	return cmdElts, nil
}

// fixme: handle optional inputs
// UPDATE: optional inputs which are not provided should input.Binding as nil
func (tool *Tool) inputElts() (cmdElts CommandElements, err error) {
	tool.Task.infof("begin handle command input elements")

	// debug
	fmt.Printf("begin handle command input elements")

	cmdElts = make([]*CommandElement, 0)
	var inputType string
	for _, input := range tool.Task.Root.Inputs {
		// no binding -> input doesn't get processed for representation on the commandline (though this input may be referenced by an argument)
		fmt.Printf("-- handling input: %v --\n", input.ID)
		if input.Binding != nil {
			printJSON(input)
			pos := input.Binding.Position // default position is 0, as per CWL spec
			// get non-null input type - should encapsulate this to a function
			for _, _type := range input.Types {
				if _type.Type != "null" {
					inputType = _type.Type
					break
				}
			}
			val, err := inputValue(input, input.Provided.Raw, inputType, input.Binding)
			if err != nil {
				return nil, tool.Task.errorf("%v", err)
			}
			fmt.Printf(" - input value: %v -\n", val)
			cmdElt := &CommandElement{
				Position: pos,
				Value:    val,
			}
			cmdElts = append(cmdElts, cmdElt)
		}
	}
	tool.Task.infof("end handle command input elements")
	return cmdElts, nil
}

// NOTE: inputs of type 'object' presently not supported
func inputValue(input *cwl.Input, rawInput interface{}, inputType string, binding *cwl.Binding) (val []string, err error) {
	// binding is non-nil
	// input sources:
	// 1. if valueFrom specified in binding, then input value taken from there
	// 2. else input value taken from input object
	// regardless of input source, the input value to work with for the binding is stored in input.Provided.Raw
	// need a type switch to cover all the possible cases
	// recall a few different binding rules apply for different input types
	// see: https://www.commonwl.org/v1.0/CommandLineTool.html#CommandLineBinding

	/*
		Steps:
		1. identify type
		2. retrieve value based on type -> convert to string based on type -> collect in val
		3. if prefix specified -> handle prefix based on type (also handle `separate` if specified) -> collect in val
		4. if array and separator specified - handle separator -> collect in val

		5. handle shellQuote -> need to test (feature not yet supported on array inputs)
	*/

	var s string
	switch inputType {
	case "object": // TODO - presently bindings on 'object' inputs not supported - have yet to find an example to work with
		// "Add prefix only, and recursively add object fields for which inputBinding is specified."
		return nil, fmt.Errorf("inputs of type 'object' not supported. input: %v", rawInput)

	case "array": // NOTE: presently not supporting shellQuote feature for bindings on array inputs because need to find an example to work with
		// add prefix if specified
		if binding.Prefix != "" {
			val = append(val, binding.Prefix)
		}
		if binding.Separator != "NOT SPECIFIED" {
			// get string repr of array with specified separator
			s, err = joinArray(input) // ex: [a, b, c] & sep=="," -> "a,b,c"
			if err != nil {
				return nil, err
			}

			// add the string repr of the array
			val = append(val, s)

			// join with no space if separate==false
			if !binding.Separate {
				val = []string{strings.Join(val, "")}
			}

			return val, nil
		}
		///// no itemSeparator case: handle/process elements of the array individually --> /////
		var itemType string
		// retrieve first non-null item type - presently not supporting multiple different types in one input array
		for _, item := range input.Types[0].Items {
			if item.Type != "null" {
				itemType = item.Type
				break
			}
		}
		itemToString, err := itemHandler(itemType)
		if err != nil {
			return nil, err
		}
		inputArray := reflect.ValueOf(input.Provided.Raw)
		for i := 0; i < inputArray.Len(); i++ {
			if input.Types[0].Items[0].Binding != nil {
				// need to handle this case of binding specified to be applied to each element individually
				itemVal, err := inputValue(nil, inputArray.Index(i).Interface(), itemType, input.Types[0].Items[0].Binding)
				if err != nil {
					return nil, err
				}
				val = append(val, itemVal...)
			} else {
				sItem, err := itemToString(inputArray.Index(i).Interface())
				if err != nil {
					return nil, err
				}
				val = append(val, sItem)

				if i == 0 && !binding.Separate {
					// if 'separate' specified as false -> join prefix and first element
					val = []string{strings.Join(val, "")}
				}
			}
		}
		return val, nil
		////// <-- end array no itemSeparator case //////

	case "null":
		// "Add nothing."
		return val, nil
	case "boolean":
		if binding.Prefix == "" {
			return nil, fmt.Errorf("boolean input provided but no prefix provided")
		}
		boolVal, err := boolFromRaw(rawInput)
		if err != nil {
			return nil, err
		}
		// "if true, add 'prefix' to the commandline. If false, add nothing."
		if boolVal {
			// need to test shellQuote feature
			if binding.ShellQuote {
				val = append(val, "\""+binding.Prefix+"\"")
			} else {
				val = append(val, binding.Prefix)
			}
		}
		return val, nil

	case "string", "number", "int", "long", "float", "double":
		s, err = valFromRaw(rawInput)
	case CWLFileType, CWLDirectoryType:
		s, err = pathFromRaw(rawInput)
	}
	// string/number and file/directory share the same processing here
	// other cases end with return statements -> maybe refactor/revise this
	if err != nil {
		return nil, err
	}
	if binding.Prefix != "" {
		val = append(val, binding.Prefix)
	}
	val = append(val, s)
	if !binding.Separate {
		val = []string{strings.Join(val, "")}
	}
	// need to test ShellQuote feature
	if binding.ShellQuote {
		val = []string{"\"" + strings.Join(val, " ") + "\""}
	}
	return val, nil
}

// handles case where 'separator' field is specified
// returns string which is joined input array with the given separator
func joinArray(input *cwl.Input) (arr string, err error) {
	var itemType, sItem string
	for _, item := range input.Types {
		if item.Type != "null" {
			// retrieve first non-null type listed
			// NOTE: not immediately sure how to handle multiple different datatypes in a single array - can address this issue later
			itemType = item.Type
			break
		}
	}
	itemToString, err := itemHandler(itemType)
	if err != nil {
		return "", err
	}
	resArray := []string{}
	inputArray := reflect.ValueOf(input.Provided.Raw)
	for i := 0; i < inputArray.Len(); i++ {
		// get string form of item
		sItem, err = itemToString(inputArray.Index(i).Interface())
		if err != nil {
			return "", err
		}
		resArray = append(resArray, sItem)
	}
	arr = strings.Join(resArray, input.Binding.Separator)
	return arr, nil
}

// called in joinArray() to get appropriate function to convert the array element to a string
func itemHandler(itemType string) (handler func(interface{}) (string, error), err error) {
	switch itemType {
	case "string", "number":
		return valFromRaw, nil
	case CWLFileType, CWLDirectoryType:
		return pathFromRaw, nil
	default:
		return nil, fmt.Errorf("binding on input array with items of type %T not supported", itemType)
	}
}

// called in getInputValue()
func boolFromRaw(rawInput interface{}) (boolVal bool, err error) {
	boolVal, ok := rawInput.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected data type for input specified as bool: %v; %T", rawInput, rawInput)
	}
	return boolVal, nil
}

// called in getInputValue()
func pathFromRaw(rawInput interface{}) (path string, err error) {
	switch rawInput.(type) {
	case string:
		path = rawInput.(string)
	case *File:
		fileObj := rawInput.(*File)
		path = fileObj.Path
	default:
		path, err = filePath(rawInput)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve file or directory path from object of type %T with value %v", rawInput, rawInput)
		}
	}
	return path, nil
}

// called in getInputValue()
func valFromRaw(rawInput interface{}) (val string, err error) {
	switch rawInput.(type) {
	case string:
		val = rawInput.(string)
	case int:
		val = strconv.Itoa(rawInput.(int))
	case float64:
		val = strconv.FormatFloat(rawInput.(float64), 'f', -1, 64)
	default:
		return "", fmt.Errorf("unexpected data type for input specified as number or string: %v; %T", rawInput, rawInput)
	}
	return val, nil
}

// collect CommandElement objects from arguments
func (tool *Tool) argElts() (cmdElts CommandElements, err error) {
	tool.Task.infof("begin handle command argument elements")
	cmdElts = make([]*CommandElement, 0) // this might be redundant - basic q: do I need to instantiate this array if it's a named output?
	for i, arg := range tool.Task.Root.Arguments {
		pos := 0 // if no position specified the default is zero, as per CWL spec
		if arg.Binding != nil {
			pos = arg.Binding.Position
		}
		val, err := tool.argVal(arg) // okay
		if err != nil {
			return nil, tool.Task.errorf("%v", err)
		}
		cmdElt := &CommandElement{
			Position:    pos,
			ArgPosition: i + 1, // beginning at 1 so that can detect nil/zero value of 0
			Value:       val,
		}
		cmdElts = append(cmdElts, cmdElt)
	}
	tool.Task.infof("end handle command argument elements")
	return cmdElts, nil
}

// gets value from an argument - i.e., returns []string containing strings which will be put on the commandline to represent this argument
func (tool *Tool) argVal(arg cwl.Argument) (val []string, err error) {
	tool.Task.infof("begin get value from command element argument")
	// cases:
	// either a string literal or an expression
	// OR a binding with valueFrom field specified
	val = make([]string, 0)
	if arg.Value != "" {
		// implies string literal or expression to eval - see NOTE at typeSwitch
		// fmt.Println("string literal or expression to eval..")
		// NOTE: *might* need to check "$(" or "${" instead of just "$"
		if strings.HasPrefix(arg.Value, "$") {
			// expression to eval - here `self` is null - no additional context to load - just need to eval in inputsVM
			// fmt.Println("expression to eval..")
			result, err := evalExpression(arg.Value, tool.Task.Root.InputsVM)
			if err != nil {
				return nil, tool.Task.errorf("failed to evaluate expression: %v; err: %v", arg.Value, err)
			}
			// NOTE: what type can I expect the result to be here? - hopefully string or []string - need to test and find additional examples to work with
			switch result.(type) {
			case string:
				val = append(val, result.(string))
			case []string:
				val = append(val, result.([]string)...)
			default:
				return nil, tool.Task.errorf("unexpected type returned by argument expression: %v; %v; %T", arg.Value, result, result)
			}
		} else {
			// string literal - no processing to be done
			val = append(val, arg.Value)
		}
	} else {
		// fmt.Println("resolving valueFrom..")
		// get value from `valueFrom` field which may itself be a string literal, an expression, or a string which contains one or more expressions
		resolvedText, _, err := tool.resolveExpressions(arg.Binding.ValueFrom.String)
		if err != nil {
			return nil, tool.Task.errorf("%v", err)
		}

		// handle shellQuote - default value is true
		if arg.Binding.ShellQuote {
			resolvedText = "\"" + resolvedText + "\""
		}

		// capture result
		val = append(val, resolvedText)
	}
	tool.Task.infof("end get value from command element argument")
	return val, nil
}
