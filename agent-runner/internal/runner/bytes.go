package runner

import "bytes"

func bytesReader(value []byte) *bytes.Reader { return bytes.NewReader(value) }
