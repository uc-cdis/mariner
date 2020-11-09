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
