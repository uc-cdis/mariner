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

// recursively validate each cwl object in the graph
// log any grievances encountered
func (v *Validator) validate(obj map[string]interface{}, parentID string) {
	g := make(Grievances, 0)
	v.Grievances.ByProcess[obj["id"].(string)] = g

	// collect grievances for this object

	var commonFields = []string{
		"inputs",
		"outputs",
		"class",
	}

	var ok bool
	for _, field := range commonFields {
		if _, ok = obj[field]; !ok {
			g.log("missing field: '%v'", field)
		}
	}

	// all general checks here

	// NOTE: don't need super specific checks
	// just rough checks are okay for first build

	// here all class-specific checks
	// for now, case sensitive, strict match
	var class string
	switch class {
	case "CommandLineTool":
	case "Workflow":
	case "ExpressionTool":
	default:
		g.log(fmt.Sprintf("invalid value for field 'class': %v", class))
	}
}

// of course, here, making the assumption that 'id' is a string in the json
// which ultimately is not an assumption we will make

/*
func (v *Validator) validateCLT(clt map[string]interface{}, parentID string) {
	g := v.Grievances.ByProcess[clt["id"].(string)]

	// collect g's
}

func (v *Validator) validateWF(wf map[string]interface{}, parentID string) {
	g := v.Grievances.ByProcess[wf["id"].(string)]
}

func (v *Validator) validateET(et map[string]interface{}, parentID string) {
	g := v.Grievances.ByProcess[et["id"].(string)]
}
*/
