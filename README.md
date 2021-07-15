# Mariner: The Gen3 Workflow Execution Service

![version](https://img.shields.io/github/release/uc-cdis/mariner.svg) [![Apache license](http://img.shields.io/badge/license-Apache-blue.svg?style=flat)](LICENSE)

Mariner is a workflow execution service written in [Go](https://golang.org)
for running [CWL](https://www.commonwl.org) workflows on [Kubernetes](https://kubernetes.io).
Mariner's API is an implementation of the [GA4GH](https://www.ga4gh.org)
standard [WES API](https://ga4gh.github.io/workflow-execution-service-schemas).

The original technical design proposal for Mariner can be found [here](https://github.com/uc-cdis/mariner/blob/master/TechnicalDesignProposal.md).

## How to deploy Mariner in a Gen3 environment

### Prereq's

1. Mariner depends on the [Workspace Token Service (WTS)](https://github.com/uc-cdis/workspace-token-service)
to access data from the commons.
If WTS is not already running in your environment, deploy the WTS.

2. Add the Mariner pieces to your manifest:
    1. Add [version](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L19)
    2. Add [config](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L183-L292)

### Deployment

3. Deploy the Mariner server by running `gen3 kube-setup-mariner`

### Auth and User YAML

4. Mariner utilizes Gen3's policy engine, [Arborist](https://github.com/uc-cdis/arborist), for authorization. Make sure you have the following Mariner auth scheme in your User YAML:
    1. [Policy](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L57-L60)
    2. [Resource](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L419-L420)
    3. [Role](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L577-L582)

5. Give the `mariner_admin` policy to those users who need it. ([example](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L1433))

#### Auth Note

Right now the Mariner auth scheme is atomic - you
either have access to all the API endpoints or none of them.
In order for a user to interact with Mariner, that user will need to have Mariner admin privileges.

A Mariner admin can do the following:
  - Run workflows
  - Fetch run status via runID
  - Fetch run logs and output via runID
  - Cancel a run that's in-progress via runID
  - Query run history (i.e., fetch a list of all your runIDs)

## How to use Mariner

### A Full Example

To demonstrate how to interact with Mariner, here's a step-by-step process
of how to run a small test workflow and hit all the Mariner API endpoints.

1. On your machine, move to directory `testdata/no_input_test`

2. Fetch token using API key
```
echo Authorization: bearer $(curl -d '{"<replaceme>": "<replaceme>", "key_id": "<replaceme>"}' -X POST -H "Content-Type: application/json" https://<replaceme>.planx-pla.net/user/credentials/api/access_token | jq .access_token | sed 's/"//g') > auth
```

3. POST the workflow request
```
curl -d "@request_body.json" -X POST -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs
```

4. Check run status
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>/status
```

5. Fetch run logs (includes output json)
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>
```

6. Fetch your run history (list of runIDs)
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs
```

7. Cancel a run that's currently in-progress
```
curl -d "@request_body.json" -X POST -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>/cancel
```

### Writing And Running Your Own Workflows

A workflow request to Mariner consists of the following:
1. A CWL workflow (serialized into JSON)
2. An inputs mapping file (also in the form of JSON)

The workflow specifies the computations to run,
the inputs mapping file specifies the data to run those computations on.

So if you want to write and run your own workflow with Mariner,
the process would go like this:

1. Write your CWL workflow.

2. Use the [Mariner wftool](https://github.com/uc-cdis/mariner/tree/master/wftool)
to serialize your CWL file(s) into a single JSON file.

3. Create your inputs mapping file, which
is a JSON file where the keys are CWL input parameters
and the values are the corresponding input values
for those parameters. Here is an example
of an inputs mapping file with two input files.
One file is commons data and is specified by a GUID
with the prefix `COMMONS/`. The other file is a user file, which exists in
the user data space (an S3 bucket) and is specified by
the file path within that user data space
plus the prefix `USER/`:
```
{
    "commons_file_1": {
        "class": "File",
        "location": "COMMONS/8bc9f306-5b5d-4b6b-b34e-f90680824b17"
    },
    "user_file": {
        "class": "File",
        "location": "USER/user-data.txt"
    }
}
```


4. Now you can construct the Mariner workflow request
JSON body, which looks like this:
```
{
  "workflow": <output_from_wftool>,
  "input": <inputs_mapping_json>,
  "manifest": <manifest_containing_GUIDs_of_all_commons_input_data>,
  "tags": {
    "author": "matt",
    "type": "example",
  }
}
```

An example request body can be found [here](https://github.com/uc-cdis/mariner/blob/master/testdata/user_data_test/request_body.json).

5. At this point you're ready to ask Mariner to run your workflow,
and you can do that via the API call demonstrated in step 3 from the [A Full Example](#A-Full-Example) section above.

#### Notes

Notice you can apply tags to your workflow request,
which can be useful for identifying or categorizing your workflow runs.
For example if you are running a certain set of workflows for one study,
and another set of workflows for another,
you could apply a studyID tag to each workflow run.

The `manifest` field will (very) soon be removed from the workflow request body,
since of course Mariner can generate the required manifest
by parsing the inputs mapping file and collecting all the GUIDs it comes across.

#### Learning Resources

A good way to get a handle on CWL in a relatively short period of time
is to explore the [CWL User Guide](https://www.commonwl.org/user_guide/02-1st-example/index.html),
which contains a number of example workflows with explanations
of all the different parts of the syntax - what they mean and how they function -
in the context of each example.

### Browsing and Retrieving Output From A Workflow Run

Mariner implicitly depends on the existence of a user data client,
an API where users can browse/upload/download/delete files
from their user data space, which is persistent storage
on the Gen3/commons side for data which belongs to a user
and is not commons data.

The user data space is where a user can stage files to be input
to a workflow run, and theoretically, also the same place
where users can stage input files for any "app on Gen3", e.g., a Jupyter notebook.

The user data space (also could be called an "analysis space") is also
where output files from apps are stored.

Currently, there's an S3 bucket which is a dedicated user data space,
where keys at the root are userID's, and any file which belongs to user_A
has `user_A/` as a prefix. Per workflow run, there is a working directory
created and dedicated to that run, under that user's prefix in that S3 bucket.
All files generated by the workflow run are written to this working directory,
and any files which are not explicitly listed as output files of the top-level workflow
(i.e., all intermediate files) get deleted at the end of the run so that only
the desired output files are kept.

Currently there does not exist a Gen3 user-data-client,
so in order to browse and retrieve your output files from
the workflow's working directory in S3,
you must use the [AWS S3 CLI](https://docs.aws.amazon.com/cli/latest/reference/s3/) directly.

## Running the CWL Conformance Tests against Mariner

See [here](https://github.com/uc-cdis/mariner/tree/master/conformance).
