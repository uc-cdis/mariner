#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: CommandLineTool

requirements:
  - class: InlineJavascriptRequirement
  - class: ShellCommandRequirement
  - class: InitialWorkDirRequirement
    listing:
      - entryname: $(runtime.outdir + 'touchFiles.sh')
        entry: |
          #!/bin/sh
          cat $(inputs.unprocessed_file_1.location) > $(runtime.outdir + 'unprocessed_file_1.txt')
          cat $(inputs.unprocessed_file_2.location) > $(runtime.outdir + 'unprocessed_file_2.txt')
          cat $(inputs.unprocessed_file_3.location) > $(runtime.outdir + 'unprocessed_file_3.txt')
          echo 'NOTE this commons_file_1 was processed in step 2' | cat $(inputs.processed_file_1.location) - > $(runtime.outdir + 'final_processed_file_1.txt')
          echo 'NOTE this commons_file_2 was processed in step 2' | cat $(inputs.processed_file_2.location) - > $(runtime.outdir + 'final_processed_file_2.txt')
          echo 'NOTE this user_file was processed in step 2' | cat $(inputs.processed_file_3.location) - > $(runtime.outdir + 'final_processed_file_3.txt')

          
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
        - $(runtime.outdir + 'final_processed*')
        - $(runtime.outdir + 'unprocessed*')

baseCommand: ['/bin/sh']

arguments:
  - position: 1
    valueFrom: $(runtime.outdir + 'touchFiles.sh')
