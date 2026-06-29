package utils

import (
	"crypto/aes"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
)

func CopyHeaders(from http.Header, to http.Header, length bool) {
	// Loop over header names
outer:
	for name, values := range from {
		for _, header := range strip_headers {
			if name == header {
				continue outer
			}
		}
		if (name != "Content-Length" || length) && !strings.HasPrefix(name, "Access-Control") {
			// Loop over all values for the name.
			for _, value := range values {
				if strings.Contains(value, "jpeg") {
					continue
				}
				to.Set(name, value)
			}
		}
	}
}

func CopyHeadersNew(from http.Header, to http.Header) {
	for from_header, value := range from {
		for _, header := range headers_for_response {
			if from_header == header {
				to.Add(header, value[0])
			}
		}
	}
}

func GetBestThumbnail(path string) (newpath string) {

	formats := [4]string{"maxresdefault.jpg", "sddefault.jpg", "hqdefault.jpg", "mqdefault.jpg"}

	for _, format := range formats {
		newpath = strings.Replace(path, "maxres.jpg", format, 1)
		url := "https://i.ytimg.com" + newpath
		resp, _ := httpc.Client.Head(url)
		if resp.StatusCode == 200 {
			return newpath
		}
	}

	return strings.Replace(path, "maxres.jpg", "mqdefault.jpg", 1)
}

func RelativeUrl(in string) (newurl string) {
	segment_url, err := url.Parse(in)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to parse url")
	}
	segment_query := segment_url.Query()
	segment_query.Set("host", segment_url.Hostname())
	segment_url.RawQuery = segment_query.Encode()
	segment_url.Path = path_prefix + segment_url.Path
	return segment_url.RequestURI()
}

func PanicHandler(w http.ResponseWriter) {
	if r := recover(); r != nil {
		log.Logger.Panic().Any("recover", r).Msg("something panicked!")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// https://stackoverflow.com/a/41652605
func DecryptQueryParams(encryptedQuery string, key string) (string, error) {
	se, err := base64.StdEncoding.DecodeString(encryptedQuery)
	if err != nil {
		log.Logger.Error().Err(err).Msg("error when decoding base64 string")
		return "", err
	}

	cipher, err := aes.NewCipher([]byte(key)[0:16])
	if err != nil {
		log.Logger.Error().Err(err).Msg("error initializating cipher.Block")
		return "", err
	}
	decrypted := make([]byte, len(se))
	size := 16

	for bs, be := 0, size; bs < len(se); bs, be = bs+size, be+size {
		cipher.Decrypt(decrypted[bs:be], se[bs:be])
	}

	paddingSize := int(decrypted[len(decrypted)-1])
	return string(decrypted[0 : len(decrypted)-paddingSize]), nil
}
