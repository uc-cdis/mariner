#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement

inputs:
  commons_file_1:
    type: File
  commons_file_2:
    type: File

outputs:
  processed_file_1:
    type: File
    outputBinding:
      glob: 'processed_file_1'
  processed_file_2:
    type: File
    outputBinding:
      glob: 'processed_file_2'

baseCommand: []

arguments:
  - position: 0
    valueFrom: >-
      cat $(inputs.commons_file_1.location) > processed_file_1.txt \
      echo 'NOTE this commons_file_1 was processed in step 1' >> processed_file_1.txt \
      cat $(inputs.commons_file_2.location) > processed_file_2.txt \
      echo 'NOTE this commons_file_2 was processed in step 1' >> processed_file_2.txt
