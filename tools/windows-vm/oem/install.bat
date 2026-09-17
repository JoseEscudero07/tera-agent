@echo off
rem Se ejecuta una vez al terminar la instalacion desatendida de Windows.
powershell -NoProfile -ExecutionPolicy Bypass -File C:\OEM\setup-remote.ps1 > C:\OEM\setup-remote.log 2>&1
