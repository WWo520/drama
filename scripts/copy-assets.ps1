# 复制生成的封面/横幅素材到工程 assets
$src = "C:\Users\yz02\.qoder\vibe_images"
$dst = "d:\develop\a_work\duanju\apps\web-user\src\assets\covers"
New-Item -ItemType Directory -Force -Path $dst | Out-Null
$pairs = @(
  @("project-cover-1_1787040042.png", "project-cover-1.png"),
  @("project-cover-2_1787040110.png", "project-cover-2.png"),
  @("project-cover-3_1787040178.png", "project-cover-3.png"),
  @("project-cover-4_1787040219.png", "project-cover-4.png"),
  @("carousel-banner-member_1787040240.png", "banner-member.png"),
  @("carousel-banner-canvas_1787040272.png", "banner-canvas.png"),
  @("carousel-banner-tutorial_1787040313.png", "banner-tutorial.png"),
  @("carousel-banner-discover_1787040359.png", "banner-discover.png")
)
foreach ($p in $pairs) {
  Copy-Item (Join-Path $src $p[0]) (Join-Path $dst $p[1]) -Force
}
Get-ChildItem $dst | ForEach-Object { Write-Output ("$($_.Name) $([int]($_.Length/1KB))KB") }
