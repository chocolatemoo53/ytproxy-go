package utils

import "testing"

func TestDecryptQueryParams(t *testing.T) {
	const key = "abcdEFGH12345678"
	got, err := DecryptQueryParams("W/45ZRRlrST3QrEzkQEjc+QvrB+sNKqa1OUBpVgMbPk=", key)
	if err != nil || got != `[["pot","MnTok3n_-=/+abc"]]` {
		t.Fatalf("companion ciphertext: got %q, %v", got, err)
	}
	for _, bad := range []string{"", "YWJj", "G9fajP+glSP22Odqa4vpkQ==G9fa"} {
		if _, err := DecryptQueryParams(bad, key); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	DecryptQueryParams("AAAAAAAAAAAAAAAAAAAAAA==", key) // garbage padding must not panic
}
