#!/usr/bin/env bash

DEP_STATUS=0

checkStatus(){
    PHASE=$(kubectl get po crdb-0 -n threeport-control-plane --output=jsonpath='{.status.phase}')
    if [ "$PHASE" == "Running" ]; then
        DEP_STATUS=1
    fi
}

while [ $DEP_STATUS == 0 ] ;
do
    echo "Threeport API dependencies not yet ready..."
    sleep 10
    checkStatus
done

echo "Threeport API dependencies ready"

exit 0
