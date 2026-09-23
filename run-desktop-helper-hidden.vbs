Option Explicit

Dim fso, shell, root, executable
Set fso = CreateObject("Scripting.FileSystemObject")
Set shell = CreateObject("WScript.Shell")

root = fso.GetParentFolderName(WScript.ScriptFullName)
executable = fso.BuildPath(root, "build\thesis-agent-desktop.exe")

If Not fso.FileExists(executable) Then
    WScript.Quit 1
End If

' Window style 0 keeps the Go console process hidden.
shell.Run Chr(34) & executable & Chr(34), 0, False
