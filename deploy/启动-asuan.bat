@echo off
setlocal
cd /d "%~dp0"
title asuan 同步启动器

echo ============================================
echo   asuan 同步启动器
echo ============================================
echo.

if not exist "asuan.exe" (
    echo [错误] 未找到 asuan.exe，请把本脚本放在 asuan.exe 同目录。
    pause
    exit /b 1
)

if not exist "asuan.json" (
    echo [首次运行] 未找到 asuan.json，正在初始化配置...
    echo.
    asuan.exe init
    if errorlevel 1 (
        echo [错误] 初始化失败，请检查 asuan.exe 与 syncthing.exe 是否完整。
        pause
        exit /b 1
    )
    echo.
    echo [提示] 已生成 asuan.json。如需连接其它设备，请编辑该文件：
    echo        - peers：其它设备的设备 ID + 地址
    echo        - folders：要同步的文件夹
    echo 按任意键继续启动（使用默认配置）...
    pause >nul
) else (
    echo [启动] 已检测到 asuan.json，直接启动同步...
)

echo.
echo [启动] 正在启动 asuan 同步服务（托盘图标可右键退出/暂停）...
echo [提示] 网页控制台将自动打开：http://127.0.0.1:18084
echo.

REM 延迟 3 秒自动打开网页控制台（等 Web 服务就绪）
start "" /b cmd /c "ping -n 4 127.0.0.1 >nul & start http://127.0.0.1:18084"

asuan.exe run

echo.
echo [退出] asuan 已停止。
pause
endlocal
