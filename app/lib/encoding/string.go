package encoding

import (
	"bufio"
	"fmt"
)

func DecodeString(r *bufio.Reader) (string, error) {
	length, isStringInt, err := Decode(r)
	if err != nil {
		return "", err
	}
	if isStringInt {
		return fmt.Sprint(length), err
	}

	b := make([]byte, length)
	if _, err := r.Read(b); err != nil {
		return "", err
	}

	return string(b), nil
}
