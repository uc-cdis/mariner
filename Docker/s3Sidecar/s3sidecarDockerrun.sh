#!/bin/sh

# NOTE: configuring the AWS CLI this way, without setting envVars, gives errors and doesn't work
aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id | tr -d '"')
aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret | tr -d '"')

# so we set these variables to allow the AWS CLI to work
export AWS_ACCESS_KEY_ID=$(echo $AWSCREDS | jq .id | tr -d '"')
export AWS_SECRET_ACCESS_KEY=$(echo $AWSCREDS | jq .secret | tr -d '"')

# mount bucket at userID prefix to dir /engine-workspace
/goofys --stat-cache-ttl 0 --type-cache-ttl 0 $S3_BUCKET_NAME:$USER_ID /$ENGINE_WORKSPACE

# optionally mount bucket at conformanceTest prefix for conformance testing
if [ $CONFORMANCE_TEST == "true" ]; then
  /goofys --stat-cache-ttl 0 --type-cache-ttl 0 $S3_BUCKET_NAME:$CONFORMANCE_INPUT_S3_PREFIX /$CONFORMANCE_INPUT_DIR
fi


if [ $MARINER_COMPONENT == "engine" ]; then
  echo "setting up for the engine.."

  # create working dir for workflow run
  mkdir -p  /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID

  # pass workflow request to engine (Q. why this design decision?)
  echo $WORKFLOW_REQUEST > /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/request.json

  # wait for engine process to finish
  echo "waiting for workflow to finish.."
  sleep 10
  while [[ ! -f /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/done ]]; do
    :
  done
else # $MARINER_COMPONENT is "task"
  echo "setting up for a task.."

  # create working dir for tool
  mkdir -p $TOOL_WORKING_DIR

  # pass tool command to tool container (Q. again, why this design?)
  echo $TOOL_COMMAND > $TOOL_WORKING_DIR\run.sh 
  
  # wait for tool process to finish
  echo "waiting for commandlinetool to finish.."
  ls $TOOL_WORKING_DIR
  sleep 10
  while [[ ! -f $TOOL_WORKING_DIR\done ]]; do
    :
  done
fi

# unmount engine workspace
echo "done, unmounting goofys"

fusermount -u -z /$ENGINE_WORKSPACE
fusermount -u -z /$CONFORMANCE_INPUT_DIR

echo "goofys exited successfully"
