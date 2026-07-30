//go:build !windows

package keystore

import "fmt"

func defaultWrapMethod() WrapMethod { return WrapPassphrase }

func dpapiProtect(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("%w: dpapi unavailable on this OS", ErrUnsupportedWrap)
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("%w: dpapi unavailable on this OS", ErrUnsupportedWrap)
}
