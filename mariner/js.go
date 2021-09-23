package mariner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/robertkrimen/otto"
)

// this file contains code for evaluating JS expressions encountered in the CWL
// EvalExpression evals a single expression (something like $(...) or ${...})
// resolveExpressions processes a string which may contain several embedded expressions, each wrapped in their own $()/${} wrapper

// evaluateExpression evaluates the expression from the tool in its virtual machine.
func (tool *Tool) evaluateExpression() (err error) {
	tool.Task.infof("begin evaluate expression")
	if err = os.MkdirAll(tool.WorkingDir, os.ModeDir); err != nil {
		return tool.Task.errorf("failed to make ExpressionTool working dir: %v; error: %v", tool.Task.Root.ID, err)
	}
	if err = os.Chdir(tool.WorkingDir); err != nil {
		return tool.Task.errorf("failed to move to ExpressionTool working dir: %v; error: %v", tool.Task.Root.ID, err)
	}
	result, err := evalExpression(tool.Task.Root.Expression, tool.InputsVM)
	if err != nil {
		return tool.Task.errorf("failed to evaluate expression for ExpressionTool: %v; error: %v", tool.Task.Root.ID, err)
	}
	os.Chdir("/")
	var ok bool
	tool.ExpressionResult, ok = result.(map[string]interface{})
	if !ok {
		return tool.Task.errorf("ExpressionTool expression did not return a JSON object: %v", tool.Task.Root.ID)
	}
	cmdPath := tool.WorkingDir + "expression.txt"
	cmd := []string{"touch", cmdPath}
	tool.Command = exec.Command(cmd[0], cmd[1:]...)
	tool.Task.infof("end evaluate expression")
	return nil
}

// NOTE: make uniform either UpperCase, or camelCase for naming functions
// ----- none of these names really need to be exported, since they get called within the `mariner` package

// getJS strips the cwl prefix for an expression
// and tells whether to just eval the expression, or eval the exp as a js function
// this is modified from the cwl.Eval.ToJavaScriptString() method
func js(s string) (js string, fn bool, err error) {
	// if curly braces, then need to eval as a js function
	// see https://www.commonwl.org/v1.0/Workflow.html#Expressions
	fn = strings.HasPrefix(s, "${")
	s = strings.TrimLeft(s, "$(\n")
	s = strings.TrimRight(s, ")\n")
	return s, fn, nil
}

// EvalExpression is an engine for handling in-line js in cwl
// the exp is passed before being stripped of any $(...) or ${...} wrapper
// the vm must be loaded with all necessary context for eval
// EvalExpression handles parameter references and expressions $(...), as well as functions ${...}
func evalExpression(exp string, vm *otto.Otto) (result interface{}, err error) {
	// strip the $() (or if ${} just trim leading $), which appears in the cwl as a wrapper for js expressions
	var output otto.Value
	js, fn, _ := js(exp)
	if js == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if fn {
		// if expression wrapped like ${...}, need to run as a zero arg js function

		// construct js function definition
		fnDef := fmt.Sprintf("function f() %s", js)

		// run this function definition so the function exists in the vm
		vm.Run(fnDef)

		// call this function in the vm
		output, err = vm.Run("f()")
		if err != nil {
			fmt.Printf("\terror running js function: %v\n", err)
			return nil, err
		}
	} else {
		output, err = vm.Run(js)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate js expression: %v", err)
		}
	}
	result, _ = output.Export()
	return result, nil
}

func (tool *Tool) evalExpression(exp string) (result interface{}, err error) {
	tool.Task.infof("begin eval expression: %v", exp)
	val, err := evalExpression(exp, tool.InputsVM)
	if err != nil {
		return nil, tool.Task.errorf("%v", err)
	}
	tool.Task.infof("end eval expression")
	return val, nil
}

// resolveExpressions intakes a string which can be an expression, a string literal, or JS expression like $(...) or ${...} and resolves it. The resolved result is put into a file and the file pointer is returned
func (tool *Tool) resolveExpressions(inText string) (outText string, outFile *File, err error) {
	tool.Task.infof("begin resolve expression: %v", inText)
	// TODO: assert that the full inText is only a single JS expression. and refactor logic in the per-rune parser.
	if inText[0] == '$' && inText[1] == '{' {
		tool.Task.infof("Interpreting as single JS expression: %v", inText)
		result, err := evalExpression(inText, tool.InputsVM)
		if err != nil {
			return "", nil, tool.Task.errorf("%v", err)
		}

		switch result.(type) {
		case string:
			outText = result.(string)
		case File:
			f := result.(File)
			return "", &f, nil
		}

		tool.Task.infof("end resolve expression. resolved text: %v", outText)
		return outText, nil, nil
	}

	r := bufio.NewReader(strings.NewReader(inText))
	var c0, c1, c2 string
	var done bool
	image := make([]string, 0)
	for !done {
		nextRune, _, err := r.ReadRune()
		if err != nil {
			if err == io.EOF {
				done = true
			} else {
				return "", nil, tool.Task.errorf("%v", err)
			}
		}

		c0, c1, c2 = c1, c2, string(nextRune)
		if c1 == "$" && c2 == "(" && c0 != "\\" {
			expression, err := r.ReadString(')')
			if err != nil {
				return "", nil, tool.Task.errorf("%v", err)
			}

			expression = c1 + c2 + expression

			result, err := evalExpression(expression, tool.InputsVM)
			if err != nil {
				return "", outFile, tool.Task.errorf("%v", err)
			}

			switch result.(type) {
			case string:
				val := result.(string)

				image = image[:len(image)-1]

				image = append(image, val)
			case File:
				f := result.(File)
				return "", &f, nil
			}
		} else {
			if !done {
				image = append(image, string(c2))
			}
		}
	}

	outText = strings.Join(image, "")
	tool.Task.infof("end resolve expression. resolved text: %v", outText)
	return outText, nil, nil
}

/*
	explanation for PreProcessContext

	problem: setting variable name in a js vm to a golang struct doesn't work

	suggested solution by otto examples/docs: use otto.ToValue()

	NOTE: otto library does NOT handle case of converting "complex data types" (e.g., structs) to js objects
	see `func (self *_runtime) toValue(value interface{})` in `runtime.go` in otto library
	comment from otto developer in the typeSwitch in the *_runtime.toValue() method:
	"
	case Object, *Object, _object, *_object:
		// Nothing happens.
		// FIXME We should really figure out what can come here.
		// This catch-all is ugly.
	"

	that means we need to preprocess structs (to a map or array of maps (?)) before loading to vm

	real solution: marshal any struct into json, and then unmarshal that json into a map[string]interface{}
	set the variable in the vm to this map
	this works, and is a simple, elegant solution
	way better to do this json marshal/unmarshal than to handle individual cases
	could suggest this to the otto developer to fix his object handling dilemma
*/

// PreProcessContext is a utility function to preprocess any struct/array before loading into js vm (see above note)
// NOTE: using this json marshalling/unmarshalling strategy implies that the struct field names
// ----- get loaded into the js vm as their json representation/alias.
// ----- this means we can use the cwl fields as json aliases for any struct type's fields
// ----- and then using this function to preprocess the struct/array, all the keys/data will get loaded in properly
// ----- which saves us from having to handle special cases
func preProcessContext(in interface{}) (out interface{}, err error) {
	j, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(j, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
