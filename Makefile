.PHONY: test race vet fmt proto tools up down demo load
test:
	go test ./... -count=1 -cover
race:
	go test -race ./... -count=1
vet:
	go vet ./...
fmt:
	gofmt -w cmd internal pkg tests
tools:
	docker build -t core5g-tools -f deployments/docker/Tools.Dockerfile .
proto:
	docker run --rm -v "$(CURDIR):/src" core5g-tools protoc -I api/proto --go_out=. --go_opt=module=core5g --go-grpc_out=. --go-grpc_opt=module=core5g api/proto/core/v1/core.proto
up:
	docker compose up --build -d --wait
down:
	docker compose down
demo:
	python scripts/demo.py
load:
	python tests/performance/load.py --count 1000 --concurrency 50
