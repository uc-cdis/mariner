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
    ramMin: 100MB

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
