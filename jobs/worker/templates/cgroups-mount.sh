#!/bin/bash

set -e

if grep -q /sys/fs/cgroup /proc/mounts; then
  exit 0
fi

if ! mountpoint -q /sys/fs/cgroup; then
  mount -t tmpfs -o uid=0,gid=0,mode=0755 cgroup /sys/fs/cgroup
fi

for d in $(tail -n +2 /proc/cgroups | awk '{print $1}'); do
  mkdir -p /sys/fs/cgroup/$d

  if ! mountpoint -q /sys/fs/cgroup/$d; then
    mount -n -t cgroup -o $d cgroup /sys/fs/cgroup/$d
  fi
done

exit 0
