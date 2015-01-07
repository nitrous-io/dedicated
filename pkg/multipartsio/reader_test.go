package multipartsio

import (
	"bytes"
	"io"
	"mime/multipart"
	"regexp"
	"strings"
	"testing"
)

func TestPartsReaderCopy(t *testing.T) {
	testBody := regexp.MustCompile("\n").ReplaceAllString(message, "\r\n")
	b := strings.NewReader(testBody)
	mp := multipart.NewReader(b, boundary)
	f, err := mp.ReadForm(25)
	if err != nil {
		t.Fatalf("ReadForm: %s", err)
	}
	defer f.RemoveAll()

	output := bytes.NewBuffer(nil)
	pr := NewPartsReader(f.File["filea"][0])
	_, err = io.Copy(output, pr)
	if err != nil {
		t.Errorf("PartsReader#WriteTo %s", err)
	}
	if o := output.String(); o != fileaContents {
		t.Errorf("Output(%q), expect %q", o, fileaContents)
	}
}

func TestPartsReaderRead(t *testing.T) {
	testBody := regexp.MustCompile("\n").ReplaceAllString(message, "\r\n")
	b := strings.NewReader(testBody)
	mp := multipart.NewReader(b, boundary)
	f, err := mp.ReadForm(5)
	if err != nil {
		t.Fatalf("ReadForm: %s", err)
	}
	defer f.RemoveAll()

	output := make([]byte, 10)
	pr := NewPartsReader(f.File["filea"][0])

	n1, err := pr.Read(output)
	if err != nil {
		t.Errorf("unexpected error %s", err)
	}
	if n1 != 10 || string(output) != "This is a " {
		t.Errorf("output(%q) unexpected", string(output))
	}

	n2, err := pr.Read(output)
	if err != nil {
		t.Errorf("unexpected error %s", err)
	}
	if n2 != 10 || string(output) != "test file." {
		t.Errorf("output(%q) unexpected", string(output))
	}

	n3, err := pr.Read(output)
	if err != io.EOF || n3 != 0 {
		t.Log(string(output))
		t.Errorf("Unexpected result %d %s", n3, err)
	}
}

const (
	fileaContents = "This is a test file."
	boundary      = `MyBoundary`
)

const message = `
--MyBoundary
Content-Disposition: form-data; name="filea"; filename="filea.txt"
Content-Type: text/plain

` + fileaContents + `
--MyBoundary--
`
