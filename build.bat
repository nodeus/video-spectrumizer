set version="v1.0.0"
for /f "delims=" %%a in ('git log -1 --pretty^=format:"%%an (%%ae)"') do set "user=%%a"
go build -o video-spectrumizer.exe -v -ldflags="-X 'main.Version=%version%' -X 'main.BuildUser=%user%' -X 'main.BuildTime=%date% | %time%'" main.go