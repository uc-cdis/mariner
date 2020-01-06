package wftool

import (
	"testing"
)

var input = `#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement
  - class: InitialWorkDirRequirement
    listing:
      - entryname: 'touchFiles.sh'
        entry: |
          #!/bin/sh
          cat $(inputs.unprocessed_file_1.location) > unprocessed_file_1.txt
          cat $(inputs.unprocessed_file_2.location) > unprocessed_file_2.txt
          cat $(inputs.unprocessed_file_3.location) > unprocessed_file_3.txt
          echo 'NOTE this commons_file_1 was processed in step 2' | cat $(inputs.processed_file_1.location) - > final_processed_file_1.txt
          echo 'NOTE this commons_file_2 was processed in step 2' | cat $(inputs.processed_file_2.location) - > final_processed_file_2.txt
          echo 'NOTE this user_file was processed in step 2' | cat $(inputs.processed_file_3.location) - > final_processed_file_3.txt

          
inputs:
  processed_file_1: File
  processed_file_2: File
  processed_file_3: File
  unprocessed_file_1: File
  unprocessed_file_2: File
  unprocessed_file_3: File


outputs:
  output_files:
    type: File[]
    outputBinding:
      loadContents: true
      glob:
        - 'final_processed*'
        - 'unprocessed*'

baseCommand: ['/bin/sh']

arguments:
  - position: 1
    valueFrom: 'touchFiles.sh'
`

var desiredOutput = `{
	"class": "CommandLineTool",
	"requirements": [
		{
			"class": "InlineJavascriptRequirement"
		},
		{
			"class": "ShellCommandRequirement"
		},
		{
			"class": "InitialWorkDirRequirement",
			"listing": [
				{
					"entryname": "touchFiles.sh",
					"entry": "#!/bin/sh\ncat $(inputs.unprocessed_file_1.location) > unprocessed_file_1.txt\ncat $(inputs.unprocessed_file_2.location) > unprocessed_file_2.txt\ncat $(inputs.unprocessed_file_3.location) > unprocessed_file_3.txt\necho 'NOTE this commons_file_1 was processed in step 2' | cat $(inputs.processed_file_1.location) - > final_processed_file_1.txt\necho 'NOTE this commons_file_2 was processed in step 2' | cat $(inputs.processed_file_2.location) - > final_processed_file_2.txt\necho 'NOTE this user_file was processed in step 2' | cat $(inputs.processed_file_3.location) - > final_processed_file_3.txt\n"
				}
			]
		}
	],
	"inputs": [
		{
			"type": "File",
			"id": "#read_from_all.cwl/processed_file_1"
		},
		{
			"type": "File",
			"id": "#read_from_all.cwl/processed_file_2"
		},
		{
			"type": "File",
			"id": "#read_from_all.cwl/processed_file_3"
		},
		{
			"type": "File",
			"id": "#read_from_all.cwl/unprocessed_file_1"
		},
		{
			"type": "File",
			"id": "#read_from_all.cwl/unprocessed_file_2"
		},
		{
			"type": "File",
			"id": "#read_from_all.cwl/unprocessed_file_3"
		}
	],
	"outputs": [
		{
			"id": "#read_from_all.cwl/output_files",
			"type": "File[]",
			"outputBinding": {
				"loadContents": true,
				"glob": [
					"final_processed*",
					"unprocessed*"
				]
			}
		}
	],
	"baseCommand": [
		"/bin/sh"
	],
	"arguments": [
		{
			"position": 1,
			"valueFrom": "touchFiles.sh"
		}
	],
	"id": "#read_from_all.cwl"
}`

func TestPack(t *testing.T) {
	Pack([]byte(input))
}
