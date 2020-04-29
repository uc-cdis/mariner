# Mariner CWL Conformance Testing Tool

Mariner is a workflow execution service written in [Go](https://golang.org/) 
for running workflows written in [Common Workflow Language (CWL)](https://www.commonwl.org/)
on [Kubernetes](https://kubernetes.io/).

CWL has a [suite of conformance tests](https://github.com/common-workflow-language/common-workflow-language/blob/master/v1.0/conformance_test_v1.0.yaml)
(see actual CWL files [here](https://github.com/common-workflow-language/common-workflow-language/tree/master/v1.0/v1.0))
associated with it which are instrumental in testing workflow engine implementations
to see how much and what parts of the CWL specification are supported in a particular implementation.

`conformance` is a command-line interface for automated and efficient conformance-testing of Mariner.
Each time you run a test set the tool generates a report (in the form of JSON)
which contains test results and the full Mariner logs for each test case.
In particular for failed tests, the error messages from the Mariner logs
are extracted in order to facilitate diagnosing the cause of failure.

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

## Setup

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

Your current working directory should now look something like this:

```
Matts-MacBook-Pro:testTool mattgarvin$ ls
common-workflow-language	creds.json
```

Now you're ready to run some conformance tests!

You can view the tool's usage by passing it a flag it doesn't recognize - e.g., `-help`:

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

## Filters and Example Usage

The full set of conformance tests consists of about 200 test cases.
You may be interested in running only a subset of these tests -
for example, maybe you want to run only the negative tests,
or you want to run only those tests which test the scatter/gather features.

You can do exactly that by passing filter flags to `conformance`.
To view the set of test cases resulting from a particular filter
without actually running them, pass the `-showFiltered` flag and 
don't pass the `-run` flag. For example, you can view the first 
test case like so:

```
Matts-MacBook-Pro:testTool mattgarvin$ conformance -cwl ./common-workflow-language/ -id 1 -showFiltered
[
   {
      "job": "common-workflow-language/v1.0/v1.0/bwa-mem-job.json",
      "output": {
         "args": [
            "bwa",
            "mem",
            "-t",
            "2",
            "-I",
            "1,2,3,4",
            "-m",
            "3",
            "chr20.fa",
            "example_human_Illumina.pe_1.fastq",
            "example_human_Illumina.pe_2.fastq"
         ]
      },
      "should_fail": false,
      "tool": "common-workflow-language/v1.0/v1.0/bwa-mem-tool.cwl",
      "label": "cl_basic_generation",
      "id": 1,
      "doc": "General test of command line generation",
      "tags": [
         "required",
         "command_line_tool"
      ]
   }
]
--- nTests: 1 ---
```

Filter by ID:

```
conformance -cwl ./common-workflow-language -id 1,2,3 -showFiltered 
```

Filter by label:

```
conformance -cwl ./common-workflow-language -lab cl_basic_generation,nested_cl_bindings -showFiltered
```

Filter by tags:

```
conformance -cwl ./common-workflow-language -tag schema_def,command_line_tool -showFiltered
```

Only negative test cases:

```
conformance -cwl ./common-workflow-language -neg -showFiltered
```

Passing multiple filter flags results in a union of the sets of 
test cases defined by each individual filter flag.
For example, the following command selects for
the first two tests as well as all negative tests:

```
conformance -cwl ./common-workflow-language -id 1,2 -neg -showFiltered
```

To actually run the tests, pass the `-run` flag:

```
conformance -cwl ./common-workflow-language -creds ./creds.json -id 1 -run -out report.json
```

Run all tests:

```
conformance -cwl ./common-workflow-language -creds ./creds.json -run -out report.json
```

By default, tests are run concurrently. The default max number of tests allowed to be running
at one time is 4, but you can define that limit yourself by passing the `-async` flag:

```
conformance -cwl ./common-workflow-language -creds ./creds.json -async 8 -run -out report.json
```


