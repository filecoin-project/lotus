@echo off
set TOP=%~dp0
cl /nologo /c /O2 /Zi /Fdblst.pdb /MT /Zl %TOP%src\server.c || EXIT /B
FOR %%F IN (%TOP%build\win64\*-x86_64.asm) DO (
    ml64 /nologo /c /Cp /Cx /Zi %%F || EXIT /B
)
rem FOR %%F IN (%TOP%src\asm\*-x86_64.pl) DO (
rem     IF NOT EXIST %%~nF.asm (perl %%F masm %%~nF.asm)
rem )
rem FOR %%F IN (*.asm) DO (ml64 /nologo /c /Cp /Cx /Zi %%F || EXIT /B)
FOR %%O IN (%*) DO ( IF "%%O" == "-shared" (
    cl /nologo /c /O2 /Oi- /MD %TOP%build\win64\dll.c || EXIT /B
    link /nologo /debug /dll /entry:DllMain /incremental:no /implib:blst.lib /pdb:blst.pdb /def:%TOP%build\win64\blst.def *.obj kernel32.lib && del *.obj
    EXIT /B
    )
)
lib /nologo /OUT:blst.lib *.obj && del *.obj
