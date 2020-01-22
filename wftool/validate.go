package main

import "fmt"

// WorkflowJSON ..
type WorkflowJSON struct {
	Graph      *[]map[string]interface{} `json:"$graph"`
	CWLVersion string                    `json:"cwlVersion"`
}

// Grievances ..
type Grievances []string

// WorkflowGrievances ..
type WorkflowGrievances struct {
	Main      Grievances            `json:"main"`
	ByProcess map[string]Grievances `json:"byProcess"`
}

// Validator ..
type Validator struct {
	Workflow   *WorkflowJSON
	Grievances *WorkflowGrievances
}

func (g Grievances) log(f string, vs ...interface{}) {
	g = append(g, fmt.Sprintf(f, vs...))
}

// ValidateWorkflow ..
// this function feels exceedingly awkward
func ValidateWorkflow(wf *WorkflowJSON) (bool, *WorkflowGrievances) {
	v := &Validator{Workflow: wf}
	valid := v.Validate()
	return valid, v.Grievances
}

// Validate ..
func (v *Validator) Validate() bool {
	g := &WorkflowGrievances{
		Main:      make(Grievances, 0),
		ByProcess: make(map[string]Grievances),
	}
	v.Grievances = g

	// collect grievances

	// check if '$graph' field is populated
	if v.Workflow.Graph == nil {
		g.Main.log("missing graph")
	}

	// check version
	// here also validate that the cwlVersion matches
	// the version currently supported by mariner
	// todo
	if v.Workflow.CWLVersion == "" {
		g.Main.log("missing cwlVersion")
	}

	// check that '#main' routine (entrypoint into the graph) exists
	foundMain := false
	for _, obj := range *v.Workflow.Graph {
		if obj["id"] == "#main" {
			foundMain = true
			// recursively validate the whole graph
			v.validate(obj, "")
			break
		}
	}
	if !foundMain {
		g.Main.log("missing '#main' workflow")
	}

	// if there are grievances, report them
	if !g.empty() {
		return false
	}
	return true
}

func (wfg *WorkflowGrievances) empty() bool {
	if len(wfg.Main) > 0 {
		return false
	}
	for _, g := range wfg.ByProcess {
		if len(g) > 0 {
			return false
		}
	}
	return true
}

// check required field
func fieldCheck(obj map[string]interface{}, field string, g Grievances) bool {
	valid := true
	if i, ok := obj[field]; !ok {
		g.log("missing required field: '%v'", field)
		valid = false
	} else if mapToArray[field] {
		// enforce array structure
		// because of cwl.go's internals
		// later, if I change cwl.go library to be map-based instead of array-based
		// this check has to change to enforce map[string]interface{} structure
		if _, ok = i.([]interface{}); !ok {
			g.log("value for field '%v' must be an array", field)
			valid = false
		}
	}
	return valid
}

// recursively validate each cwl object in the graph
// log any grievances encountered
func (v *Validator) validate(obj map[string]interface{}, parentID string) {
	id := obj["id"].(string)
	g := make(Grievances, 0)
	v.Grievances.ByProcess[id] = g

	// collect grievances for this object

	// NOTE: don't need super specific checks
	// just rough checks are okay for first build

	// all general checks here
	var commonFields = []string{
		"inputs",
		"outputs",
		"class",
	}

	for _, field := range commonFields {
		fieldCheck(obj, field, g)
	}

	// here all class-specific checks
	var class string
	switch class {
	case "CommandLineTool":
		// no specific validation here yet
	case "Workflow":
		if valid := fieldCheck(obj, "steps", g); valid {
			steps := obj["steps"].([]interface{})
			for _, step := range steps {
				v.validateStep(step, id)
			}
		}
	case "ExpressionTool":
		fieldCheck(obj, "expression", g)
	default:
		g.log(fmt.Sprintf("invalid value for field 'class': %v", class))
	}
}

// validate a workflow step
// call validate routine on referenced graph object
func (v *Validator) validateStep(step interface{}, parentID string) {
	// g := v.Grievances.ByProcess[parentID]

	// first thing for now - find 'run' field
	// call validate routine on the value of that field
	// since it's "run: id"

}
