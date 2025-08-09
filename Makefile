TS := $(shell date '+%Y%m%d_%H%M%S')
EXPORT_FILE := reports/report_$(TS).html
VERSION_API := v2.0.0-api
VERSION_WORKER := v2.0.0-worker

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

run-amd64:
	make run-processor-amd64 && docker compose up --build -d

run-processor:
	docker compose -f rinha-docker-compose-arm64.yml up -d

run-processor-amd64:
	docker compose -f rinha-docker-compose-amd64.yml up -d

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
	-t davidalecrim1/rinha-with-go-2025:$(VERSION_API) \
	--push \
	-f Dockerfile.api .

	docker buildx build \
	--platform linux/amd64 \
	-t davidalecrim1/rinha-with-go-2025:$(VERSION_WORKER) \
	--push \
	-f Dockerfile.worker .