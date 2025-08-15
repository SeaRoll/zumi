test:
	docker compose -f docker-compose.ci.yml up -d --wait
	go test -v -coverpkg=./... -coverprofile cover-full.out ./...
	docker compose -f docker-compose.ci.yml down
	grep -E -v "examples/|gen/gen\.go" cover-full.out > cover.out
	rm -rf cover-full.out

cover:
	go tool cover -func cover.out