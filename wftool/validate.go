package main

import ()

// WorkflowJSON ..
type WorkflowJSON struct {
	Graph      *[]map[string]interface{} `json:"$graph"`
	CWLVersion string                    `json:"cwlVersion"`
}

// Grievances ..
type Grievances []string

func (g Grievances) log(m interface{}) {
	switch x := m.(type) {
	case string:
		g = append(g, x)
	case []string:
		g = append(g, x...)
	}
}

// could optionally take path or []bytes input?
func (wf *WorkflowJSON) validate() (bool, []string) {
	g := make(Grievances, 0)

	// collect grievances

	// check if '$graph' field is populated
	if wf.Graph == nil {
		g.log("missing graph")
	}

	// check version
	// here also validate that the cwlVersion matches
	// the version currently supported by mariner
	// todo
	if wf.CWLVersion == "" {
		g.log("missing cwlVersion")
	}

	// check that '#main' routine (entrypoint into the graph) exists
	foundMain := false
	for _, obj := range *wf.Graph {
		if obj["id"] == "#main" {
			foundMain = true
			ok, gg := wf.traceGraph(obj) // trace graph and validate each vertex
			if !ok {
				g.log(gg)
			}
			break
		}
	}
	if !foundMain {
		g.log("missing '#main' workflow")
	}

	if len(g) > 0 {
		return false, g
	}
	return true, nil
}

// notice the make(g), check, return pattern - same in these validaton functions
// the same pattern at different depths

// trace graph and collect grievances
func (wf *WorkflowJSON) traceGraph(main map[string]interface{}) (bool, Grievances) {
	g := make(Grievances, 0)

	// collect grievances

	if len(g) > 0 {
		return false, g
	}
	return true, nil
}
