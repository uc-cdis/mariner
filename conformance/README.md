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

You can view the tool's usage by passing it a flag it doesn't recognize - e.g., `-h`:

```
Matts-MacBook-Pro:testTool mattgarvin$ conformance -h
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

To run all tests, don't pass any filter flags:

```
conformance -cwl ./common-workflow-language -creds ./creds.json -run -out report.json
```

By default, tests are run concurrently. The default max number of tests allowed to be running
at one time is 4, but you can define that limit yourself by passing the `-async` flag:

```
conformance -cwl ./common-workflow-language -creds ./creds.json -async 8 -run -out report.json
```

## Example Run and Output

Here's what it looks like when we run the first test case and view the results:

```
Matts-MacBook-Pro:testTool mattgarvin$ conformance -cwl ./common-workflow-language/ -creds ./creds.json -id 1 -run -out results.json

--- running 1 tests ---
--- async settings: ---
{
   "Enabled": true,
   "MaxConcurrent": 4
}

------ running test 1 ------
--- 1 - packing cwl to json
--- 1 - loading inputs
--- 1 - collecting tags
--- 1 - POSTing request to mariner
--- 1 - marshalling RunID to json
--- 1 - runID: 042920031731-qsxfj
--- 1 - waiting for run to finish
--- 1 - run status: completed
--- 1 - matching output
------ writing test results to results.json ------
Matts-MacBook-Pro:testTool mattgarvin$ cat results.json 
{
  "timestamp": "042820221729",
  "duration": "30s",
  "async": {
    "Enabled": true,
    "MaxConcurrent": 4
  },
  "results": {
    "Total": 1,
    "Coverage": 0,
    "Pass": 0,
    "Fail": 1,
    "Manual": 0
  },
  "log": {
    "Pass": {},
    "Fail": {
      "1": {
        "TestCase": {
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
        },
        "TimeOut": false,
        "FailedToKillJob": false,
        "LocalError": null,
        "MarinerError": [
          "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to run CommandLineTool: #main; error: failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
          "2020/4/29 3:17:54 - ERROR - failed to run tool: #main; error: failed to run CommandLineTool: #main; error: failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined"
        ],
        "RunLog": {
          "request": {
            "Workflow": {
              "$graph": [
                {
                  "arguments": [
                    "bwa",
                    "mem",
                    {
                      "position": 1,
                      "prefix": "-t",
                      "valueFrom": "$(runtime.cores)"
                    }
                  ],
                  "baseCommand": "python",
                  "class": "CommandLineTool",
                  "cwlVersion": "v1.0",
                  "hints": [
                    {
                      "class": "ResourceRequirement",
                      "coresMin": 2,
                      "ramMin": 8
                    },
                    {
                      "class": "DockerRequirement",
                      "dockerPull": "python:2-slim"
                    }
                  ],
                  "id": "#main",
                  "inputs": [
                    {
                      "id": "#main/reference",
                      "inputBinding": {
                        "position": 2
                      },
                      "type": "File"
                    },
                    {
                      "id": "#main/reads",
                      "inputBinding": {
                        "position": 3
                      },
                      "type": {
                        "items": "File",
                        "type": "array"
                      }
                    },
                    {
                      "id": "#main/minimum_seed_length",
                      "inputBinding": {
                        "position": 1,
                        "prefix": "-m"
                      },
                      "type": "int"
                    },
                    {
                      "id": "#main/min_std_max_min",
                      "inputBinding": {
                        "itemSeparator": ",",
                        "position": 1,
                        "prefix": "-I"
                      },
                      "type": {
                        "items": "int",
                        "type": "array"
                      }
                    },
                    {
                      "default": {
                        "class": "File",
                        "location": "args.py"
                      },
                      "id": "#main/args.py",
                      "inputBinding": {
                        "position": -1
                      },
                      "type": "File"
                    }
                  ],
                  "outputs": [
                    {
                      "id": "#main/sam",
                      "outputBinding": {
                        "glob": "output.sam"
                      },
                      "type": [
                        "null",
                        "File"
                      ]
                    },
                    {
                      "id": "#main/args",
                      "type": {
                        "items": "string",
                        "type": "array"
                      }
                    }
                  ],
                  "stdout": "output.sam"
                }
              ],
              "cwlVersion": "v1.0"
            },
            "Input": {
              "min_std_max_min": [
                1,
                2,
                3,
                4
              ],
              "minimum_seed_length": 3,
              "reads": [
                {
                  "class": "File",
                  "location": "example_human_Illumina.pe_1.fastq"
                },
                {
                  "class": "File",
                  "location": "example_human_Illumina.pe_2.fastq"
                }
              ],
              "reference": {
                "checksum": "sha1$hash",
                "class": "File",
                "location": "USER/conformanceTesting/chr20.fa",
                "size": 123
              }
            },
            "Tags": {
              "doc": "General test of command line generation",
              "id": "1",
              "job": "common-workflow-language/v1.0/v1.0/bwa-mem-job.json",
              "label": "cl_basic_generation",
              "should_fail": "false",
              "tags": "required,command_line_tool",
              "tool": "common-workflow-language/v1.0/v1.0/bwa-mem-tool.cwl"
            }
          },
          "main": {
            "created": "2020/4/29 3:17:52",
            "lastUpdated": "2020/4/29 3:17:54",
            "jobID": "f699f9bd-89c7-11ea-a95c-12dda9fc743b",
            "jobName": "042920031731-qsxfj",
            "status": "completed",
            "stats": {
              "cpuReq": {
                "min": 0,
                "max": 0
              },
              "memReq": {
                "min": 0,
                "max": 0
              },
              "resourceUsage": {
                "data": null,
                "samplingPeriod": 0
              },
              "duration": 2.111747974,
              "nfailures": 0,
              "nretries": 0
            },
            "eventLog": [
              "2020/4/29 3:17:52 - INFO - init log",
              "2020/4/29 3:17:52 - INFO - begin resolve graph",
              "2020/4/29 3:17:52 - INFO - end resolve graph",
              "2020/4/29 3:17:52 - INFO - begin run task: #main",
              "2020/4/29 3:17:52 - INFO - begin dispatch task: #main",
              "2020/4/29 3:17:53 - INFO - begin make tool object",
              "2020/4/29 3:17:53 - INFO - begin make task working dir",
              "2020/4/29 3:17:53 - INFO - end make task working dir: /engine-workspace/workflowRuns/042920031731-qsxfj/main/",
              "2020/4/29 3:17:53 - INFO - end make tool object",
              "2020/4/29 3:17:53 - INFO - begin setup tool",
              "2020/4/29 3:17:53 - INFO - begin make tool working dir",
              "2020/4/29 3:17:53 - INFO - end make tool working dir",
              "2020/4/29 3:17:53 - INFO - begin load inputs",
              "2020/4/29 3:17:53 - INFO - tool has no parent workflow",
              "2020/4/29 3:17:53 - INFO - begin load input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - begin transform input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - begin load input value for input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - end load input value for input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - end transform input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - end load input: #main/args.py",
              "2020/4/29 3:17:53 - INFO - begin load input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - begin transform input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - begin load input value for input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - end load input value for input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - end transform input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - end load input: #main/min_std_max_min",
              "2020/4/29 3:17:53 - INFO - begin load input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - begin transform input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - begin load input value for input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - end load input value for input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - end transform input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - end load input: #main/minimum_seed_length",
              "2020/4/29 3:17:53 - INFO - begin load input: #main/reference",
              "2020/4/29 3:17:53 - INFO - begin transform input: #main/reference",
              "2020/4/29 3:17:53 - INFO - begin load input value for input: #main/reference",
              "2020/4/29 3:17:53 - INFO - end load input value for input: #main/reference",
              "2020/4/29 3:17:53 - INFO - end transform input: #main/reference",
              "2020/4/29 3:17:53 - INFO - end load input: #main/reference",
              "2020/4/29 3:17:53 - INFO - begin load input: #main/reads",
              "2020/4/29 3:17:53 - INFO - begin transform input: #main/reads",
              "2020/4/29 3:17:53 - INFO - begin load input value for input: #main/reads",
              "2020/4/29 3:17:53 - INFO - end load input value for input: #main/reads",
              "2020/4/29 3:17:53 - INFO - end transform input: #main/reads",
              "2020/4/29 3:17:53 - INFO - end load input: #main/reads",
              "2020/4/29 3:17:53 - INFO - end load inputs",
              "2020/4/29 3:17:53 - INFO - begin load inputs to js vm",
              "2020/4/29 3:17:53 - INFO - end load inputs to js vm",
              "2020/4/29 3:17:53 - INFO - begin handle InitialWorkDirRequirement",
              "2020/4/29 3:17:53 - INFO - end handle InitialWorkDirRequirement",
              "2020/4/29 3:17:53 - INFO - end setup tool",
              "2020/4/29 3:17:53 - INFO - begin run tool: #main",
              "2020/4/29 3:17:54 - INFO - begin run CommandLineTool: #main",
              "2020/4/29 3:17:54 - INFO - begin generate command",
              "2020/4/29 3:17:54 - INFO - begin process command elements",
              "2020/4/29 3:17:54 - INFO - begin handle command argument elements",
              "2020/4/29 3:17:54 - INFO - begin get value from command element argument",
              "2020/4/29 3:17:54 - INFO - end get value from command element argument",
              "2020/4/29 3:17:54 - INFO - begin get value from command element argument",
              "2020/4/29 3:17:54 - INFO - end get value from command element argument",
              "2020/4/29 3:17:54 - INFO - begin get value from command element argument",
              "2020/4/29 3:17:54 - INFO - begin resolve expression: $(runtime.cores)",
              "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to run CommandLineTool: #main; error: failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:54 - ERROR - failed to run tool: #main; error: failed to run CommandLineTool: #main; error: failed to generate command for tool: #main; error: failed to evaluate js expression: ReferenceError: 'runtime' is not defined",
              "2020/4/29 3:17:55 - INFO - end run task: #main",
              "2020/4/29 3:17:55 - INFO - end run workflow",
              "2020/4/29 3:17:55 - INFO - begin intermediate file cleanup",
              "2020/4/29 3:17:55 - INFO - begin collect paths to keep",
              "2020/4/29 3:17:55 - INFO - end collect paths to keep",
              "2020/4/29 3:17:57 - INFO - end intermediate file cleanupf"
            ],
            "input": {
              "#main/args.py": {
                "basename": "args.py",
                "class": "File",
                "contents": "",
                "location": "args.py",
                "nameext": ".py",
                "nameroot": "args",
                "path": "args.py",
                "secondaryFiles": null
              },
              "#main/min_std_max_min": [
                1,
                2,
                3,
                4
              ],
              "#main/minimum_seed_length": 3,
              "#main/reads": [
                {
                  "class": "File",
                  "location": "example_human_Illumina.pe_1.fastq"
                },
                {
                  "class": "File",
                  "location": "example_human_Illumina.pe_2.fastq"
                }
              ],
              "#main/reference": {
                "basename": "chr20.fa",
                "class": "File",
                "contents": "",
                "location": "/engine-workspace/conformanceTesting/chr20.fa",
                "nameext": ".fa",
                "nameroot": "chr20",
                "path": "/engine-workspace/conformanceTesting/chr20.fa",
                "secondaryFiles": null
              }
            },
            "output": {}
          },
          "byProcess": {}
        }
      }
    },
    "Manual": {}
  }
}
```

