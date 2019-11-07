
# common to engine and task ->

# NOTE: configuring the AWS CLI this way, without setting envVars, gives errors and doesn't work
/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id | tr -d '"')
/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret | tr -d '"')

# so we set these variables to allow the AWS CLI to work
export AWS_ACCESS_KEY_ID=$(echo $AWSCREDS | jq .id | tr -d '"')
export AWS_SECRET_ACCESS_KEY=$(echo $AWSCREDS | jq .secret | tr -d '"')

# FIXME this script in general - don't split out common execution flows

# conditional here
if [ $MARINER_COMPONENT == "engine" ]; then
  echo "setting up for the engine.."
  echo "mounting prefix $USER_ID"
  /goofys --stat-cache-ttl 0 --type-cache-ttl 0 $S3_BUCKET_NAME:$USER_ID /$ENGINE_WORKSPACE
  # make this path handling cleaner -> put 'workflowRuns/' in the envVar
  mkdir -p  /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID
  echo $WORKFLOW_REQUEST > /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/request.json
  echo "successfully wrote workflow request to /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/request.json"
  echo "here is the workflow:"
  echo $WORKFLOW_REQUEST | jq 
  echo "waiting for workflow to finish.."
  sleep 10
  while [[ ! -f /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/done ]]; do
    :
  done
else # $MARINER_COMPONENT is "task"
  echo "setting up for a task.."
  echo "mounting prefix $USER_ID"
  /goofys --stat-cache-ttl 0 --type-cache-ttl 0 $S3_BUCKET_NAME:$USER_ID /$ENGINE_WORKSPACE
  echo "here is /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID:"
  ls -R /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID
  echo "creating working dir for tool.."
  # fix - working dir is userID/workflowRuns/runID/taskID
  mkdir -p $TOOL_WORKING_DIR
  echo "writing command to workdir.."
  echo $TOOL_COMMAND > $TOOL_WORKING_DIR\run.sh # this might be a problematic way of writing/passing the command - quotations and spaces/breaks preserved or not, etc
  echo "successfully wrote tool command to $TOOL_WORKING_DIR\run.sh"
  echo "waiting for commandlinetool to finish.."
  ls $TOOL_WORKING_DIR
  sleep 10
  while [[ ! -f $TOOL_WORKING_DIR\done ]]; do
    :
  done
fi

# unmount
echo "done, unmounting goofys"

fusermount -u -z /$ENGINE_WORKSPACE

echo "goofys exited successfully"

# while true; do
#   echo "staying alive"
#   sleep 10
# done
