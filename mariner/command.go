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
4. Iterate through sorted cmdElts -> cmd = append(cmd, cmdElt.Value...)
5. cmd = append(baseCmd, cmd...)
6. return cmd
*/

// CommandElement represents an input/argument on the commandline for commandlinetools
type CommandElement struct {
	Position    int      // position from binding
	ArgPosition int      // index from arguments list, if argument
	Value       []string // representation of this input/arg on the commandline (after any/all valueFrom, eval, prefix, separators, shellQuote, etc. has been resolved)
}

// define this type and methods for sort.Interface so these CommandElements can be sorted by position
type CommandElements []*CommandElement

// from first example at: https://golang.org/pkg/sort/
func (cmdElts CommandElements) Len() int           { return len(cmdElts) }
func (cmdElts CommandElements) Swap(i, j int)      { cmdElts[i], cmdElts[j] = cmdElts[j], cmdElts[i] }
func (cmdElts CommandElements) Less(i, j int) bool { return cmdElts[i].Position < cmdElts[j].Position }

// GenerateCommand ..
func (tool *Tool) GenerateCommand() (err error) {
	cmdElts, err := tool.getCmdElts() // 1. get arguments (okay) 2. get inputs from bindings - TODO
	if err != nil {
		return err
	}

	// 3. Sort the command elements by position (okay)
	sort.Sort(cmdElts)

	// 3.1. capture stdout if specified - tool.Root.Stdout resolves to a file name where stdout will be redirected
	if tool.Root.Stdout != "" {
		// append "1> stdout_file" to end of command
		stdoutElts, err := tool.getStdElts(1)
		if err != nil {
			return err
		}
		cmdElts = append(cmdElts, stdoutElts...)
	}

	// 3.2. capture stderr if specified - tool.Root.Stderr resolves to a file name where stderr will be redirected
	if tool.Root.Stderr != "" {
		// append "2> stderr_file" to end of command
		stderrElts, err := tool.getStdElts(2)
		if err != nil {
			return err
		}
		cmdElts = append(cmdElts, stderrElts...)
	}

	fmt.Println("here are cmdElts:")
	PrintJSON(cmdElts)

	/*
		4. Iterate through sorted cmdElts -> cmd = append(cmd, cmdElt.Args...) (okay)
		5. cmd = append(baseCmd, cmd...) (okay)
		6. return cmd (okay)
	*/
	cmd := tool.Root.BaseCommands // BaseCommands is []string - zero length if no BaseCommand specified
	for _, cmdElt := range cmdElts {
		cmd = append(cmd, cmdElt.Value...)
	}
	tool.Command = exec.Command(cmd[0], cmd[1:]...)
	return nil
}

// i==1 --> stdout; i==2 --> stderr
// TODO - handle prefix issue in order to write file to this step's dir in path/to/mount_point/workflow/step_dir
func (tool *Tool) getStdElts(i int) (cmdElts CommandElements, err error) {
	cmdElts = make([]*CommandElement, 0)
	var f string
	switch i {
	case 1:
		f, err = tool.resolveExpressions(tool.Root.Stdout)
	case 2:
		f, err = tool.resolveExpressions(tool.Root.Stderr)
	}
	if err != nil {
		return nil, err
	}
	// prefix := "path/to/mount_point/workflow/step_dir"
	prefix := ""

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
	return cmdElts, nil
}

func (tool *Tool) getCmdElts() (cmdElts CommandElements, err error) {
	// 0.
	cmdElts = make([]*CommandElement, 0)

	// 1. handle arguments
	argElts, err := tool.getArgElts() // good - need to test
	if err != nil {
		return nil, err
	}
	cmdElts = append(cmdElts, argElts...)

	// 2. handle inputs
	inputElts, err := tool.getInputElts() // good - need to test
	if err != nil {
		return nil, err
	}
	cmdElts = append(cmdElts, inputElts...)

	return cmdElts, nil
}

// TODO: handle optional inputs
func (tool *Tool) getInputElts() (cmdElts CommandElements, err error) {
	cmdElts = make([]*CommandElement, 0)
	var inputType string
	for _, input := range tool.Root.Inputs {
		// no binding -> input doesn't get processed for representation on the commandline (though this input may be referenced by an argument)
		if input.Binding != nil {
			pos := input.Binding.Position // default position is 0, as per CWL spec
			// get non-null input type - should encapsulate this to a function
			for _, _type := range input.Types {
				if _type.Type != "null" {
					inputType = _type.Type
					break
				}
			}
			val, err := getInputValue(input, input.Provided.Raw, inputType, input.Binding) // TODO - return []string which is the resolved binding (representation on commandline) for this input
			if err != nil {
				return nil, err
			}
			cmdElt := &CommandElement{
				Position: pos,
				Value:    val,
			}
			cmdElts = append(cmdElts, cmdElt)
		}
	}
	return cmdElts, nil
}

// okay - inputs of type 'object' presently not supported
func getInputValue(input *cwl.Input, rawInput interface{}, inputType string, binding *cwl.Binding) (val []string, err error) {
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

	case "array": // okay - presently not supporting shellQuote feature for bindings on array inputs because need to find an example to work with
		// add prefix if specified
		if binding.Prefix != "" {
			val = append(val, binding.Prefix)
		}
		if binding.Separator != "NOT SPECIFIED" {
			// get string repr of array with specified separator
			s, err = joinArray(input) // [a, b, c] & sep=="," -> "a,b,c"
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
		itemToString, err := getItemHandler(itemType)
		if err != nil {
			return nil, err
		}
		inputArray := reflect.ValueOf(input.Provided.Raw)
		for i := 0; i < inputArray.Len(); i++ {
			if input.Types[0].Items[0].Binding != nil {
				// need to handle this case of binding specified to be applied to each element individually
				itemVal, err := getInputValue(nil, inputArray.Index(i).Interface(), itemType, input.Types[0].Items[0].Binding)
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

	case "null": // okay
		// "Add nothing."
		return val, nil
	case "boolean": // okay
		if binding.Prefix == "" {
			return nil, fmt.Errorf("boolean input provided but no prefix provided")
		}
		boolVal, err := getBoolFromRaw(rawInput)
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

	case "string", "number": // okay
		s, err = getValFromRaw(rawInput)
	case "File", "Directory": // okay
		s, err = getPathFromRaw(rawInput)
	}
	// string/number and file/directory share the same processing here
	// other cases end with return statements
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
			// not immediately sure how to handle multiple different datatypes in a single array
			// can address this issue later
			itemType = item.Type
			break
		}
	}
	itemToString, err := getItemHandler(itemType)
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
func getItemHandler(itemType string) (handler func(interface{}) (string, error), err error) {
	switch itemType {
	case "string", "number":
		return getValFromRaw, nil
	case "File", "Directory":
		return getPathFromRaw, nil
	default:
		return nil, fmt.Errorf("binding on input array with items of type %T not supported", itemType)
	}
}

// called in getInputValue()
func getBoolFromRaw(rawInput interface{}) (boolVal bool, err error) {
	boolVal, ok := rawInput.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected data type for input specified as bool: %v; %T", rawInput, rawInput)
	}
	return boolVal, nil
}

// called in getInputValue()
func getPathFromRaw(rawInput interface{}) (path string, err error) {
	switch rawInput.(type) {
	case string:
		path = rawInput.(string)
	case *File:
		fileObj := rawInput.(*File)
		path = fileObj.Path
	default:
		path, err = GetPath(rawInput)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve file or directory path from object of type %T with value %v", rawInput, rawInput)
		}
	}
	return path, nil
}

// called in getInputValue()
func getValFromRaw(rawInput interface{}) (val string, err error) {
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
func (tool *Tool) getArgElts() (cmdElts CommandElements, err error) {
	cmdElts = make([]*CommandElement, 0) // this might be redundant - basic q: do I need to instantiate this array if it's a named output?
	for i, arg := range tool.Root.Arguments {
		pos := 0 // if no position specified the default is zero, as per CWL spec
		if arg.Binding != nil {
			pos = arg.Binding.Position
		}
		val, err := tool.getArgValue(arg) // okay
		if err != nil {
			return nil, err
		}
		cmdElt := &CommandElement{
			Position:    pos,
			ArgPosition: i + 1, // beginning at 1 so that can detect nil/zero value of 0
			Value:       val,
		}
		cmdElts = append(cmdElts, cmdElt)
	}
	return cmdElts, nil
}

// gets value from an argument - i.e., returns []string containing strings which will be put on the commandline to represent this argument
func (tool *Tool) getArgValue(arg cwl.Argument) (val []string, err error) {
	// cases:
	// either a string literal or an expression (okay)
	// OR a binding with valueFrom field specified (okay)
	val = make([]string, 0)
	if arg.Value != "" {
		// implies string literal or expression to eval - okay - see NOTE at typeSwitch

		// NOTE: *might* need to check "$(" or "${" instead of just "$"
		if strings.HasPrefix(arg.Value, "$") {
			// expression to eval - here `self` is null - no additional context to load - just need to eval in inputsVM
			result, err := EvalExpression(arg.Value, tool.Root.InputsVM)
			if err != nil {
				return nil, err
			}
			// NOTE: what type can I expect the result to be here? - hopefully string or []string - need to test and find additional examples to work with
			switch result.(type) {
			case string:
				val = append(val, result.(string))
			case []string:
				val = append(val, result.([]string)...)
			default:
				return nil, fmt.Errorf("unexpected type returned by argument expression: %v; %v; %T", arg.Value, result, result)
			}
		} else {
			// string literal - no processing to be done
			val = append(val, arg.Value)
		}
	} else {
		// get value from `valueFrom` field which may itself be a string literal, an expression, or a string which contains one or more expressions
		resolvedText, err := tool.resolveExpressions(arg.Binding.ValueFrom.String)
		if err != nil {
			return nil, err
		}

		// handle shellQuote - default value is true
		if arg.Binding.ShellQuote {
			resolvedText = "\"" + resolvedText + "\""
		}

		// capture result
		val = append(val, resolvedText)
	}
	return val, nil
}
