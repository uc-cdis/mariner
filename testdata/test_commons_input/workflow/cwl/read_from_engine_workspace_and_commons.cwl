#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement

inputs:
  processed_file_1: File
  processed_file_2: File
  unprocessed_file_1: File
  unprocessed_file_2: File

outputs:
  output_files:
    type: File[]
    outputBinding:
      loadContents: true
      glob:
        - 'final_processed'
        - 'unprocessed'

baseCommand: []

arguments:
  - position: 0
    valueFrom: >-
      cat $(inputs.unprocessed_file_1.location) > unprocessed_file_1.txt \
      cat $(inputs.unprocessed_file_2.location) > unprocessed_file_2.txt \
      cat $(inputs.processed_file_1.location) > final_processed_file_1.txt \
      echo 'NOTE this commons_file_1 was processed in step 2' >> final_processed_file_1.txt \
      cat $(inputs.processed_file_2.location) > final_processed_file_2.txt \
      echo 'NOTE this commons_file_2 was processed in step 2' >> final_processed_file_2.txt
