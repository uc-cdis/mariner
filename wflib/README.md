# wftool

wftool serializes ("[packs](https://github.com/common-workflow-language/cwltool#combining-parts-of-a-workflow-into-a-single-document)") CWL into JSON.
For example, given a workflow consisting of 10 `.cwl` files,
where each [workflow step](https://www.commonwl.org/v1.0/Workflow.html#Subworkflows)
refers to a tool or subworkflow by specifying 
the relative path of the corresponding `.cwl` file in the `run` field,
wftool will serialize all 10 `.cwl` files into a single `.json` file.


wftool also performs some basic validation
to let you know if there are any errors in the CWL that would prevent
a workflow engine from successfully running the workflow.
Presently this validation is coarse,
so there may be other issues with the CWL
which may cause it to fail at runtime
which do not get caught by the wftool validator.

## Installing wftool

wftool is written in [Go](https://golang.org/). If you don't already have Go on your machine,
check out this [installation guide](https://golang.org/doc/install).

Once you have Go on your machine, 
run this command at the commandline:  
`go get github.com/uc-cdis/mariner/wftool`

Now wftool is installed!  

If bash doesn't recognize wftool at the commandline,
it may be that the path to your go bin is not part of your `$PATH`.
See [How To Write Go Code](https://golang.org/doc/code.html)
for more information.


## Usage

- `-pack`: pass this flag to serialize a CWL workflow to JSON
- `-validate`: pass this flag to validate a workflow JSON
- `-i`: path to workflow (i.e., path to a `.cwl` file for `-pack` or `.json` file for `-validate`)
- `-o`: output path for `-pack`

### Notes

Pass exactly one of the `-pack` or `-validate` flags  
Always specify input path with `-i`  
If you pass `-pack`, specify the output path with `-o`  

### Example Usage

#### Pack

`wftool -pack -i myWorkflow.cwl -o wf.json`

#### Validate

`wftool -validate -i wf.json`

