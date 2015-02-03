package test

import (
	"bytes"
	"fmt"
	"math/rand"
	"mime/multipart"

	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
)

const TestSSHPrivKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEA9CgGtdpQrXmgVT54goMB2Jo9MnxtnpIrxnnnxgav8WIrKm0R
OQIi8XRLmxa4/dVILEp+6uCVdSu050XwqR4FrLJzn+67D5xHLZghhQZpQKhOeldD
3EFmuYFWjNs6uYGuLWAPCpOoz5e5zP7KynfNcN93I/v8dERF6ZAnl5V3gJ+XCyhh
L3ybczyYUoTsmCpxibtHemmcElldhIHfo+4iL8pDMsb6DSUMgvmdFtV6TpmqPO6O
Ixb8cJw2uQnHdT+ehXyBvJcwyhvch8M7YTsxft6K6gdqaCtn6ilw5EBSTt7xG3nu
vhW9bODTTxEVSW6RUUyIOPPM2E/AswzrIolZCwIDAQABAoIBAQCAJDHBZ99H7Lm1
g6AD/3YMlIbh5GQ0AUtwU7LNP90/tuIo1i1f2js0tnCn6PkWX8YNWNvbshs+7712
VELGv7cCuHXuzoDm5IKG731IL+E1yh/GHd7Dzp93ZOZj7vuXOPSNZAkA6BaFPYCK
kn8kXFQ6lU4WWL5a1ypKNxpQeGXL3vAcLGfgC1RI12hVucVkPvaNm58kCJZqFq/h
nAi6mUF4ZNWmgd1eQ48luPkZnjDY/CA7kk4pE+3cyCeeZ3GD530AfreyRUMgiWc0
DdSBDGeqzRgVadAyqsbUHuheIX+Hqiq78HcxmNNdTrbIwPAJ4sfMYFg+1aBZywoB
2c503caBAoGBAPt7G5vyIlrMGky+cRIFf2zxSefp0LphtyCpfjLD+PrMdOvZ6GZ+
Nb779tLlnUD03dVIvFf4iUPxrz5OSH2TgyVyQM62JrLEYqezWlEaoM29iP1+nKay
lbCpXPAUZnHD8EILJvsY/fEj7NnCwgHCa9FGtZVVNQ4jBOooZQqYxeI7AoGBAPiL
OSD14Odl/119+MxyTe7rAtuuqvbOsbQ0cmA5lULiUaJnjNOQmFTfCuVxcNR2i3Hw
KH5OQ1xhHCd+23zTHHo04FxazZNTLvPE+xYMIFLZ61aQHEbB26qUNDirVouZAFmT
aUlPaYMP/F6YMIaC6UD0pb2EOeIrbYXL2v3QFKdxAoGBAMnlaSIHACvTJt4sGsz0
GvxUrxgAcHHiF0qiqM2K5i47zTa4qEr40p903BEvBNPsgthD3+nkgBdLRn0IDky7
DBQZKQyL0n+9CNNIO59XeHgUzZFuf8eZInJFVnbsed6mBxKOr4nx45YgWHvvTyBo
fXTzhr7QPJsP5Fc1fQFH676XAoGAZOmaI3DRhQJbwajxCIlsbRIMUgfEMHtRmXol
6j3XBZYAUTE783Zk8yiwMuJxTkJncbZxqn66SNrA5KRUF4VbrZ6kbbsMdsGycZVL
4PLBeQIz1Cyk3nG9b6/G2C9w7zMK+lOozvQoQeeOrxWZtjOW4yGx3ZW/Lr41NLrF
SoJesHECgYEAx+LRS4tMB2x+26AMzguKWJIgjy9oddnf8VpmGqnTmcCytX1wb3Kc
528LIeP0lhmg2jAJCVIfT9USpx1bwuUE3Nhd3U8/LIv48BEspGjF7uQo0pcunlD8
s2l0IfX0aE2PdZH6vueV/otr9L+1GoVuVu/IQ8+MFLKDo2b4Qf0JTmY=
-----END RSA PRIVATE KEY-----`

const TestSSHPubKey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD0KAa12lCteaBVPniCgwHYmj0yfG2ekivGeefGBq/xYisqbRE5AiLxdEubFrj91UgsSn7q4JV1K7TnRfCpHgWssnOf7rsPnEctmCGFBmlAqE56V0PcQWa5gVaM2zq5ga4tYA8Kk6jPl7nM/srKd81w33cj+/x0REXpkCeXlXeAn5cLKGEvfJtzPJhShOyYKnGJu0d6aZwSWV2Egd+j7iIvykMyxvoNJQyC+Z0W1XpOmao87o4jFvxwnDa5Ccd1P56FfIG8lzDKG9yHwzthOzF+3orqB2poK2fqKXDkQFJO3vEbee6+Fb1s4NNPERVJbpFRTIg488zYT8CzDOsiiVkL test@example.com`

const NitrousImage = "nitrousio/ubuntu-bare:latest"

// dependent on ENV declarations in nitrousio/ubuntu-bare Dockerfile
const (
	DfileEnvUser     = "nitrous"                         // NITROUS_USERNAME
	DfileEnvWorkPath = "/home/" + DfileEnvUser + "/code" // NITROUS_APP_PATH parent
)

func NewRequestWithSecret(secret string, method string, urlStr string, params *url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Freighter-Secret", secret)
	return req, nil
}

func NewRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func NewFormRequest(secret string, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Freighter-Secret", secret)
	return req, err
}

func NewFileRequest(secret string, method string, urlStr string, params map[string]string, files map[string]string) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, contents := range files {
		part, err := writer.CreateFormFile(key, fmt.Sprintf("%s.txt", key))
		if err != nil {
			return nil, err
		}
		part.Write([]byte(contents))
	}

	for key, val := range params {
		_ = writer.WriteField(key, val)
	}

	err := writer.Close()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Freighter-Secret", secret)
	return req, nil
}

func TempFile(extension string) string {
	randomName := RandomName("freightertest")
	if extension == "" {
		return path.Join(os.TempDir(), randomName)
	}
	return path.Join(os.TempDir(), randomName+"."+extension)
}

func TempDir() (string, error) {
	dir := path.Join(os.TempDir(), RandomName("freightertest"))
	err := os.Mkdir(dir, 0755)
	if err != nil {
		return "", err
	}
	return dir, nil
}

func RandomName(name string) string {
	return fmt.Sprintf("%v-%v-%v", name, time.Now().UnixNano(), rand.Intn(1000))
}

var pulledImages = make(map[string]struct{})

func PullImage(name string) {
	if v, _ := strconv.ParseBool(os.Getenv("TEST_SKIP_PULL_IMAGES")); v {
		return
	}
	if _, ok := pulledImages[name]; ok {
		return
	}
	start := time.Now()
	fmt.Printf("Pulling %s... ", name)
	err := exec.Command("docker", "pull", name).Run()
	fmt.Printf("Pulled after %.1fs\n", time.Since(start).Seconds())
	Expect(err).To(BeNil(), "failed to pull %s image", name)
	pulledImages[name] = struct{}{}
}

// createImages writes a dockerfile from contents and
// builds a docker image from it, returning the name of the image.
// If contents is zero value, it uses a default dockerfile.
func CreateImages(count int, name string, noEntropy bool, contents string) []string {
	dockerLoc, err := ioutil.TempDir("", "DockerTest")
	Expect(err).To(BeNil())
	dockerFile, err := os.Create(filepath.Join(dockerLoc, "Dockerfile"))
	Expect(err).To(BeNil())

	entropy := name
	if noEntropy {
		entropy = "thing"
	}

	if contents == "" {
		contents = fmt.Sprintf(`
				FROM %s
				MAINTAINER Nitrous.IO, hello@nitrous.io
				RUN echo '%s' >> /tmp/info
				CMD /bin/bash
			`, NitrousImage, entropy)
	}
	dockerFile.WriteString(contents)
	dockerFile.Close()

	list := []string{}
	for index := 1; index <= count; index++ {
		findableName := fmt.Sprintf("%v:n%v", name, index)
		out, err := exec.Command("docker", "build", "-t", findableName, dockerLoc).CombinedOutput()
		Expect(err).To(BeNil(), "Failed to build image, output: %s", string(out))
		list = append(list, findableName)
	}

	err = os.RemoveAll(dockerLoc)
	Expect(err).To(BeNil())

	return list
}

func RemoveImages(names []string) {
	args := append([]string{"rmi", "-f"}, names...)
	out, err := exec.Command("docker", args...).CombinedOutput()
	Expect(err).To(BeNil(), "Failed to remove images, output: %v", string(out))
}

// CleanUpContainer removes the container in a best effort way. It does not
// fail when the container cant be removed
func CleanUpContainer(name string) {
	exec.Command("docker", "stop", "--time=1", name).Run()
	exec.Command("docker", "rm", "-f", name).Run()
}
