param([string]$Binary = '', [switch]$Stop)
$ErrorActionPreference = 'Stop'
$projectDirectory = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$stateFile = Join-Path $projectDirectory 'test-results/local-lab-processes.json'
if ($Stop) {
    if (Test-Path -LiteralPath $stateFile) {
        $previous = Get-Content -Raw -LiteralPath $stateFile | ConvertFrom-Json
        foreach ($record in $previous) {
            $ownedProcess = Get-Process -Id $record.pid -ErrorAction SilentlyContinue
            if ($ownedProcess -and $ownedProcess.Path -eq $record.binary) { Stop-Process -Id $record.pid }
        }
    }
    Write-Output 'Core5G local processes stopped.'
    exit 0
}
if (-not $Binary) { $Binary = Join-Path $projectDirectory 'bin/core5g.exe' }
$Binary = (Resolve-Path -LiteralPath $Binary).Path
$logDirectory = Join-Path $projectDirectory 'test-results/local-logs'
New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
$ports = @{udm=50051;ausf=50052;amf=50053;smf=50054;upf=50055;'gnb-simulator'=50056;'control-api'=50057}
$httpPorts = @{udm=18081;ausf=18082;amf=18083;smf=18084;upf=18085;'gnb-simulator'=18086;'control-api'=8085}
$variables = @('SERVICE','GRPC_ADDR','HTTP_ADDR','DATABASE_URL','OTEL_EXPORTER_OTLP_ENDPOINT','UDM_ADDR','AUSF_ADDR','AMF_ADDR','SMF_ADDR','UPF_ADDR','GNB_ADDR')
$saved = @{}
foreach ($name in $variables) { $saved[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
$started = @()
try {
    foreach ($role in @('udm','ausf','amf','smf','upf','gnb-simulator','control-api')) {
        [Environment]::SetEnvironmentVariable('SERVICE',$role,'Process')
        [Environment]::SetEnvironmentVariable('GRPC_ADDR',('127.0.0.1:' + $ports[$role]),'Process')
        [Environment]::SetEnvironmentVariable('HTTP_ADDR',('127.0.0.1:' + $httpPorts[$role]),'Process')
        [Environment]::SetEnvironmentVariable('DATABASE_URL','','Process')
        [Environment]::SetEnvironmentVariable('OTEL_EXPORTER_OTLP_ENDPOINT','','Process')
        foreach ($dependency in @('udm','ausf','amf','smf','upf')) { [Environment]::SetEnvironmentVariable(($dependency.ToUpper() + '_ADDR'),('127.0.0.1:' + $ports[$dependency]),'Process') }
        [Environment]::SetEnvironmentVariable('GNB_ADDR','127.0.0.1:50056','Process')
        $process = Start-Process -FilePath $Binary -WindowStyle Hidden -WorkingDirectory $projectDirectory -RedirectStandardOutput (Join-Path $logDirectory "$role.out.log") -RedirectStandardError (Join-Path $logDirectory "$role.err.log") -PassThru
        $started += [pscustomobject]@{service=$role;pid=$process.Id;binary=$Binary}
    }
    $started | ConvertTo-Json | Set-Content -LiteralPath $stateFile -Encoding UTF8
} catch {
    foreach ($record in $started) { Stop-Process -Id $record.pid -ErrorAction SilentlyContinue }
    throw
} finally {
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$saved[$name],'Process') }
}
Write-Output 'Core5G: http://127.0.0.1:8085 (volatile in-memory mode; no PostgreSQL/Grafana/Jaeger in this preview).'
