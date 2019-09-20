
# common to engine and task ->

# NOTE: configuring the AWS CLI this way, without setting envVars, gives errors and doesn't work
/root/bin/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id | tr -d '"')
/root/bin/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret | tr -d '"')

# so we set these variables to allow the AWS CLI to work
export AWS_ACCESS_KEY_ID=$(echo $AWSCREDS | jq .id | tr -d '"')
export AWS_SECRET_ACCESS_KEY=$(echo $AWSCREDS | jq .secret | tr -d '"')

# echo "mounting prefix $S3PREFIX"
# goofys workflow-engine-garvin:$S3PREFIX /engine-workspace
# <- common to engine and task

# conditional here
if [ $MARINER_COMPONENT == "ENGINE" ]; then
  echo "setting up for the engine.."
  echo "mounting prefix $S3PREFIX"
  goofys workflow-engine-garvin:$S3PREFIX /$ENGINE_WORKSPACE
  echo $WORKFLOW_REQUEST > /$ENGINE_WORKSPACE/request.json
  echo "successfully wrote workflow request to /$ENGINE_WORKSPACE/request.json"
  echo "waiting for workflow to finish.."
  sleep 10
  while [[ ! -f /$ENGINE_WORKSPACE/done ]]; do
    :
  done
else # $MARINER_COMPONENT is "TASK"
  echo "setting up for a task.."
  echo "mounting prefix $S3PREFIX"
  echo "here is /$ENGINE_WORKSPACE:"
  ls -R /$ENGINE_WORKSPACE
  echo "mounting bucket.."
  goofys workflow-engine-garvin:$S3PREFIX /$ENGINE_WORKSPACE
  echo "creating working dir for tool.."
  mkdir -p $S3PREFIX
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
fusermount -u /$ENGINE_WORKSPACE

# while true; do
#   echo "staying alive"
#   sleep 10
# done
