

echo "mychan:long process is done" | nc -N ntfy.sh 9999
curl -d "long process is done" ntfy.sh/mychan
    publish on channel

curl ntfy.sh/mychan
    subscribe to channel

ntfy.sh/mychan/ws
