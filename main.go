package main

import (
	"errors"
	"io/ioutil"
	"log"
	"os"

	"github.com/uc-cdis/gen3cwl/gen3cwl"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "gen3cwl"
	app.Usage = "Run CWL job"
	app.Action = func(c *cli.Context) error {
		workflowPath := c.Args().Get(0)
		inputsPath := c.Args().Get(1)
		if workflowPath == "" {
			return errors.New("Missing workflow")
		}
		if inputsPath == "" {
			return errors.New("Missing Inputs")
		}
		workflowF, err := os.Open(workflowPath)
		if err != nil {
			return err
		}
		if _, err := os.Open(inputsPath); err != nil {
			return err
		}
		workflow, err := ioutil.ReadAll(workflowF)
		if err != nil {
			return err
		}
		return gen3cwl.RunWorkflow(workflow)
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
