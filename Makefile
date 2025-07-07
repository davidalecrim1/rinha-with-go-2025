load-test: configure-dashboard
	k6 run ./rinha-source/rinha-test/rinha.js

configure-dashboard:
	export K6_WEB_DASHBOARD=true
	export K6_WEB_DASHBOARD_PORT=5665
	export K6_WEB_DASHBOARD_PERIOD=2s
	export K6_WEB_DASHBOARD_OPEN=true
	export K6_WEB_DASHBOARD_EXPORT='report.html'

run-one-instance-local:
	docker compose -f docker-compose-arm64.yml restart && air . 