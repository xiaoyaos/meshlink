@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0meshlink.ps1" %*
exit /b %ERRORLEVEL%
