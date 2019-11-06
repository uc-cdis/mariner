#!/bin/sh

while [[ ! -f /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/request.json ]]; do
    echo "Waiting for mariner-engine-sidecar to finish setting up..";
    sleep 1
done
    echo "Sidecar setup complete! Running mariner-engine now.."
    /mariner run $RUN_ID