@echo off
setlocal
cd /d "%~dp0"
set "IMAJER_EXE=dist\imajer-windows-amd64.exe"
if /I "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "IMAJER_EXE=dist\imajer-windows-arm64.exe"
if not exist "%IMAJER_EXE%" (
  echo IMAJER binary bulunamadi.
  echo Lutfen GitHub Releases paketini indirin veya kaynak koddan derleyin.
  pause
  exit /b 1
)
"%IMAJER_EXE%" ui
if errorlevel 1 pause
