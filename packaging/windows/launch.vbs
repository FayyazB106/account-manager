' Launches Account Manager in the background with a system-tray icon.
' Runs the exe hidden (window style 0 = no console window). The exe shows the tray
' icon, opens the browser at the correct port, and writes its own log to
' data\server.log. We deliberately do NOT redirect output here with "cmd > file":
' the running instance keeps that file open, so a second launch (clicking the
' shortcut again) would fail the redirect and never start — instead we let the exe
' run and detect the already-running instance, then just open the browser.
' Data is written under the executable's folder (a per-user, writable location).
Option Explicit
Dim sh, dir
Set sh = CreateObject("WScript.Shell")
dir = Left(WScript.ScriptFullName, InStrRev(WScript.ScriptFullName, "\"))
sh.CurrentDirectory = dir
sh.Run Chr(34) & dir & "account-manager.exe" & Chr(34) & " -tray", 0, False
