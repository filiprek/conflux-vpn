#!/bin/sh
# Run only inside the disposable network namespace (relay or client).
set -eu
role=$1
tc qdisc add dev eth0 root handle 1: htb default 30
tc class add dev eth0 parent 1: classid 1:1 htb rate 1000mbit
tc class add dev eth0 parent 1:1 classid 1:10 htb rate 2mbit ceil 2mbit
tc class add dev eth0 parent 1:1 classid 1:20 htb rate 2mbit ceil 2mbit
tc class add dev eth0 parent 1:1 classid 1:30 htb rate 996mbit ceil 1000mbit
tc qdisc add dev eth0 parent 1:10 handle 10: fq_codel limit 128
tc qdisc add dev eth0 parent 1:20 handle 20: fq_codel limit 128
field=dport
[ "$role" != relay ] || field=sport
tc filter add dev eth0 protocol ip parent 1: prio 1 u32 match ip protocol 6 0xff match ip "$field" 19841 0xffff flowid 1:10
tc filter add dev eth0 protocol ip parent 1: prio 1 u32 match ip protocol 6 0xff match ip "$field" 19842 0xffff flowid 1:20
