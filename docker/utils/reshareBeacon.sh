#bin/sh

# README - Previous tasks
# 1) Run dkg first with startBeacon.sh script
# 2) Copy group file .drand/default/groups/drand_group.toml from node 0 to node 4 volume
# 2) Copy group file .drand/test_beacon/groups/drand_group.toml from node 0 to node 4 volume
# 3) You are ready to run this script

# Start leader
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --transition --leader --nodes 5 --threshold 4 --period "5s"' &

sleep 3s

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable' &
nohup docker exec -u drand drand_4 /bin/sh -c 'drand share --connect drand_0:8080 --from ./data/drand/.drand/default/groups/drand_group.toml --tls-disable' &

sleep 10s

# Start leader
nohup docker exec -u drand drand_0 /bin/sh -c 'drand share --transition --leader --nodes 5 --threshold 4 --period "60s" --id test_beacon' &

sleep 3s

# Start the rest of the nodes
nohup docker exec -u drand drand_2 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable --id test_beacon' &
nohup docker exec -u drand drand_1 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable --id test_beacon' &
nohup docker exec -u drand drand_3 /bin/sh -c 'drand share --transition  --connect drand_0:8080 --tls-disable --id test_beacon' &
nohup docker exec -u drand drand_4 /bin/sh -c 'drand share --connect drand_0:8080 --from ./data/drand/.drand/test_beacon/groups/drand_group.toml --tls-disable --id test_beacon' &