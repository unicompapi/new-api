[CmdletBinding()]
param(
    [string]$BaseUrl = "http://127.0.0.1:3000",
    [string]$GatewayKey,
    [string]$Prompt = "Cinematic shot of a horse running across a grassland, sunlight through dust",
    [string[]]$ReferenceImageUrl = @(
        "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20250925/wpimhv/rap.png"
    ),
    [string]$OutputDirectory = ".\tmp\media-api-test-output",
    [int]$PollIntervalSeconds = 5,
    [int]$PollTimeoutSeconds = 900,
    [switch]$SkipVideo,
    [switch]$SkipImage,
    [switch]$DownloadVideo
)

# The gateway token is intentionally read from the environment or a parameter.
if ([string]::IsNullOrWhiteSpace($GatewayKey)) {
    $GatewayKey = $env:NEW_API_KEY
}
if ([string]::IsNullOrWhiteSpace($GatewayKey)) {
    throw "Set NEW_API_KEY or pass -GatewayKey before running this script."
}
if ($ReferenceImageUrl.Count -lt 1) {
    throw "At least one publicly reachable image URL is required for i2v/r2v tests."
}

$BaseUrl = $BaseUrl.TrimEnd('/')
$Headers = @{
    Authorization = "Bearer $GatewayKey"
    Accept        = "application/json"
}

if ([System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $ResolvedOutputDirectory = $OutputDirectory
} else {
    $ResolvedOutputDirectory = Join-Path (Get-Location).Path $OutputDirectory
}
New-Item -ItemType Directory -Path $ResolvedOutputDirectory -Force | Out-Null

function Get-SafeFilePart {
    param([Parameter(Mandatory)][string]$Value)
    return ($Value -replace '[^A-Za-z0-9._-]', '_')
}

function Save-JsonResponse {
    param(
        [Parameter(Mandatory)]$Value,
        [Parameter(Mandatory)][string]$FileName
    )
    $path = Join-Path $ResolvedOutputDirectory $FileName
    $Value | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $path -Encoding UTF8
    return $path
}

function Invoke-GatewayJson {
    param(
        [Parameter(Mandatory)][ValidateSet('Get', 'Post')][string]$Method,
        [Parameter(Mandatory)][string]$Path,
        $Body
    )

    $request = @{
        Uri         = "$BaseUrl$Path"
        Method      = $Method
        Headers     = $Headers
        ErrorAction = 'Stop'
    }
    if ($null -ne $Body) {
        $request.ContentType = 'application/json; charset=utf-8'
        $json = $Body | ConvertTo-Json -Depth 30 -Compress
        $request.Body = [System.Text.Encoding]::UTF8.GetBytes($json)
    }

    try {
        return Invoke-RestMethod @request
    } catch {
        $detail = $_.ErrorDetails.Message
        if ([string]::IsNullOrWhiteSpace($detail)) {
            $detail = $_.Exception.Message
        }
        throw "[$Method $Path] $detail"
    }
}

function Wait-VideoTask {
    param(
        [Parameter(Mandatory)][string]$TaskId,
        [Parameter(Mandatory)][string]$Model
    )

    $safeModel = Get-SafeFilePart $Model
    $safeTaskId = Get-SafeFilePart $TaskId
    $deadline = (Get-Date).AddSeconds($PollTimeoutSeconds)
    $lastResponse = $null

    do {
        $lastResponse = Invoke-GatewayJson -Method Get -Path ("/v1/videos/{0}" -f [Uri]::EscapeDataString($TaskId))
        $statusFile = "video-$safeModel-$safeTaskId-status.json"
        Save-JsonResponse -Value $lastResponse -FileName $statusFile | Out-Null

        $status = ([string]$lastResponse.status).ToLowerInvariant()
        Write-Host ("  status: {0}  progress: {1}" -f $status, $lastResponse.progress)
        if ($status -in @('completed', 'succeeded', 'success')) {
            return $lastResponse
        }
        if ($status -in @('failed', 'failure', 'cancelled', 'canceled')) {
            throw ("Video task {0} failed: {1}" -f $TaskId, $lastResponse.error.message)
        }
        if ((Get-Date) -ge $deadline) {
            throw ("Timed out waiting for video task {0}. Last status: {1}" -f $TaskId, $status)
        }
        Start-Sleep -Seconds $PollIntervalSeconds
    } while ($true)
}

function Submit-VideoTest {
    param(
        [Parameter(Mandatory)][hashtable]$Request,
        [Parameter(Mandatory)][string]$Model
    )

    $safeModel = Get-SafeFilePart $Model
    Write-Host "`n[video] $Model"
    Write-Host ("  request: {0}" -f (($Request | ConvertTo-Json -Depth 30 -Compress)))
    $created = Invoke-GatewayJson -Method Post -Path '/v1/videos' -Body $Request
    Save-JsonResponse -Value $created -FileName "video-$safeModel-created.json" | Out-Null

    $taskId = [string]$created.id
    if ([string]::IsNullOrWhiteSpace($taskId)) {
        $taskId = [string]$created.task_id
    }
    if ([string]::IsNullOrWhiteSpace($taskId)) {
        throw "The video response did not contain id or task_id."
    }
    Write-Host "  task id: $taskId"

    $completed = Wait-VideoTask -TaskId $taskId -Model $Model
    if ($DownloadVideo) {
        $safeTaskId = Get-SafeFilePart $taskId
        $videoPath = Join-Path $ResolvedOutputDirectory "video-$safeModel-$safeTaskId.mp4"
        try {
            Invoke-WebRequest `
                -Uri "$BaseUrl/v1/videos/$([Uri]::EscapeDataString($taskId))/content" `
                -Method Get `
                -Headers $Headers `
                -OutFile $videoPath `
                -ErrorAction Stop | Out-Null
            Write-Host "  saved video: $videoPath"
        } catch {
            Write-Warning ("  task completed, but video download failed: {0}" -f $_.Exception.Message)
        }
    }
    return $completed
}

$videoResults = @()
$imageResults = @()

if (-not $SkipVideo) {
    $videoRequests = @(
        [ordered]@{
            model      = 'happyhorse-1.1-i2v'
            prompt     = $Prompt
            images     = @($ReferenceImageUrl[0])
            resolution = '720P'
            duration   = 5
            metadata   = [ordered]@{ ratio = '16:9'; prompt_extend = $true; watermark = $false }
        },
        [ordered]@{
            model      = 'happyhorse-1.1-r2v'
            prompt     = $Prompt
            images     = @($ReferenceImageUrl)
            resolution = '720P'
            duration   = 5
            metadata   = [ordered]@{ ratio = '16:9'; prompt_extend = $true; watermark = $false }
        },
        [ordered]@{
            model      = 'wan3.0-video'
            prompt     = $Prompt
            resolution = '1080P'
            duration   = 5
            metadata   = [ordered]@{ ratio = 'adaptive'; prompt_extend = $true; watermark = $false }
        },
        [ordered]@{
            model      = 'wan3.0-video-prime'
            prompt     = $Prompt
            resolution = '1080P'
            duration   = 5
            metadata   = [ordered]@{ ratio = 'adaptive'; prompt_extend = $true; watermark = $false }
        }
    )

    foreach ($request in $videoRequests) {
        $model = [string]$request.Model
        try {
            $videoResults += Submit-VideoTest -Request $request -Model $model
        } catch {
            Write-Warning ("  $model failed: {0}" -f $_.Exception.Message)
        }
    }
}

if (-not $SkipImage) {
    $imageModels = @(
        'qwen-image-3.0',
        'qwen-image-3.0-pro',
        'qwen-image-2.0',
        'qwen-image-2.0-pro'
    )

    foreach ($model in $imageModels) {
        Write-Host "`n[image] $model"
        $request = [ordered]@{
            model           = $model
            prompt          = 'A horse running across a grassland, cinematic composition, high detail'
            n               = 1
            size            = '1328x1328'
            watermark       = $false
            prompt_extend   = $true
            negative_prompt = 'low quality, blurry, deformed'
        }
        try {
            $response = Invoke-GatewayJson -Method Post -Path '/v1/images/generations' -Body $request
            $safeModel = Get-SafeFilePart $model
            Save-JsonResponse -Value $response -FileName "image-$safeModel.json" | Out-Null
            $urls = @($response.data | ForEach-Object { if ($_.url) { $_.url } })
            if ($urls.Count -gt 0) {
                Write-Host ("  output: {0}" -f ($urls -join ', '))
            } else {
                Write-Host "  response saved; no URL field was returned (check b64_json)."
            }
            $imageResults += $response
        } catch {
            Write-Warning ("  $model failed: {0}" -f $_.Exception.Message)
        }
    }
}

Write-Host "`nDone. Raw responses are in: $ResolvedOutputDirectory"
