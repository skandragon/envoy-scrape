package main

import (
	"fmt"
	"testing"
)

func Test_makeHashForSerial(t *testing.T) {
	tests := []struct {
		serial   string
		username string
		want     string
	}{
		{"012345678901", "installer", "1d98c6f7a544876adc4bae222c2e6cff"},
		{"123456789012", "bob", "7c60b191a3f45f42c8b99d209a5c60cb"},
	}
	for idx, tt := range tests {
		t.Run(fmt.Sprintf("case-%d", idx+1), func(t *testing.T) {
			if got := makeHashForSerial(tt.serial, tt.username); got != tt.want {
				t.Errorf("makeHashForSerial() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makePasswordFromHash(t *testing.T) {
	tests := []struct {
		s    string
		want string
	}{
		{"1d98c6f7a544876adc4bae222c2e6cff", "ffc6e2c2"},
		{"7c60b191a3f45f42c8b99d209a5c60cb", "bch6c5a9"},
		{"5bab74732cca8faf481303b664045ce9", "9ec54h46"},
		{"00000000000000000000000000000000", "zyxwvuts"},
		{"11111111111111111111111111111111", "ZYXWVUTS"},
	}
	for idx, tt := range tests {
		t.Run(fmt.Sprintf("case-%d", idx+1), func(t *testing.T) {
			if got := makePasswordFromHash(tt.s); got != tt.want {
				t.Errorf("makePasswordFromHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makePasswordForSerial(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		serial   string
		username string
		want     string
	}{
		{"012345678901", "installer", "ffc6e2c2"},
		{"123456789012", "bob", "bch6c5a9"},
	}
	for idx, tt := range tests {
		t.Run(fmt.Sprintf("case-%d", idx+1), func(t *testing.T) {
			if got := makePasswordForSerial(tt.serial, tt.username); got != tt.want {
				t.Errorf("makePasswordForSerial() = %v, want %v", got, tt.want)
			}
		})
	}
}
