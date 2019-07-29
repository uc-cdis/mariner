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
