#!/bin/bash -e

RUN_DIR=/var/vcap/sys/run/web
LOG_DIR=/var/vcap/sys/log/web
DATA_DIR=/var/vcap/data/web
STORE_DIR=/var/vcap/store/web
PIDFILE=$RUN_DIR/web.pid

DRONE_PKG=/var/vcap/packages/drone

source /var/vcap/packages/pid_utils/pid_utils.sh

case $1 in

  start)
    pid_guard $PIDFILE "web"

    mkdir -p $RUN_DIR
    chown -R vcap:vcap $RUN_DIR
    
    mkdir -p $LOG_DIR
    chown -R vcap:vcap $RUN_DIR

    mkdir -p $DATA_DIR
    chown -R vcap:vcap $DATA_DIR

    mkdir -p $STORE_DIR
    chown -R vcap:vcap $STORE_DIR

    echo $$ > /var/vcap/sys/run/web/web.pid

    export PATH=$DRONE_PKG/bin:$PATH

    setcap cap_net_bind_service=+ep $DRONE_PKG/bin/droned

    exec chpst -u vcap:vcap $DRONE_PKG/bin/droned \
      -port=:80 \
      -datasource=$STORE_DIR/droned.sqlite \
      1>>$LOG_DIR/web.stdout.log \
      2>>$LOG_DIR/web.stderr.log

    ;;

  stop)
    kill_and_wait $PIDFILE

    ;;

  *)
    echo "Usage: ctl {start|stop}"

    ;;

esac
