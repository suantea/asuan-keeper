' asuan background launcher
' Runs asuan.exe in a hidden window (no console, no taskbar window).
' Double-click to start asuan sync in the background.
' Stop it via the tray menu or by running: asuan stop
Set fso = CreateObject("Scripting.FileSystemObject")
Set sh = CreateObject("WScript.Shell")
dir = fso.GetParentFolderName(WScript.ScriptFullName)
sh.Run """" & dir & "\asuan.exe"" run", 0, False
