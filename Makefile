TS := $(shell date '+%Y%m%d_%H%M%S')
EXPORT_FILE := reports/report_$(TS).html

load-test:
	K6_WEB_DASHBOARD=true \
	K6_WEB_DASHBOARD_PORT=5665 \
	K6_WEB_DASHBOARD_OPEN=true \
	K6_WEB_DASHBOARD_EXPORT="$(EXPORT_FILE)" \
	k6 run ./rinha-source/rinha-test/rinha.js

run-one-instance-local:
	docker compose -f docker-compose-arm64.yml restart && air . 