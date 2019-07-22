/root/bin/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id)
/root/bin/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret)
goofys workflow-engine-garvin:$S3PREFIX /data
echo $WORKFLOW_REQUEST > /data/request.json

while true; do 
 echo "staying alive"
 sleep 10
done
