@echo off
setlocal EnableDelayedExpansion
REM Original Windows Batch sample for SourceView.
set "name=SourceView"
set /a count=0
for %%i in (*.txt) do (
    set /a count+=1
    echo !count!: %%~fi
)
echo Project: %name%
echo Script directory: %~dp0
echo This command continues ^
    on the next physical line.
goto :done
:done
endlocal
