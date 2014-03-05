#!/bin/bash -e

RUN_DIR=/var/vcap/sys/run/web
LOG_DIR=/var/vcap/sys/log/web
DATA_DIR=/var/vcap/data/web
STORE_DIR=/var/vcap/store/web
PIDFILE=$RUN_DIR/drone_wall.pid

WALL_PKG=/var/vcap/packages/drone_wall

source /var/vcap/packages/pid_utils/pid_utils.sh

case $1 in

  start)
    pid_guard $PIDFILE "drone_wall"

    mkdir -p $RUN_DIR
    chown -R vcap:vcap $RUN_DIR

    mkdir -p $LOG_DIR
    chown -R vcap:vcap $RUN_DIR

    mkdir -p $DATA_DIR
    chown -R vcap:vcap $DATA_DIR

    mkdir -p $STORE_DIR
    chown -R vcap:vcap $STORE_DIR

    echo $$ > $PIDFILE

    export PATH=$WALL_PKG/bin:$PATH

    exec chpst -u vcap:vcap $WALL_PKG/bin/drone-wall \
      -port=:8080 \
      -datasource=$STORE_DIR/droned.sqlite \
      -repos=<%= p("drone_wall.repos").join(",") %> \
      1>>$LOG_DIR/drone_wall.stdout.log \
      2>>$LOG_DIR/drone_wall.stderr.log

    ;;

  stop)
    kill_and_wait $PIDFILE

    ;;

  *)
    echo "Usage: wall_ctl {start|stop}"

    ;;

esac
