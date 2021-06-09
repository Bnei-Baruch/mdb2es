#!/usr/bin/env sh

set +e
set -x

BASE_DIR="/app"
TIMESTAMP="$(date '+%Y%m%d%H%M%S')"
LOG_FILE="/tmp/cms_sync_$TIMESTAMP.log"

cleanup() {
  find /tmp -name "cms_sync_*.log" -type f -mmin +60 -exec rm -f {} \;
  find /assets/cms -mmin +60 -exec rm -rf {} \;
}

cd ${BASE_DIR} &&
  ./archive-backend cms >>"${LOG_FILE}" 2>&1

WARNINGS="$(grep -Ec "level=(warning|error)" ${LOG_FILE})"
if [ "$WARNINGS" = "0" ]; then
  echo "No warnings"
  cleanup
  exit 0
fi

echo "Errors in cms sync" | mail -s "ERROR: CMS sync" edoshor@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE}"
cleanup
exit 1
