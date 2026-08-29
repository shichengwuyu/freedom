<#
.SYNOPSIS
  Generate PNG icons (16/32/48/128) for the vendor-browser-extension.
  Uses System.Drawing (built into Windows). No npm/third-party dependency.

.USAGE
  In PowerShell inside vendor-browser-extension:
    powershell -ExecutionPolicy Bypass -File .\generate-icons.ps1
#>

Add-Type -AssemblyName System.Drawing

$OutDir = Join-Path $PSScriptRoot "icons"
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Path $OutDir -Force | Out-Null }

$sizes = @(16, 32, 48, 128)

foreach ($size in $sizes) {

    $bmp = New-Object System.Drawing.Bitmap -ArgumentList $size, $size, ([System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g   = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode      = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.TextRenderingHint  = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit

    # Background: indigo-500 -> violet-500 gradient rounded rect
    $rect = New-Object System.Drawing.Rectangle -ArgumentList 0, 0, $size, $size
    $c1   = [System.Drawing.Color]::FromArgb(0xFF, 0x63, 0x66, 0xF1)
    $c2   = [System.Drawing.Color]::FromArgb(0xFF, 0x8B, 0x5C, 0xF6)
    $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush -ArgumentList $rect, $c1, $c2, ([System.Drawing.Drawing2D.LinearGradientMode]::ForwardDiagonal)

    $r = [int][Math]::Max(2, [Math]::Floor($size * 0.22))
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $X1 = 0
    $Y1 = 0
    $X2 = $size - 2 * $r
    $Y2 = $size - 2 * $r
    $W  = 2 * $r
    $path.AddArc($X1, $Y1, $W, $W, 180, 90)
    $path.AddArc($X2, $Y1, $W, $W, 270, 90)
    $path.AddArc($X2, $Y2, $W, $W,   0, 90)
    $path.AddArc($X1, $Y2, $W, $W,  90, 90)
    $path.CloseFigure()
    $g.FillPath($brush, $path)

    # Foreground: large 48/128 show cloud char (Segoe UI Symbol) ; small 16/32 show letter V bold
    $white  = [System.Drawing.Brushes]::White
    $sf = New-Object System.Drawing.StringFormat
    $sf.Alignment     = [System.Drawing.StringAlignment]::Center
    $sf.LineAlignment = [System.Drawing.StringAlignment]::Center

    if ($size -ge 48) {
        $emojiPx = [int][Math]::Floor($size * 0.72)
        $font = New-Object System.Drawing.Font -ArgumentList "Segoe UI Symbol", $emojiPx, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
        $g.DrawString([char]0x2601, $font, $white, [System.Drawing.RectangleF]$rect, $sf)
    } else {
        $letterPx = [int][Math]::Floor($size * 0.78)
        $font = New-Object System.Drawing.Font -ArgumentList "Segoe UI", $letterPx, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
        $g.DrawString("V", $font, $white, [System.Drawing.RectangleF]$rect, $sf)
    }

    $dest = Join-Path $OutDir "icon$size.png"
    $bmp.Save($dest, [System.Drawing.Imaging.ImageFormat]::Png)

    Write-Host ("[OK] generated: icon{0}.png  ({0}x{0})" -f $size)

    $g.Dispose()
    $bmp.Dispose()
    $brush.Dispose()
    $path.Dispose()
    $sf.Dispose()
    $font.Dispose()
}

Write-Host ""
Write-Host "Done. Now reload the unpacked extension in chrome://extensions to see the new icon."
