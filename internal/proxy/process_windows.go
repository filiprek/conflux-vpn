//go:build windows

package proxy

import (
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

// Every child starts suspended. Assignment to a non-inheritable kill-on-close job
// precedes its first instruction, so abrupt service termination cannot orphan it.
func runManaged(cmd *exec.Cmd) error {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		return e
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED, HideWindow: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	cleanup := func(err error) error { cmd.Process.Kill(); cmd.Wait(); return err }
	process, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if e != nil {
		return cleanup(e)
	}
	defer windows.CloseHandle(process)
	if e = windows.AssignProcessToJobObject(job, process); e != nil {
		return cleanup(e)
	}
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return cleanup(e)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	e = windows.Thread32First(snapshot, &entry)
	for e == nil {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return cleanup(err)
			}
			_, err = windows.ResumeThread(thread)
			windows.CloseHandle(thread)
			if err != nil {
				return cleanup(err)
			}
			return cmd.Wait()
		}
		e = windows.Thread32Next(snapshot, &entry)
	}
	return cleanup(errors.New("cannot resume supervised sing-box process"))
}
