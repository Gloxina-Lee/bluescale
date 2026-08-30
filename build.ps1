Set-Location frontend
npm ci
npm run build
Set-Location ..

New-Item -ItemType Directory -Force dist | Out-Null
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"

go build -trimpath -ldflags="-s -w" -o dist/bluescale-linux-amd64 .