# Mariner CWL Conformance Testing Tool

Mariner is a workflow execution service written in [Go](https://golang.org/) 
for running workflows written in [Common Workflow Language (CWL)](https://www.commonwl.org/)
on [Kubernetes](https://kubernetes.io/).

CWL has a [suite of conformance tests](https://github.com/common-workflow-language/common-workflow-language/blob/master/v1.0/conformance_test_v1.0.yaml)
(see actual CWL files [here](https://github.com/common-workflow-language/common-workflow-language/tree/master/v1.0/v1.0))
associated with it which are useful for testing workflow engine implementations
to see how much and what parts of the CWL specification are supported in a particular implementation.

`conformance` is a command-line interface for automated and efficient conformance-testing of Mariner.

## PreReq's

The tool is built to test a particular instance of Mariner deployed in some environment - 
for example, your dev environment or a QA or staging environment for some commons.
That is, the tool must be "pointed at" a particular environment.
Whatever environment you choose, in order to run the tests 
you must have authZ to access the Mariner API and run workflows in that environment.

Additionally, the tests depend on a small collection of test input files
being present in the compute environment in order to run as desired.
This hurdle is in the process of being removed -
check back soon for the solution!

## How To Use The Tool

If necessary, first [install Go on your machine](https://golang.org/doc/install)
and ensure that the path to your Go binaries is part of your $PATH so that
the tool will be recognized at the command-line.

1. Now you can fetch and install the tool by running this command:

```
go get github.com/uc-cdis/mariner/conformance
```

2. Clone the repo containing the CWL conformance tests:

```
git clone https://github.com/common-workflow-language/common-workflow-language
```

3. Have your API key for the target environment on-hand. Or if you don't have one, generate one - i.e., download `creds.json` 
from the target environment's portal and move it to your current working directory.

Now you're ready to run some conformance tests!

You can view the tool's usage by passing it the `-help` flag:

```
Matts-MacBook-Pro:testTool mattgarvin$ conformance -help
Usage of conformance:
  -async int
    	specify maximum number of tests concurrently running at any given time (default 4)
  -creds string
    	path to creds (i.e., the api key json from the portal) (default "./creds.json")
  -cwl string
    	path to the common-workflow-language repo (default "./common-workflow-language")
  -id value
    	comma-separated list of IDs by which to filter the test cases
  -lab value
    	comma-separated list of labels by which to filter the test cases
  -neg
    	if provided, then filter for negative test cases
  -out string
    	path to output json containing test results
  -pos
    	if provided, then filter for positive test cases
  -run
    	bool indicating whether the user wants to run the selected tests
  -showFiltered
    	specify whether to send resulting set of test cases after filter to stdout
  -tag value
    	comma-separated list of tags by which to filter the test cases
```



