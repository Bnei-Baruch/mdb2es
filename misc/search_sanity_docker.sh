#!/usr/bin/env sh

set +e
set -x

BASE_DIR="/app"
DATA_DIR="$BASE_DIR/data"
TIMESTAMP="$(date '+%Y%m%d%H%M%S')"
LOG_FILE="/tmp/eval_sanity_$TIMESTAMP.log"
FLAT_REPORT_FILE="/tmp/eval_sanity_$TIMESTAMP.flat.report"

cleanup() {
  find /tmp -name "eval_sanity_*" -type f -mtime +1 -exec rm -f {} \;
}

cd ${BASE_DIR} &&
  ./archive-backend eval --server=http://nginx/backend --flat_report="${FLAT_REPORT_FILE}" --eval_set="$DATA_DIR/search/sanity.csv" >> "${LOG_FILE}" 2>&1

SANITY_OK="$(grep -Ec "Good.*100.00%" ${LOG_FILE})"
if [ "${SANITY_OK}" = "1" ]; then
  echo "Sanity OK."
  cleanup
  exit 0
fi

echo "Sanity failed." | mail -s "ERROR: Search sanity." edoshor@gmail.com kolmanv@gmail.com yurihechter@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE}" -a "${FLAT_REPORT_FILE}"
cleanup
exit 1
