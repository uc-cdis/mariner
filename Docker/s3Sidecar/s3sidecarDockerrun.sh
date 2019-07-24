
# common to engine and task ->

# NOTE: configuring the AWS CLI this way, without setting envVars, gives errors and doesn't work
/root/bin/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id | tr -d '"')
/root/bin/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret | tr -d '"')

# so we set these variables to allow the AWS CLI to work
export AWS_ACCESS_KEY_ID=$(echo $AWSCREDS | jq .id | tr -d '"')
export AWS_SECRET_ACCESS_KEY=$(echo $AWSCREDS | jq .secret | tr -d '"')

echo "mounting prefix $S3PREFIX"
goofys -f workflow-engine-garvin:$S3PREFIX /data 2>&1
# <- common to engine and task

# conditional here
if [ $MARINER_COMPONENT == "ENGINE" ]; then
  echo "setting up for the engine.."
  echo $WORKFLOW_REQUEST > /data/request.json
  echo "successfully wrote workflow request to /data/request.json"
else # $MARINER_COMPONENT is "TASK"
  echo "setting up for a task.."
  echo $TOOL_COMMAND > $TOOL_WORKING_DIR\run.sh # this might be a problematic way of writing/passing the command - quotations and spaces/breaks preserved or not, etc
  echo "successfully wrote tool command to $TOOL_WORKING_DIR\run.sh"
fi

while true; do
  echo "staying alive"
  sleep 10
done
