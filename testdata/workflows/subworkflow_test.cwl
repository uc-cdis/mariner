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
        run: ../tools/initdir_test.cwl
        in:
            input_bam: input_bam
        out: [ bam_with_index ]
    
    test_expr:
        run: ../tools/expressiontool_test.cwl
        in:
            file_array:
                source: test_initworkdir/bam_with_index
                valueFrom: $([self, self.secondaryFiles[0]])
        out: [ output ]