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

var expressiontool = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

requirements:
  - class: InlineJavascriptRequirement

class: ExpressionTool

inputs:
  file_array:
    type:
      type: array
      items: ['null', 'File']

outputs:
  output: File[]

expression: |
  ${
    var trueFile = [];
    for (var i = 0; i < inputs.file_array.length; i++){
      if (inputs.file_array[i] != null){
        trueFile.push(inputs.file_array[i])
      }
    };
    return {'output': trueFile};
  }
`

var gen3test = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: Workflow

requirements:
  - class: InlineJavascriptRequirement
  - class: StepInputExpressionRequirement
  - class: MultipleInputFeatureRequirement
  - class: ScatterFeatureRequirement
  - class: SubworkflowFeatureRequirement

inputs:
    input_bam: File

outputs:
    output:
        type: string[]
        outputSource: test_scatter/output

steps:
    test_subworkflow:
        run: subworkflow_test.cwl
        in:
            input_bam: input_bam
        out: [ output_files ]

    test_scatter:
        run: scatter_test.cwl
        scatter: file
        in:
            file: test_subworkflow/output_files
        out: [ output ]

`

var initDir = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement
  - class: DockerRequirement
    dockerPull: quay.io/cdis/samtools:dev_cloud_support
  - class: InitialWorkDirRequirement
    listing:
      - entry: $(inputs.input_bam)
        entryname: $(inputs.input_bam.basename)
  - class: ResourceRequirement
    coresMin: 1
    coresMax: 1
    ramMin: 100

inputs:
  input_bam:
    type: File

outputs:
  bam_with_index:
    type: File
    outputBinding:
      glob: $(inputs.input_bam.basename)
    secondaryFiles:
      - '.bai'

baseCommand: ['touch']
arguments:
  - position: 0
    valueFrom: >-
      $(inputs.input_bam.basename + '.bai')
`

var scatter = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement
  - class: DockerRequirement
    dockerPull: alpine
  - class: ResourceRequirement
    coresMin: 1
    coresMax: 1
    ramMin: 100

inputs:
  file: File

stdout: file_md5
outputs:
  output:
    type: string
    outputBinding:
      glob: file_md5
      loadContents: true
      outputEval: |
        ${
          var local_md5 = self[0].contents.trim().split(' ')[0]
          return local_md5
        }

baseCommand: []
arguments:
  - position: 0
    shellQuote: false
    valueFrom: >-
      md5sum $(inputs.file.path)
`

var subwf = `
#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: Workflow

requirements:
  - class: InlineJavascriptRequirement
  - class: StepInputExpressionRequirement
  - class: MultipleInputFeatureRequirement

inputs:
    input_bam: File
outputs:
    output_files:
        type: File[]
        outputSource: test_expr/output
steps:
    test_initworkdir:
        run: initdir_test.cwl
        in:
            input_bam: input_bam
        out: [ bam_with_index ]

    test_expr:
        run: expressiontool_test.cwl
        in:
            file_array:
                source: test_initworkdir/bam_with_index
                valueFrom: $([self, self.secondaryFiles[0]])
        out: [ output ]
`

func TestPackCWL(t *testing.T) {
	// Pack([]byte(tool), "#read_from_all.cwl")
	// Pack([]byte(workflow), "#main")
	// Pack([]byte(expressiontool), "#expressiontool_test.cwl")
	// Pack([]byte(gen3test), "#main")
	// Pack([]byte(initDir), "#initdir_test.cwl")
	// Pack([]byte(scatter), "#scatter_test.cwl")
	PackCWL([]byte(subwf), "#subworkflow_test.cwl")
}
