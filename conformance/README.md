# Mariner CWL Conformance Testing Tool

Mariner is a workflow execution service written in [Go](https://golang.org/) 
for running workflows written in [Common Workflow Language (CWL)](https://www.commonwl.org/)
on [Kubernetes](https://kubernetes.io/).

CWL has a [suite of conformance tests](https://github.com/common-workflow-language/common-workflow-language/blob/master/v1.0/conformance_test_v1.0.yaml)
(see actual CWL files [here](https://github.com/common-workflow-language/common-workflow-language/tree/master/v1.0/v1.0))
associated with it which are useful for testing workflow engine implementations
to see how much and what parts of the CWL specification are supported in a particular implementation.

`conformance` is a command-line interface for automated and efficient conformance-testing of Mariner.
