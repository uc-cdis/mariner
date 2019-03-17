package gen3cwl

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestEngine(t *testing.T) {
	cwlfile, _ := os.Open("../testdata/gen3_test.pack.cwl")
	body, _ := ioutil.ReadAll(cwlfile)
	err := RunWorkflow(body)
	if err != nil {
		t.Error(err.Error())
	}

	// fmt.Printf("step \n")
	// cwlfile, _ := os.Open("./testdata/step.json")
	// body, _ := ioutil.ReadAll(cwlfile)
	// var step cwl.Step
	// json.Unmarshal(body, &step)
	// fmt.Printf("%v \n", step.In[0].Source)

	// fmt.Printf("workflow \n")
	// cwlfile2, _ := os.Open("./testdata/subworkflow.json")
	// body2, _ := ioutil.ReadAll(cwlfile2)
	// var workflow cwl.Root
	// json.Unmarshal(body2, &workflow)
	// fmt.Printf("%v \n", workflow.Steps[0].In[0].Source)
}
