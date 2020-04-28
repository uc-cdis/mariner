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



## How To Use The Tool

If necessary, first [install Go on your machine](https://golang.org/doc/install)
and ensure that the path to your Go binaries is part of your $PATH so that
the tool will be recognized at the command-line.

Now you can fetch and install the tool by running this command:

```
go get github.com/uc-cdis/mariner/conformance
```

Clone the repo containing the CWL conformance tests:

```
git clone https://github.com/common-workflow-language/common-workflow-language
```



