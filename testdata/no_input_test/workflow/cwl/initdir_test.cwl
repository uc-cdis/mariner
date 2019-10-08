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
