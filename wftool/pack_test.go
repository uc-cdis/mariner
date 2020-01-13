package wftool

import (
	"testing"
)

var tool = `
#!/usr/bin/env cwl-runner

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

var workflow = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: Workflow

requirements:
  - class: InlineJavascriptRequirement

inputs:
    commons_file_1: File
    commons_file_2: File
    user_file: File

outputs:
    output_files:
        type: File[]
        outputSource: read_from_all/output_files

steps:
    read_from_commons_and_user:
        run: ./read_from_commons_and_user.cwl
        in:
            commons_file_1: commons_file_1
            commons_file_2: commons_file_2
            user_file: user_file
        out: [processed_file_1, processed_file_2, processed_file_3]

    read_from_all:
        run: ./read_from_all.cwl
        in:
            processed_file_1: 
                source: read_from_commons_and_user/processed_file_1
            processed_file_2: 
                source: read_from_commons_and_user/processed_file_2
            processed_file_3:
                source: read_from_commons_and_user/processed_file_3
            unprocessed_file_1: commons_file_1
            unprocessed_file_2: commons_file_2
            unprocessed_file_3: user_file
        out: [ output_files ]
`

func TestPack(t *testing.T) {
	Pack([]byte(tool))
	Pack([]byte(workflow))
}
