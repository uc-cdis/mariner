# mariner: the gen3 workflow engine

## Context 

### What are "workflows" and why should we run them?

A workflow is a partially ordered set of computations,
where the output of one computation may be the input of another computation,
while other computations may be independent of one another.

The computations are partially ordered because
if computation A takes as input the output of computation B,
then B has to run, and finish running, before A runs.
However, if computations X and Y are not related via input/output (i.e., they are independent),
then X and Y can run concurrently.

Bioinformatics at scale necessitates running workflows over massive amounts of data.
In order for Gen3 to be a more complete and useful cloud-based bioinformatics platform,
Gen3 needs the functionality to run these large scale workflows.

### What does it mean to "run a workflow"?

A workflow is specified in some workflow language.
Examples of workflow languages include [Common Workflow Language (CWL)](https://www.commonwl.org/)
and [Workflow Description Language (WDL)](https://software.broadinstitute.org/wdl/documentation/spec).
There exist many less prevalent workflow languages.
Hopefully eventually one workflow language will "win out", and become an actual standard.
In this first iteration of the workflow execution service, we will only support running workflows written in CWL.
CWL is basically a schema over YAML.
A [workflow](https://www.commonwl.org/v1.0/Workflow.html) consists of many CWL files,
where one file defines the top-level workflow,
and every other file defines either a subworkflow or a [tool](https://www.commonwl.org/v1.0/CommandLineTool.html) of that workflow.
The workflow execution service will handle "packed" workflows, which are workflows in JSON,
where all the CWL files for that workflow have been serialized into one JSON object.

So let's say you define a workflow in CWL.
In order to actually execute this workflow, you need to pass the CWL to some engine
which parses the CWL and then schedules and executes all the associated jobs.
The engine must also handle all the input/output dependencies among workflow steps,
including passing the output of one step to the input of another step.

In particular, your workflow might consist of several containerized bioinformatics processes.
In this case, for a given containerized computation, the engine must pull the specified image
and run the computation in a container built from that image.

### Aren't there engines out there already that run workflows? Why did we write our own engine?

There do exist other workflow engines. Examples include [Cromwell](https://cromwell.readthedocs.io/en/stable/) and [cwltool](https://github.com/common-workflow-language/cwltool).

Reasons we wrote our own:
  - allow for seamless integration with kubernetes
  - extensible design to allow for simple integration with the rest of Gen3

## mariner API 

The core of the API will be the [GA4GH Workflow Execution Service (WES) API](https://github.com/ga4gh/workflow-execution-service-schemas).
Optionally we may implement extensions or additional functionality.
Do let me know if you have any ideas or thoughts on how the mariner API should be, what it should be able to do.

Endpoints:

/service-info
  - GET: get service information, currently supported workflow languages and versions, currently supported WES version

/runs
  - GET: list workflow runs (returns list of (runID, status) pairs)
    - it would be nice to differentiate between past and in-progress runs,
    so that a user can retrieve a list of only those runs which are currently in-progress
    - alternatively, retrieve a list of only those runs which have finished running
    - also a basic option which returns all runs, both completed and in-progress
  - POST: run a workflow; returns runID for the workflow job

/runs/{runID}
  - GET: get complete logs for the given workflow run (i.e., full entry for that run from workflowHistorydb)
    - might be nice for there to be an endpoint/option for: get output of a successfully completed workflow run,
    which returns only the output JSON and none of the logs/stats/versioning information from the run.
    Unsure of the usefulness of a separate endpoint for this though, if the parent endpoint
    already returns the full entry from workflowHistorydb, which contains the output JSON.

/runs/{runID}/cancel
  - POST: cancel a workflow run which is currently in-progress

/runs/{runID}/status
  - GET: get status of a workflow run (complete | in-progress | failed)

## System Components

### mariner
  - server
    - listens for and handles API calls
    - upon authZ'd workflow request, dispatches engine job to run a workflow
    - the server may dispatch arbitrarily many engine jobs
  - engine (1 workflow <-> 1 engine job)
    - resolves workflow graph
    - manages input/output dependencies among workflow steps
    - schedules and dispatches workflow tasks as jobs
  - task (1 engine job <-> many task jobs)
    - runs a particular workflow tool
    - if image specified for the tool, the task runs in a container built from the specified image

### workflowHistorydb
  - stores logs of all workflow runs

### data entities
  - data commons (input source)
  - user data (input source)
  - engine workspace (serves as an isolated working directory for a particular workflow run)

### auth
  - every call to the mariner API must include a token for auth
  - mariner passes token from API request to arborist to check that user's privileges;
  only upon arborist's okay does mariner perform the requested action

## How does it work? 

Prerequisite: an API token

To run a workflow, pass (workflow, inputs, token)
as the JSON body of a POST request to the mariner API /runs endpoint.

mariner will first check authorization for the user by passing the token
to arborist. arborist will check workflow privileges for the user
and return either "okay" or "not okay" for this user to run a workflow.

If the user is authorized to run workflows, then the mariner-server will dispatch
an instance of mariner-engine as a k8s job to run the workflow.

mariner-engine resolves the graph of the workflow and
creates an input/output dependency map for all the steps of the workflow.
the engine uses the dependency map to schedule all the tasks of the workflow as k8s jobs.
1 task <-> 1 job
independent steps run concurrently, while if step A takes as input the output of step B,
then A does not get dispatched until B finishes running and the output of B has been collected.

mariner-engine logs all events of the workflow run and incrementally writes these logs to workflowHistorydb.

the final output JSON of the workflow execution gets written to workflowHistorydb

## Who has authZ to run workflows? 

Anyone with "admin" privileges can run workflows.

In the first iteration, Matt and some of the bioinformaticians and developers will have admin privileges for testing and running workflows internally.

## Logs, workflowHistorydb 

Logs are written to workflowHistorydb incrementally to allow for retrieval of logs
in the case where the workflow run fails at any point
in particular, this allows for debugging failed workflows.

A complete record for a workflow run could be a JSON identified with key /userID/runID/ consisting of fields:
- status
- logs
- output
- stats

Any thoughts on the workflowHistorydb (what kind of storage to use, how to organize the db) are welcome.

## The Engine Workspace 

A workflow run generates files, and workflow tasks may define and depend on particular directory structures
for their execution to run as intended.

Additionally, CWL spec mandates that each task must run in its own isolated container and workspace (i.e., directory)

For these reasons, when a workflow runs, the whole workflow run has its own workspace,
which is just a directory in a bucket.
And each task of that workflow also has its own workspace, which is just a subdirectory there.

engine workspace bucket directory structure:
/userID/runID/
  - task_1/
    - some_file
  - task_2/
  ...

During workflow execution, the workspace for that workflow run gets mounted (fuse mount via <goofys>)
to the engine job as well as each task job.

The bucket gets mounted at the /userID/runID/ prefix,
so that user_1's engine workspace and files are not exposed to user_2's workflow run.

(Open Question 1 - workflow workspace)
An open question is to how this feature of the system will scale.
One solution is to have a collection of buckets be dedicated to being workspace for workflows.
There would be an abstraction layer over the buckets
so that every time a workflow runs, a workspace for that run gets mounted
without reference to a particular bucket.

I would love to hear other ideas and potential solutions, and discuss them, if others have thoughts on this engine-workspace issue.

## Data Flow

mariner relies on three data entities:
  - Commons Data (input source)
  - User Data (input source)
  - Engine Workspace
    - stores all files generated by workflow run
    - files generated by intermediate steps (stored here) may be used as inputs to later steps

Per workflow run, each data entity is mounted to the engine job as well as to each task job.
mariner reads from commons-data and user-data, and reads/writes from/to engine-workspace.

### Where does input data come from?

When you submit a workflow execution request,
you must include a JSON which maps workflow input parameters to actual input values to use for execution.
Input parameters may be of different types, e.g., file, string, integer, boolean, etc.

Inputs of type "file" may be one of two types:
1. Commons data
2. User data

Specify a commons data file by its GUID.
Commons data is made available to the workflow run via gen3fuse.
Commons data is mounted to the engine job as well as each task job.

(Open Question 2 - user-data)
Right now it's an open question as to how to support the following use-case:
A user has data files (either locally or in some bucket somewhere) which are not commons data, and the user wants
to run a workflow using these non-commons data files as input to the workflow.

In the short term for internal usage, we could create one S3 bucket,
and all data files for testing and internal usage of the service could live there.
To use a data file from the bucket as input to a workflow,
you would specify the file by giving its full path within the bucket.
This data would be made available to the workflow run via fuse mount similar to gen3fuse ([goofys](https://github.com/kahing/goofys)).

A potential solution which scales is to create a user-data-space
which is a collection of buckets with an abstraction layer over it.
And there would be a user-data-client which the user could use to
browse, upload, download, delete files from their user-data-space
without reference to a particular bucket.
Users could stage input files for a workflow run by uploading them to their user-data-space
and specify them either by a path or objectID of some kind.
Per workflow run, that user's user-data-space would be mounted and those data files made available
to the engine job as well as each task job.

Again on this open question, I would love to hear people's ideas on how to support this use-case.

(Open Question 3 - data in other buckets)
Another unaddressed use-case/open question:
User has a large amount of data in their bucket(s) somewhere, which may or may not be private/controlled-access.
User wants to run workflow using these data as input, without moving/staging the data to some user-data-space.

### What is the output of a workflow and where does it end up?

Every workflow defines a set of output parameters.
Output parameters, like input parameters, may be of different types: file, int, bool, string, etc.

Every workflow run results in a JSON object which maps output parameters to values.
File outputs are specified by paths relative to the /userID/runID/ directory for that run
in the engine-workspace bucket.

The output JSON object is written to workflowHistorydb.

All files generated during a workflow run are stored in the engine workspace,
and final output files are no exception.

So the final output files of a workflow run are stored in the workspace for that run, in engine-workspace-bucket/userID/runID/.

### How do you retrieve workflow output?

To retrieve the output JSON for a completed workflow run, pass the runID to the mariner /output endpoint.

(Open question 4 - retrieving output data files)
It's currently an open question as to how users would browse and retrieve data files resulting from a workflow run.

One idea:
Some service which allows users to access
  1. user-data-space
  2. engine-workspace/userID/
Present both of these entities as separate directories, in one place
So that a user could browse, upload, download, delete files from their user-data-space
As well as browse and download files from their engine-workspace
All in one place.

## Open Questions 

1. What is the long-term solution for the engine-workspace issue?
Currently there is a single bucket which gets used as the workspace for all workflow runs.
This does not scale.
What are possible designs and solutions for the engine-workspace which scale
and could possibly be generally applied to other similar problems or used by other services?

2. What is the solution for the user-data use-case?
Will we have something like a user-data-space where users can stage input files for workflow runs?
This would necessitate something like a user-data-client service, which would be a little bit more than
a wrapper around the AWS CLI.

3. How could we support the use-case where a user has a large amount of (possibly private) data in a bucket
that they would like to use as input to a workflow run, but they cannot or do not want to move/stage the data
to some user-data-space?
If the user specified full paths from within that bucket,
we could just mount the bucket and things would run fine.
But that would necessitate the user passing aws credentials to mariner somehow so that mariner can mount arbitrary buckets.

4. How will users browse and download output files from a workflow run?
Presently, all files generated by a workflow run are stored in the engine-workspace.
Also presently, the engine-workspace is a single S3 bucket.
Browsing and downloading files from a workflow run
means browsing and downloading files from this S3 bucket.
Presently Matt (or anyone who has the aws user creds) could manually interact with the bucket.
This might be okay for testing, but is not okay for anything beyond that.
We would need some service or mechanism to allow users to browse and download
files from the engine-workspace.
Could be the same as the user-data-client,
where the user-data-client would connect a user with their user-data-space as well as their workflow-workspace (engine-workspace)
Also, it's not just the final output files that are of interest.
All files generated during a workflow run are of interest and must be made available
to allow for checking/verifying intermediate outputs are as expected,
for optimization of workflows, debugging failed workflow runs, etc.

5. What functionality/endpoints (in addition to the WES API) should the mariner API have?

