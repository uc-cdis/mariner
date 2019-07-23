
# common to engine and task ->
/root/bin/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id)
/root/bin/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret)
goofys workflow-engine-garvin:$S3PREFIX /data
# <- common to engine and task

# conditional here
if [ $MARINER_COMPONENT == "ENGINE" ]; then
  echo "setting up for the engine.."
  echo $WORKFLOW_REQUEST > /data/request.json
  echo "successfully wrote workflow request to /data/request.json"
else # $MARINER_COMPONENT is "TASK"
  echo "setting up for a task.."
  echo '$TOOL_COMMAND' > $TOOL_WORKING_DIR\run.sh
  echo "successfully wrote tool command to $TOOL_WORKING_DIR\run.sh"
fi

while true; do
  echo "staying alive"
  sleep 10
done
