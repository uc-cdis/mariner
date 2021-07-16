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
and you can do that via the API call demonstrated in step 3 from the [Quickstart](run_a_workflow.md) section.

#### Notes

Notice you can apply tags to your workflow request,
which can be useful for identifying or categorizing your workflow runs.
For example if you are running a certain set of workflows for one study,
and another set of workflows for another,
you could apply a studyID tag to each workflow run.

The `manifest` field will (very) soon be removed from the workflow request body,
since of course Mariner can generate the required manifest
by parsing the inputs mapping file and collecting all the GUIDs it comes across.
