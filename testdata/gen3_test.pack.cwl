{
    "cwlVersion": "v1.0", 
    "$graph": [
        {
            "inputs": [
                {
                    "type": "File", 
                    "id": "#main/input_bam"
                }
            ], 
            "requirements": [
                {
                    "class": "InlineJavascriptRequirement"
                }, 
                {
                    "class": "StepInputExpressionRequirement"
                }, 
                {
                    "class": "MultipleInputFeatureRequirement"
                }, 
                {
                    "class": "ScatterFeatureRequirement"
                }, 
                {
                    "class": "SubworkflowFeatureRequirement"
                }
            ], 
            "outputs": [
                {
                    "outputSource": "#main/test_scatter/output", 
                    "type": {
                        "items": "string", 
                        "type": "array"
                    }, 
                    "id": "#main/output"
                }
            ], 
            "class": "Workflow", 
            "steps": [
                {
                    "id": "#main/test_scatter", 
                    "out": [
                        "#main/test_scatter/output"
                    ], 
                    "run": "#scatter_test.cwl", 
                    "scatter": "#main/test_scatter/file", 
                    "in": [
                        {
                            "source": "#main/test_subworkflow/output_files", 
                            "id": "#main/test_scatter/file"
                        }
                    ]
                }, 
                {
                    "out": [
                        "#main/test_subworkflow/output_files"
                    ], 
                    "run": "#subworkflow_test.cwl", 
                    "id": "#main/test_subworkflow", 
                    "in": [
                        {
                            "source": "#main/input_bam", 
                            "id": "#main/test_subworkflow/input_bam"
                        }
                    ]
                }
            ], 
            "id": "#main"
        }, 
        {
            "inputs": [
                {
                    "type": {
                        "items": [
                            "null", 
                            "File"
                        ], 
                        "type": "array"
                    }, 
                    "id": "#expressiontool_test.cwl/file_array"
                }
            ], 
            "requirements": [
                {
                    "class": "InlineJavascriptRequirement"
                }
            ], 
            "outputs": [
                {
                    "type": {
                        "items": "File", 
                        "type": "array"
                    }, 
                    "id": "#expressiontool_test.cwl/output"
                }
            ], 
            "class": "ExpressionTool", 
            "expression": "${\n  var trueFile = [];\n  for (var i = 0; i < inputs.file_array.length; i++){\n    if (inputs.file_array[i] != null){\n      trueFile.push(inputs.file_array[i])\n    }\n  };\n  return {'output': trueFile};\n}\n", 
            "id": "#expressiontool_test.cwl"
        }, 
        {
            "inputs": [
                {
                    "inputBinding": {
                        "position": 1, 
                        "valueFrom": "$(self.basename)"
                    }, 
                    "type": "File", 
                    "id": "#initdir_test.cwl/input_bam"
                }
            ], 
            "requirements": [
                {
                    "class": "InlineJavascriptRequirement"
                }, 
                {
                    "class": "ShellCommandRequirement"
                }, 
                {
                    "dockerPull": "quay.io/cdis/samtools:dev_cloud_support", 
                    "class": "DockerRequirement"
                }, 
                {
                    "class": "InitialWorkDirRequirement", 
                    "listing": [
                        {
                            "entry": "$(inputs.input_bam)", 
                            "entryname": "$(inputs.input_bam.basename)"
                        }
                    ]
                }, 
                {
                    "coresMin": 1, 
                    "ramMin": "100MB", 
                    "class": "ResourceRequirement", 
                    "coresMax": 1
                }
            ], 
            "outputs": [
                {
                    "secondaryFiles": [
                        ".bai"
                    ], 
                    "outputBinding": {
                        "glob": "$(inputs.input_bam.basename)"
                    }, 
                    "type": "File", 
                    "id": "#initdir_test.cwl/bam_with_index"
                }
            ], 
            "baseCommand": [
                "samtools", 
                "index"
            ], 
            "class": "CommandLineTool", 
            "id": "#initdir_test.cwl"
        }, 
        {
            "inputs": [
                {
                    "type": "File", 
                    "id": "#scatter_test.cwl/file"
                }
            ], 
            "requirements": [
                {
                    "class": "InlineJavascriptRequirement"
                }, 
                {
                    "class": "ShellCommandRequirement"
                }, 
                {
                    "dockerPull": "alpine", 
                    "class": "DockerRequirement"
                }, 
                {
                    "coresMin": 1, 
                    "ramMin": "100MB", 
                    "class": "ResourceRequirement", 
                    "coresMax": 1
                }
            ], 
            "stdout": "file_md5", 
            "outputs": [
                {
                    "outputBinding": {
                        "glob": "file_md5", 
                        "loadContents": true, 
                        "outputEval": "${\n  var local_md5 = self[0].contents.trim().split(' ')[0]\n  return local_md5\n}\n"
                    }, 
                    "type": "string", 
                    "id": "#scatter_test.cwl/output"
                }
            ], 
            "baseCommand": [], 
            "id": "#scatter_test.cwl", 
            "arguments": [
                {
                    "shellQuote": false, 
                    "position": 0, 
                    "valueFrom": "md5sum $(inputs.file.path)"
                }
            ], 
            "class": "CommandLineTool"
        }, 
        {
            "inputs": [
                {
                    "type": "File", 
                    "id": "#subworkflow_test.cwl/input_bam"
                }
            ], 
            "requirements": [
                {
                    "class": "InlineJavascriptRequirement"
                }, 
                {
                    "class": "StepInputExpressionRequirement"
                }, 
                {
                    "class": "MultipleInputFeatureRequirement"
                }
            ], 
            "outputs": [
                {
                    "outputSource": "#subworkflow_test.cwl/test_expr/output", 
                    "type": {
                        "items": "File", 
                        "type": "array"
                    }, 
                    "id": "#subworkflow_test.cwl/output_files"
                }
            ], 
            "class": "Workflow", 
            "steps": [
                {
                    "out": [
                        "#subworkflow_test.cwl/test_expr/output"
                    ], 
                    "run": "#expressiontool_test.cwl", 
                    "id": "#subworkflow_test.cwl/test_expr", 
                    "in": [
                        {
                            "source": "#subworkflow_test.cwl/test_initworkdir/bam_with_index", 
                            "valueFrom": "$([self, self.secondaryFiles[0]])", 
                            "id": "#subworkflow_test.cwl/test_expr/file_array"
                        }
                    ]
                }, 
                {
                    "out": [
                        "#subworkflow_test.cwl/test_initworkdir/bam_with_index"
                    ], 
                    "run": "#initdir_test.cwl", 
                    "id": "#subworkflow_test.cwl/test_initworkdir", 
                    "in": [
                        {
                            "source": "#subworkflow_test.cwl/input_bam", 
                            "id": "#subworkflow_test.cwl/test_initworkdir/input_bam"
                        }
                    ]
                }
            ], 
            "id": "#subworkflow_test.cwl"
        }
    ]
}