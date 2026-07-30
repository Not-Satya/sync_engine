//go:build windows

package keystore

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func defaultWrapMethod() WrapMethod { return WrapDPAPI }

func dpapiProtect(data []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	name, err := windows.UTF16PtrFromString("sync_engine keystore")
	if err != nil {
		return nil, err
	}
	if err := windows.CryptProtectData(&in, name, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi protect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	dup := make([]byte, out.Size)
	copy(dup, unsafe.Slice(out.Data, out.Size))
	return dup, nil
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("dpapi unprotect: empty")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var outName *uint16
	if err := windows.CryptUnprotectData(&in, &outName, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi unprotect: %w", err)
	}
	if outName != nil {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outName)))
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	dup := make([]byte, out.Size)
	copy(dup, unsafe.Slice(out.Data, out.Size))
	return dup, nil
}
