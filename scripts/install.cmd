@echo off
setlocal
cd /d "%~dp0"
net session >nul 2>&1
if not "%ERRORLEVEL%"=="0" (
  echo [INFO] Requesting administrator privileges...
  powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -FilePath '%~f0' -ArgumentList '%*' -Verb RunAs"
  exit /b 0
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
set "EXIT_CODE=%ERRORLEVEL%"
echo.
if "%EXIT_CODE%"=="0" (
  echo [OK] Install command finished.
) else (
  echo [ERROR] Install command failed with code %EXIT_CODE%.
)
echo.
pause
exit /b %EXIT_CODE%
