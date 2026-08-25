# 从 DB 导出项目71 剧本原文（UTF-8 无 BOM）
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$t = & docker exec duanju-postgres psql -U duanju -d duanju -t -A -c "SELECT source_script_text FROM projects WHERE id=71"
$text = ($t | Out-String).Trim()
[IO.File]::WriteAllText("d:\develop\a_work\duanju\scripts\verify-script.txt", $text, [System.Text.UTF8Encoding]::new($false))
Write-Host ("exported length = " + $text.Length)
