echo creating service..
kubectl apply -f mariner-service.yaml

echo creating serviceaccount..
kubectl apply -f mariner-service-account.yaml

echo creating rolebinding..
kubectl apply -f mariner-binding.yaml

echo deploying mariner-server..
kubectl apply -f mariner-deploy.yaml

echo successfully deployed mariner-server
