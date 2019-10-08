#!/usr/bin/env cwl-runner

cwlVersion: v1.0

class: Workflow

requirements:
  - class: InlineJavascriptRequirement

inputs:
    commons_file_1: File
    commons_file_2: File

outputs:
    output_files:
        type: File[]
        outputSource: read_from_engine_workspace_and_commons/output_files

steps:
    read_from_commons:
        run: ./read_from_commons.cwl
        in:
            commons_file_1: commons_file_1
            commons_file_2: commons_file_2
        out: [processed_file_1, processed_file_2]

    read_from_engine_workspace_and_commons:
        run: ./read_from_engine_workspace_and_commons.cwl
        in:
            processed_file_1: 
                source: read_from_commons/processed_file_1
            processed_file_2: 
                source: read_from_commons/processed_file_2
            unprocessed_file_1: commons_file_1
            unprocessed_file_2: commons_file_2
        out: [ output_files ]
