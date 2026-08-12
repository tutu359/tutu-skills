@echo off
setlocal

set "script_dir=%~dp0"

if /I "%PROCESSOR_ARCHITEW6432%"=="ARM64" goto arm64
if /I "%PROCESSOR_ARCHITECTURE%"=="ARM64" goto arm64
if /I "%PROCESSOR_ARCHITEW6432%"=="AMD64" goto amd64
if /I "%PROCESSOR_ARCHITECTURE%"=="AMD64" goto amd64

>&2 echo error: unsupported Windows architecture: %PROCESSOR_ARCHITECTURE%
exit /b 2

:arm64
set "executable=%script_dir%img-gen-windows-arm64.exe"
goto run

:amd64
set "executable=%script_dir%img-gen-windows-amd64.exe"

:run
"%executable%" %*
exit /b %errorlevel%
