#!/bin/bash
sudo killall p2p-node 2>/dev/null
sleep 1

# Start Node A (Bootstrap)
sudo ./release/cli/p2p-node-darwin-arm64 -port 6001 -config ./scratch/nodeA > ./scratch/nodeA.log 2>&1 &
NODE_A_PID=$!
echo "Node A started ($NODE_A_PID)"
sleep 3

NODE_A_IP=$(grep "Virtual IP" ./scratch/nodeA/address.txt | awk '{print $3}')
NODE_A_ID=$(grep "/ip4/127.0.0.1/tcp/6001/p2p/" ./scratch/nodeA/address.txt | head -n 1)

# Start Node B (Client)
sudo ./release/cli/p2p-node-darwin-arm64 -port 6002 -config ./scratch/nodeB -bootstrap $NODE_A_ID > ./scratch/nodeB.log 2>&1 &
NODE_B_PID=$!
echo "Node B started ($NODE_B_PID)"
sleep 10

NODE_B_IP=$(grep "Virtual IP" ./scratch/nodeB/address.txt | awk '{print $3}')

echo "Node A IP: $NODE_A_IP"
echo "Node B IP: $NODE_B_IP"

echo "Node A logs:"
cat ./scratch/nodeA.log
echo "-----------------------------------"
echo "Node B logs:"
cat ./scratch/nodeB.log
echo "-----------------------------------"

echo "Pinging Node A ($NODE_A_IP) from Node B ($NODE_B_IP)"
# To force ping out of a specific interface, ping -b doesn't work like that, but since both are in 10.0.0.0/8, 
# the kernel routing table will just use the first interface or the most specific route.
# Actually on macOS, if there are two utun interfaces with 10.0.0.0/8 routes, it might be tricky.
# Let's ping anyway.
ping -c 3 $NODE_A_IP
PING_EXIT=$?

sudo kill $NODE_A_PID
sudo kill $NODE_B_PID

exit $PING_EXIT
