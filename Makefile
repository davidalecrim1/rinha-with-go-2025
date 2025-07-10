TS := $(shell date '+%Y%m%d_%H%M%S')
EXPORT_FILE := reports/report_$(TS).html

load-test:
	K6_WEB_DASHBOARD=true \
	K6_WEB_DASHBOARD_PORT=5665 \
	K6_WEB_DASHBOARD_OPEN=true \
	K6_WEB_DASHBOARD_EXPORT="$(EXPORT_FILE)" \
	k6 run ./rinha-source/rinha-test/rinha.js

run-one-instance-local:
	docker compose -f rinha-docker-compose-arm64.yml restart && air . 

run-docker:
	docker compose -f rinha-docker-compose-arm64.yml restart && docker compose up --build -d

profiling-cpu:
	go tool pprof -http=:8080 ./docs/profiling/cpu.prof

profiling-memory:
	go tool pprof -http=:8081 ./docs/profiling/memory.prof

profiling-trace:
	go tool trace ./docs/profiling/trace.prof

