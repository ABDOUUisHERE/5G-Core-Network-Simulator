param([ValidateSet('test','race','vet','fmt','build','proto','tools')][string]$Task = 'test')
$ErrorActionPreference = 'Stop'
$projectDirectory = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ($Task -eq 'tools') {
    & docker build -t core5g-tools -f (Join-Path $projectDirectory 'deployments/docker/Tools.Dockerfile') $projectDirectory
} elseif ($Task -eq 'proto') {
    & docker run --rm -v "${projectDirectory}:/src" core5g-tools protoc -I api/proto --go_out=. --go_opt=module=core5g --go-grpc_out=. --go-grpc_opt=module=core5g api/proto/core/v1/core.proto
} else {
    $commands = @{test='go test ./... -count=1 -cover'; race='go test -race ./... -count=1'; vet='go vet ./...'; fmt='gofmt -w cmd internal pkg tests'; build='go build ./...'}
    & docker run --rm -v "${projectDirectory}:/src" -v core5g-go-mod:/go/pkg/mod -v core5g-go-build:/root/.cache/go-build -w /src golang:1.25-bookworm sh -c $commands[$Task]
}
if ($LASTEXITCODE -ne 0) { throw "Core5G $Task failed ($LASTEXITCODE)" }
