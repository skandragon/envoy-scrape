package main

import (
	"crypto/md5"
	"fmt"
	"strings"
)

const passwordRealm = "enphaseenergy.com"

func makeHashForSerial(serial string, username string) string {
	s := fmt.Sprintf("[e]%s@%s#%s EnPhAsE eNeRgY ", username, passwordRealm, serial)
	hmd5 := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", hmd5)
}

func makePasswordFromHash(s string) string {
	countZero := strings.Count(s, "0")
	countOne := strings.Count(s, "1")

	password := make([]byte, 8)

	rev := ""
	for _, v := range s {
		rev = string(v) + rev
	}

	for idx, c := range rev[:8] {
		if countZero == 3 || countZero == 6 || countZero == 9 {
			countZero--
		}
		if countZero > 20 {
			countZero = 20
		}
		if countZero < 0 {
			countZero = 0
		}

		if countOne == 9 || countOne == 15 {
			countOne--
		}
		if countOne > 26 {
			countOne = 26
		}
		if countOne < 0 {
			countOne = 0
		}

		if c == '0' {
			password[idx] = byte(int('f') + countZero)
			countZero--
		} else if c == '1' {
			password[idx] = byte(int('@') + countOne)
			countOne--
		} else {
			password[idx] = byte(c)
		}
	}
	return string(password)
}

func makePasswordForSerial(serial string, username string) string {
	return makePasswordFromHash(makeHashForSerial(serial, username))
}
