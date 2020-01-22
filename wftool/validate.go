package main

import ()

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

func (g Grievances) log(m interface{}) {
	switch x := m.(type) {
	case string:
		g = append(g, x)
	case []string:
		g = append(g, x...)
	}
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
func (v *Validator) validate(obj map[string]interface{}, parentID string) {
	g := make(Grievances, 0)

	// collect grievances for this object

	if len(g) > 0 {
		v.Grievances.ByProcess[obj["id"].(string)] = g
	}
}
