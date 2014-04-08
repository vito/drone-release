#!/bin/bash -e

RUN_DIR=/var/vcap/sys/run/worker
LOG_DIR=/var/vcap/sys/log/worker
PIDFILE=$RUN_DIR/worker.pid

DOCKER_PKG=/var/vcap/packages/docker_0.8.0
DOCKER_DATA_DIR=/var/vcap/data/docker

LXC_PKG=/var/vcap/packages/lxc_0.9.0
LXC_DATA_DIR=/var/vcap/data/lxc

source /var/vcap/packages/pid_utils/pid_utils.sh

case $1 in

  start)
    pid_guard $PIDFILE "worker"

    mkdir -p $RUN_DIR
    chown -R vcap:vcap $RUN_DIR

    mkdir -p $LOG_DIR
    chown -R vcap:vcap $RUN_DIR

    mkdir -p $DOCKER_DATA_DIR
    mkdir -p $LXC_DATA_DIR

    dpkg -i $DOCKER_PKG/aufs-tools_20110410-1_amd64.deb

    # mount cgroups
    $(dirname $0)/cgroups-mount

    # the sudo is, surprisingly, necessary (but not on 13.10's kernel)
    exec sudo PATH=$LXC_PKG/bin:$PATH $DOCKER_PKG/bin/docker -d \
      -H tcp://0.0.0.0:4243 \
      -p $RUN_DIR/worker.pid \
      -g $DOCKER_DATA_DIR \
      -mtu 1500 \
      1>>$LOG_DIR/worker.stdout.log \
      2>>$LOG_DIR/worker.stderr.log

    ;;

  stop)
    kill_and_wait $PIDFILE

    ;;

  *)
    echo "Usage: ctl {start|stop}"

    ;;

esac
