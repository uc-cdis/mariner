package wftool

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// right now pretty much just writing out the CWL spec in Go types

// will this tool just marshal without enforcing/validating the cwl?
// e.g., if scatter, then scattermethod - will we perform that check here?
// or does this tool assume your cwl is error-free
// probably this tool should have some kind of validation function
// this tool should answer, to some degree, the question - "will this cwl run?"
// "will mariner even attempt to run this workflow?"

/*
	NOTICE

	- currently not supporting inputParameter schemas
	in the CommandInput.Type[i] field - expecting a string

*/

// WorkflowJSON is the JSON representation of a CWL workflow
type WorkflowJSON struct {
	Graph WorkflowGraph
}

// WorkflowGraph contains all the CWLObjects of the workflow
type WorkflowGraph []CWLObject

// CWLObject represents a workflow, expressiontool, commandlinetool
type CWLObject interface {
	JSON() ([]byte, error)
}

// ObjectMeta ..
type ObjectMeta struct {
	CoreMeta
	RequirementsAndHints
	Class      string `yaml:"class"`
	CWLVersion string `yaml:"cwlVersion"`
}

// RequirementsAndHints ..
// NOTE: possibly need to make types for all the different requirements
// though I'm not sure that'll be necessary at all
// since the requirements themselves don't get touched/modifed during unmarshalling
type RequirementsAndHints struct {
	Requirements []map[string]interface{} `yaml:"requirements"`
	Hints        []interface{}            `yaml:"hints"` // some schema!
}

// CoreMeta ..
type CoreMeta struct {
	ID    string `yaml:"id" json:"id"`
	Label string `yaml:"label" json:"label"`
	Doc   string `yaml:"doc" json:"doc"`
}

// OrigWorkflow ..
type OrigWorkflow struct {
	ObjectMeta
	Inputs  []InputParameter
	Outputs []WorkflowOutputParameter
	Steps   []WorkflowStep
}

// Workflow ..
type Workflow struct {
	Class        string        `yaml:"class"`
	CWLVersion   string        `yaml:"cwlVersion"`
	Requirements []interface{} `yaml:"requirements"`
	Hints        []interface{} `yaml:"hints"`
	ID           string        `yaml:"id"`
	Label        string        `yaml:"label"`
	Doc          string        `yaml:"doc"`
	Inputs       []struct {
		ID             string `yaml:"id"`
		Label          string `yaml:"label"`
		Doc            string `yaml:"doc"`
		SecondaryFiles []string
		Streamable     bool
		Format         []string
		Type           []string
		InputBinding   struct {
			LoadContents  bool
			Position      int
			Prefix        string
			Separate      bool
			ItemSeparator string
			ValueFrom     string
			ShellQuote    bool
		}
		Default interface{}
	}
	Outputs []struct {
		ID             string `yaml:"id"`
		Label          string `yaml:"label"`
		Doc            string `yaml:"doc"`
		SecondaryFiles []string
		Streamable     bool
		Format         []string
		Type           []string
		OutputBinding  struct {
			Glob         []string
			LoadContents bool
			OutputEval   string
		}
		OutputSource []string
		LinkMerge    string
	}
	Steps []struct {
		ID           string        `yaml:"id"`
		Label        string        `yaml:"label"`
		Doc          string        `yaml:"doc"`
		Requirements []interface{} `yaml:"requirements"`
		Hints        []interface{} `yaml:"hints"`
		In           []struct {
			ID        string
			Source    []string
			LinkMerge string // 'merge_nested' or 'merge_flattened'
			Default   interface{}
			ValueFrom string
		}
		Out []struct {
			ID string
		} // could be []string or []map[string]string
		Run           interface{}
		Scatter       []string
		ScatterMethod string
	}
}

// WorkflowStep ..
type WorkflowStep struct {
	CoreMeta
	RequirementsAndHints
	In            []WorkflowStepInput
	Out           []WorkflowStepOutput // could be []string or []map[string]string
	Run           interface{}
	Scatter       []string
	ScatterMethod string
}

// WorkflowStepInput ..
type WorkflowStepInput struct {
	ID        string
	Source    []string
	LinkMerge string // 'merge_nested' or 'merge_flattened'
	Default   interface{}
	ValueFrom string
}

// WorkflowStepOutput ..
type WorkflowStepOutput struct {
	ID string
}

// InputParameter ..
type InputParameter struct {
	CoreMeta
	FileParameterFields
	InputBinding CommandLineBinding
	Default      interface{}
}

// WorkflowOutputParameter ..
type WorkflowOutputParameter struct {
	CoreMeta
	FileParameterFields
	OutputBinding CommandOutputBinding
	OutputSource  []string
	LinkMerge     string
}

// FileParameterFields ..
type FileParameterFields struct {
	SecondaryFiles []string `json:"secondaryFiles"`
	Streamable     bool
	Format         []string
	Type           []string
	// NOTE: handling 'Type' requires some thought - several possibilities here
	// see: https://www.commonwl.org/v1.0/Workflow.html#InputParameter
}

// CommandOutputBinding ..
type CommandOutputBinding struct {
	Glob         []string
	LoadContents bool
	OutputEval   string
}

// OrigCommandLineTool ..
type OrigCommandLineTool struct {
	ObjectMeta
	Inputs             []CommandInputParameter  `yaml:"inputs"`
	Outputs            []CommandOutputParameter `yaml:"outputs"`
	BaseCommand        []string                 `yaml:"baseCommand"`
	Arguments          []interface{}            `yaml:"arguments"` // an argument is one of 'expression' | 'string' | 'commandlinebinding'
	Stdin              string                   `yaml:"stdin"`
	Stderr             string                   `yaml:"stderr"`
	Stdout             string                   `yaml:"stdout"`
	SuccessCodes       []int                    `yaml:"successCodes"`
	TemporaryFailCodes []int                    `yaml:"temporaryFailCodes"`
	PermanentFailCodes []int                    `yaml:"permanentFailCodes"`
}

/*

special handling required for:
- Requirements
- Hints
- Type
- Default
- Arguments (working on this)

currently handling Arguments
- array of (string | expression | commandlinebinding)
- which translates to an array of (string | commandlinebinding)
- this is a simple thing to handle
*/

/*
other issue: <T> vs. []<T>

this issue:

i: maps
o: arrays

Requirements and Hints (done)
Inputs (done)
Outputs (done)

if


type
default

arguments (should be fine)
*/

// CommandLineTool ..
// the interface fields are trouble
type CommandLineTool struct {
	Class      string `yaml:"class"`
	CWLVersion string `yaml:"cwlVersion"`
	ID         string `yaml:"id"`
	Label      string `yaml:"label"`
	Doc        string `yaml:"doc"`
	// special handling needed for requirements and hints
	Requirements map[string]string `yaml:"requirements"`
	Hints        map[string]string `yaml:"hints"`
	// handle inputs
	Inputs map[string]struct {
		// ID             string `yaml:"id"`
		Label          string `yaml:"label"`
		Doc            string `yaml:"doc"`
		SecondaryFiles []string
		Streamable     bool
		Format         []string
		InputBinding   struct {
			LoadContents  bool
			Position      int
			Prefix        string
			Separate      bool
			ItemSeparator string
			ValueFrom     string
			ShellQuote    bool
		}
		// special handling needed for 'Type' and 'Default'
		Type    []string
		Default interface{}
	} `yaml:"inputs"`
	// should be map[string]struct, per CWL change
	// handle Outputs
	Outputs map[string]struct {
		// ID             string `yaml:"id"`
		Label          string `yaml:"label"`
		Doc            string `yaml:"doc"`
		SecondaryFiles []string
		Streamable     bool
		Format         []string
		Type           []string
		OutputBinding  struct {
			Glob         []string
			LoadContents bool
			OutputEval   string
		}
	} `yaml:"outputs"`
	BaseCommand        []string `yaml:"baseCommand"`
	Stdin              string   `yaml:"stdin"`
	Stderr             string   `yaml:"stderr"`
	Stdout             string   `yaml:"stdout"`
	SuccessCodes       []int    `yaml:"successCodes"`
	TemporaryFailCodes []int    `yaml:"temporaryFailCodes"`
	PermanentFailCodes []int    `yaml:"permanentFailCodes"`
	// handle 'Arguments'
	Arguments []interface{} `yaml:"arguments"` // an argument is one of 'expression' | 'string' | 'commandlinebinding'
}

// original
func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	}
	return i
}

/* in the middle of dev'ing.. not worth time right now to finish this fn
func convertCommandParameter(m map[interface{}]interface{}, keyWord string) []interface{} {
	arr := []interface{}{}
	for k, v := range m {
		var nuV interface{}
		switch keyWord {
		case "inputs":
			nuV = CommandInputParameter{}
		case "outputs":
			nuV = CommandOutputParameter{}
		}
		switch v.(type) {
		case map[interface{}]interface{}:
			err := mapstructure.Decode(v, &nuV)
			if err != nil {
				fmt.Println("failed to convert map to struct: ", err)
			}
			// nuV.ID = k.(string)
			reflect.ValueOf(&nuV).Elem().FieldByName("ID").SetString(k.(string))
		case []string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, len(v.([]string)))
			for _, s := range v.([]string) {
				nuV.FileParameterFields.Type = append(nuV.FileParameterFields.Type, s)
			}
		case string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, 1)
			nuV.FileParameterFields.Type[0] = v.(string)
		}
		arr = append(arr, nuV)
	}
	return arr
}
*/

func convertCommandInputs(m map[interface{}]interface{}) []CommandInputParameter {
	arr := []CommandInputParameter{}
	for k, v := range m {
		nuV := CommandInputParameter{}
		switch v.(type) {
		case map[interface{}]interface{}:
			err := mapstructure.Decode(v, &nuV)
			if err != nil {
				fmt.Println("failed to convert map to struct: ", err)
			}
			nuV.ID = k.(string)
		case []string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, len(v.([]string)))
			for _, s := range v.([]string) {
				nuV.FileParameterFields.Type = append(nuV.FileParameterFields.Type, s)
			}
		case string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, 1)
			nuV.FileParameterFields.Type[0] = v.(string)
		}
		arr = append(arr, nuV)
	}
	return arr
}

// isomorphic to the convertCommandInputs - refactor
func convertCommandOutputs(m map[interface{}]interface{}) []CommandOutputParameter {
	arr := []CommandOutputParameter{}
	for k, v := range m {
		nuV := CommandOutputParameter{}
		switch v.(type) {
		case map[interface{}]interface{}:
			err := mapstructure.Decode(v, &nuV)
			if err != nil {
				fmt.Println("failed to convert map to struct: ", err)
			}
			nuV.ID = k.(string)
		case []string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, len(v.([]string)))
			for _, s := range v.([]string) {
				nuV.FileParameterFields.Type = append(nuV.FileParameterFields.Type, s)
			}
		case string:
			nuV.ID = k.(string)
			nuV.FileParameterFields.Type = make([]string, 1)
			nuV.FileParameterFields.Type[0] = v.(string)
		}
		arr = append(arr, nuV)
	}
	return arr
}

/*
	certain fields need to be converted from map to array structure
	currently, the way the cwl.go library is setup
	could make changes there
	but first just getting things working
*/
var mapToArray = map[string]bool{
	"inputs":       true,
	"outputs":      true,
	"requirements": true,
	"hints":        true,
}

func array(m map[interface{}]interface{}, parentKey string) []map[string]interface{} {
	arr := []map[string]interface{}{}
	var nuV map[string]interface{}
	for k, v := range m {
		// i := convert(v) // this works
		i := nuConvert(v, k.(string)) // this should work
		switch x := i.(type) {
		case map[string]interface{}:
			nuV = x
		case string: // if inputs, where you have id: type
			nuV = make(map[string]interface{})
			if parentKey == "inputs" {
				nuV["type"] = x
			} else {
				panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
			}
		default:
			panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
		}
		switch parentKey {
		case "requirements", "hints":
			nuV["class"] = k.(string)
		default:
			nuV["id"] = k.(string)
		}
		arr = append(arr, nuV)
	}
	return arr
}

// map to list
func convertRequirements(m map[interface{}]interface{}) []map[string]interface{} {
	arr := []map[string]interface{}{}
	for k, v := range m {
		nuV := convert(v).(map[string]interface{})
		nuV["class"] = k.(string)
		arr = append(arr, nuV)
	}
	return arr
}

/*
HERE - TODO - finish this fn - does all the work

1. commandlinetool (okay)
2. workflow
3. expressiontool

nuConvert's gotta know which .. - this seems like a wrong train of thought
pause, reconsider
*/
func nuConvert(i interface{}, parentKey string) interface{} {
	fmt.Println("handling field: ", parentKey)
	switch x := i.(type) {
	case map[interface{}]interface{}:
		/*
			switch parentKey {
			case "inputs":
				return convertCommandInputs(x)
			case "outputs":
				return convertCommandOutputs(x)
			case "requirements", "hints":
				return convertRequirements(x)
			}
		*/

		if mapToArray[parentKey] {
			return array(x, parentKey)
		}

		m2 := map[string]interface{}{}
		for k, v := range x {
			key := k.(string)
			m2[key] = nuConvert(v, key)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = nuConvert(v, "")
		}
	}
	return i
}

// UnmarshalYAML ..
func (clb *CommandLineBinding) UnmarshalYAML(unmarshal func(interface{}) error) error {
	yamlStruct := make(map[string]interface{})
	err := unmarshal(&yamlStruct)
	if err != nil {
		return err
	}
	for k, v := range yamlStruct {
		switch k {
		case "loadContents":
			clb.LoadContents = v.(bool)
		case "position":
			clb.Position = v.(int)
		case "prefix":
			clb.Prefix = v.(string)
		case "separate":
			clb.Separate = v.(bool)
		case "itemSeparator":
			clb.ItemSeparator = v.(string)
		case "valueFrom":
			clb.ValueFrom = v.(string)
		case "shellQuote":
			clb.ShellQuote = v.(bool)
		}
	}
	return nil
}

// CommandLineBinding ..
type CommandLineBinding struct {
	LoadContents  bool
	Position      int
	Prefix        string
	Separate      bool
	ItemSeparator string
	ValueFrom     string
	ShellQuote    bool
}

// CommandInputParameter ..
type CommandInputParameter struct {
	CoreMeta
	FileParameterFields
	InputBinding CommandLineBinding
	Default      interface{}
}

// CommandOutputParameter ..
type CommandOutputParameter struct {
	CoreMeta
	FileParameterFields
	OutputBinding CommandOutputBinding
}

// ExpressionTool ..
type ExpressionTool struct {
	ObjectMeta
	Inputs     []InputParameter
	Outputs    []ExpressionToolOutputParameter
	Expression string
}

// ExpressionToolOutputParameter ..
type ExpressionToolOutputParameter struct {
	CoreMeta
	FileParameterFields
	OutputBinding CommandOutputBinding
}
