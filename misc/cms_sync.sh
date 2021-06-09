#!/usr/bin/env bash

set +e
set -x

BASE_DIR="/sites/archive-backend"
TIMESTAMP="$(date '+%Y%m%d%H%M%S')"
LOG_FILE="$BASE_DIR/logs/cms/sync_$TIMESTAMP.log"


cd ${BASE_DIR}

./archive-backend cms >> ${LOG_FILE} 2>&1

WARNINGS="$(egrep -c "level=(warning|error)" ${LOG_FILE})"

if [ "$WARNINGS" = "0" ];then
        echo "No warnings"
else
	echo "Errors in cms sync" | mail -s "ERROR: CMS sync" -r "mdb@bbdomain.org" -a ${LOG_FILE} edoshor@gmail.com
fi


find ${BASE_DIR}/logs/cms -name "sync_*.log" -type f -mmin +60 -exec rm -f {} \;
find /sites/assets/cms -mmin +60 -exec rm -rf {} \;
