<#
.SYNOPSIS
    Script de instalación para grepai en Windows.
.DESCRIPTION
    Descarga la última versión de grepai desde GitHub, la instala en
    %LOCALAPPDATA%\Programs\grepai y actualiza el PATH del usuario.
#>

$repo = "yoanbernabeu/grepai"
$installDir = "$env:LOCALAPPDATA\Programs\grepai"
$binName = "grepai.exe"

Write-Host "--- Instalador de grepai para Windows ---" -ForegroundColor Cyan

# 1. Obtener la última versión desde la API de GitHub
try {
    $releaseUrl = "https://api.github.com/repos/$repo/releases/latest"
    $latestRelease = Invoke-RestMethod -Uri $releaseUrl
    $versionTag = $latestRelease.tag_name
    # El autor usa la versión sin la 'v' inicial para el nombre del archivo
    $versionNumber = $versionTag.TrimStart('v')
    Write-Host "Versión detectada: $versionTag" -ForegroundColor Gray
} catch {
    Write-Error "Error: No se pudo conectar con la API de GitHub."
    exit 1
}

# 2. Identificar el asset correcto (grepai_{VERSION}_windows_amd64.zip)
$assetName = "grepai_$($versionNumber)_windows_amd64.zip"
$asset = $latestRelease.assets | Where-Object { $_.name -eq $assetName }

if (-not $asset) {
    Write-Error "No se encontró el paquete $assetName para Windows en el release actual."
    exit 1
}

$downloadUrl = $asset.browser_download_url

# 3. Preparar directorios de instalación
if (!(Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

$tempZip = "$env:TEMP\grepai_install.zip"

# 4. Descarga y Extracción
Write-Host "Descargando $assetName..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip

Write-Host "Extrayendo archivos en $installDir..." -ForegroundColor Cyan
# Extraemos el contenido. Force sobreescribe si ya existe.
Expand-Archive -Path $tempZip -DestinationPath $installDir -Force

# Limpiar el zip temporal
Remove-Item $tempZip

# 5. Configurar el PATH del usuario
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    Write-Host "Añadiendo grepai al PATH del usuario..." -ForegroundColor Yellow
    $newPath = "$userPath;$installDir"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

    # Actualizar la sesión actual para que 'grepai' funcione de inmediato
    $env:Path += ";$installDir"
}

Write-Host "`n¡Instalación completada con éxito!" -ForegroundColor Green
Write-Host "Lugar de instalación: $installDir" -ForegroundColor Gray
Write-Host "Ejecuta 'grepai version' para confirmar." -ForegroundColor White