TS := $(shell date '+%Y%m%d_%H%M%S')
EXPORT_FILE := reports/report_$(TS).html

load-test:
	K6_WEB_DASHBOARD=true \
	K6_WEB_DASHBOARD_PORT=5665 \
	K6_WEB_DASHBOARD_OPEN=true \
	K6_WEB_DASHBOARD_EXPORT="$(EXPORT_FILE)" \
	k6 run ./rinha-source/rinha-test/rinha.js

super-load-test:
	K6_WEB_DASHBOARD=true \
	K6_WEB_DASHBOARD_PORT=5665 \
	K6_WEB_DASHBOARD_OPEN=true \
	K6_WEB_DASHBOARD_EXPORT="$(EXPORT_FILE)" \
	k6 run -e MAX_REQUESTS=850 ./rinha-source/rinha-test/rinha.js

run-one-instance-local:
	docker compose -f rinha-docker-compose-arm64.yml restart && air . 

run-docker:
	make run-processor && docker compose up --build -d

run-processor:
	docker compose -f rinha-docker-compose-arm64.yml up -d

profiling-cpu:
	pproftui ./docs/profiling/go-backend-1/cpu.prof

profiling-memory:
	pproftui ./docs/profiling/go-backend-1/memory.prof

profiling-trace:
	pproftui ./docs/profiling/go-backend-1/trace.prof

build-docker:
	docker build -t davidalecrim1/rinha-with-go-2025:latest .

push-image:
	docker push davidalecrim1/rinha-with-go-2025:latest

build-for-amd64:
	docker buildx build \
	--platform linux/amd64 \
	-t davidalecrim1/rinha-with-go-2025:v0.6-mongodb \
	--push \
	.

