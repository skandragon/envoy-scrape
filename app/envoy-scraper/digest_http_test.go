package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getCnonce(t *testing.T) {
	t.Run("getCnonce", func(t *testing.T) {
		got := getCnonce()
		require.Len(t, got, 16)
	})
}

func Test_getMD5(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"foo", "acbd18db4cc2f85cedef654fccc4a4d8"},
		{"bar", "37b51d194a7513e45b56f6524f2d51f2"},
		{strings.Repeat("uhoh", 10), "18212ada675a1b66a3ea6e61d1560b4e"},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got := getMD5(tt.arg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_getDigestParts(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    map[string]string
	}{
		{
			"no authenticate header",
			http.Header{},
			map[string]string{},
		}, {
			"header with nonce",
			http.Header{"Www-Authenticate": []string{`Digest, nonce="foo"`}},
			map[string]string{"nonce": "foo"},
		}, {
			"header with extra parts",
			http.Header{"Www-Authenticate": []string{`Digest, nonce="foo", whatever="this"`}},
			map[string]string{"nonce": "foo"},
		}, {
			"all wanted components",
			http.Header{"Www-Authenticate": []string{`Digest, nonce="foo",realm="example.com",qop="auth"`}},
			map[string]string{"nonce": "foo", "realm": "example.com", "qop": "auth"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDigestParts(tt.headers)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_getDigestAuthorization(t *testing.T) {
	type args struct {
		nonce string
		parts map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"all parts",
			args{
				"abcdef",
				map[string]string{
					"username": "alice",
					"realm":    "example.com",
					"password": "secret",
					"method":   "GET",
					"uri":      "/foo",
					"nonce":    "abcdef",
					"qop":      "auth",
				},
			},
			`Digest username="alice", realm="example.com", nonce="abcdef", uri="/foo", cnonce="abcdef", nc="1", qop="auth", response="d1a132d3ab82343c98d18abe957b67d0"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDigestAuthorization(tt.args.nonce, tt.args.parts)
			assert.Equal(t, tt.want, got)
		})
	}
}
