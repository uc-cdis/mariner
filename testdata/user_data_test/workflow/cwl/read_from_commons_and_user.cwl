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
          echo 'NOTE this commons_file_1 was processed in step 1' | cat $(inputs.commons_file_1.location) - > processed_file_1.txt
          echo 'NOTE this commons_file_2 was processed in step 1' | cat $(inputs.commons_file_2.location) - > processed_file_2.txt
          echo 'NOTE this user_file was processed in step 1' | cat $(inputs.user_file.location) - > processed_file_3.txt
          

inputs:
  commons_file_1:
    type: File
  commons_file_2:
    type: File
  user_file:
    type: File

outputs:
  processed_file_1:
    type: File
    outputBinding:
      glob: 'processed_file_1*'
  processed_file_2:
    type: File
    outputBinding:
      glob: 'processed_file_2*'
  processed_file_3:
    type: File 
    outputBinding:
      glob: 'processed_file_3*'

baseCommand: ['/bin/sh']

arguments:
  - position: 1
    valueFrom: 'touchFiles.sh'
