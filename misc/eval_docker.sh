#!/usr/bin/env sh

set +e
set -x

BASE_DIR="/app"
DATA_DIR="$BASE_DIR/data"
RECALL_SET_DIR="${DATA_DIR}/search"
GOLDEN_DIR="${DATA_DIR}/search/golden"
LOGS_DIR="/tmp"
BACKEND="http://nginx/backend"

TIMESTAMP="$(date '+%Y%m%d%H%M%S')"
LOG_FILE_HE="${LOGS_DIR}/eval_he_${TIMESTAMP}.log"
LOG_FILE_RU="${LOGS_DIR}/eval_ru_${TIMESTAMP}.log"
LOG_FILE_EN="${LOGS_DIR}/eval_en_${TIMESTAMP}.log"
LOG_FILE_HTML="${LOGS_DIR}/eval_html_${TIMESTAMP}.log"
REPORT_FILE_HE="${LOGS_DIR}/eval_he_${TIMESTAMP}.flat.report"
REPORT_FILE_RU="${LOGS_DIR}/eval_ru_${TIMESTAMP}.flat.report"
REPORT_FILE_EN="${LOGS_DIR}/eval_en_${TIMESTAMP}.flat.report"
HTML_FILE="${LOGS_DIR}/eval_report_${TIMESTAMP}.html"
GOLDEN_REPORT_FILE_HE="$(ls ${GOLDEN_DIR}/eval_he_*.flat.report)"
GOLDEN_REPORT_FILE_RU="$(ls ${GOLDEN_DIR}/eval_ru_*.flat.report)"
GOLDEN_REPORT_FILE_EN="$(ls ${GOLDEN_DIR}/eval_en_*.flat.report)"

cd ${BASE_DIR}

./archive-backend eval --server=${BACKEND} --eval_set=${RECALL_SET_DIR}/he.recall.csv --flat_report="${REPORT_FILE_HE}" >>"${LOG_FILE_HE}" 2>&1
if [ $? -ne 0 ]; then
  mail -s "Daily Eval: Error." kolmanv@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE_HE}"
fi
./archive-backend eval --server=${BACKEND} --eval_set=${RECALL_SET_DIR}/ru.recall.csv --flat_report="${REPORT_FILE_RU}" >>"${LOG_FILE_RU}" 2>&1
if [ $? -ne 0 ]; then
  mail -s "Daily Eval: Error." kolmanv@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE_RU}"
fi
./archive-backend eval --server=${BACKEND} --eval_set=${RECALL_SET_DIR}/en.recall.csv --flat_report="${REPORT_FILE_EN}" >>"${LOG_FILE_EN}" 2>&1
if [ $? -ne 0 ]; then
  mail -s "Daily Eval: Error." kolmanv@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE_EN}"
fi

./archive-backend vs_golden_html \
  --flat_reports="${REPORT_FILE_HE},${REPORT_FILE_EN},${REPORT_FILE_RU}" \
  --golden_flat_reports="${GOLDEN_REPORT_FILE_HE},${GOLDEN_REPORT_FILE_RU},${GOLDEN_REPORT_FILE_EN}" \
  --vs_golden_html="${HTML_FILE}" >>"${LOG_FILE_HTML}" 2>&1

if [ $? -ne 0 ]; then
  mail -s "Daily Eval: Error." kolmanv@gmail.com -- -r "mdb@bbdomain.org" -a "${LOG_FILE_HTML}"
fi

# Cleanup old logs (older then week).
find ${LOGS_DIR} -name "eval_he_*.log" -type f -mtime +7 -exec rm -f {} \;
find ${LOGS_DIR} -name "eval_ru_*.log" -type f -mtime +7 -exec rm -f {} \;
find ${LOGS_DIR} -name "eval_en_*.log" -type f -mtime +7 -exec rm -f {} \;
find ${LOGS_DIR} -name "eval_report_*.log" -type f -mtime +7 -exec rm -f {} \;

mail -s "Daily Eval: Done." kolmanv@gmail.com edoshor@gmail.com eranminuchin@gmail.com yurihechter@gmail.com alex.mizrachi@gmail.com -- \
  -r "mdb@bbdomain.org" \
  -a "${REPORT_FILE_HE}" \
  -a "${REPORT_FILE_RU}" \
  -a "${REPORT_FILE_EN}" \
  -a "${LOG_FILE_HE}" \
  -a "${LOG_FILE_RU}" \
  -a "${LOG_FILE_EN}" \
  -a "${LOG_FILE_HTML}" <"${HTML_FILE}"

exit 0
