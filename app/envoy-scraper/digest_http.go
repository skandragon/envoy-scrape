package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

func digestGet(username string, password string, uri string) (int, []byte, error) {
	method := "GET"
	req, err := http.NewRequest(method, uri, nil)
	req.Header.Set("Accepts", "application/json")
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, []byte{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, err := io.ReadAll(resp.Body)
		return resp.StatusCode, body, err
	}
	digestParts := getDigestParts(resp.Header)
	digestParts["uri"] = req.RequestURI
	digestParts["method"] = method
	digestParts["username"] = username
	digestParts["password"] = password
	req, err = http.NewRequest(method, uri, nil)
	cnonce := getCnonce()
	req.Header.Set("Authorization", getDigestAuthorization(cnonce, digestParts))

	resp, err = client.Do(req)
	if err != nil {
		return -1, []byte{}, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return -1, []byte{}, err
	}
	return resp.StatusCode, body, err
}

func getDigestParts(headers http.Header) map[string]string {
	result := map[string]string{}
	if len(headers["Www-Authenticate"]) > 0 {
		wantedHeaders := []string{"nonce", "realm", "qop"}
		responseHeaders := strings.Split(headers["Www-Authenticate"][0], ",")
		for _, r := range responseHeaders {
			for _, w := range wantedHeaders {
				if strings.Contains(r, w) {
					result[w] = strings.Split(r, `"`)[1]
				}
			}
		}
	}
	return result
}

func getMD5(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func getCnonce() string {
	b := make([]byte, 8)
	io.ReadFull(rand.Reader, b)
	return fmt.Sprintf("%x", b)[:16]
}

// getDigestAuthorization does not implement the full spec, but it works for this use case.
func getDigestAuthorization(cnonce string, digestParts map[string]string) string {
	d := digestParts
	ha1 := getMD5(d["username"] + ":" + d["realm"] + ":" + d["password"])
	ha2 := getMD5(d["method"] + ":" + d["uri"])
	nonceCount := 1
	response := getMD5(fmt.Sprintf("%s:%s:%v:%s:%s:%s", ha1, d["nonce"], nonceCount, cnonce, d["qop"], ha2))
	authorization := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", cnonce="%s", nc="%v", qop="%s", response="%s"`,
		d["username"], d["realm"], d["nonce"], d["uri"], cnonce, nonceCount, d["qop"], response)
	return authorization
}
