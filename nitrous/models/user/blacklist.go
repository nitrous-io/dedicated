package user

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

const delimiter = "@"

var blacklist map[string]struct{}

func IsBlacklisted(email string) (bool, error) {
	index := strings.Index(email, delimiter)
	if index < 0 {
		return true, fmt.Errorf("Email address(%s) does not contain @", email)
	}

	domain := string(email[index+1:])
	if _, ok := blacklist[domain]; ok {
		return true, nil
	}
	return false, nil
}

func fetchBlacklist() error {
	filepath := envvars.EmailBlacklistPath
	if filepath == "" {
		return fmt.Errorf("email blacklist file path is not specified")
	}

	file, err := os.Open(filepath)
	if os.IsNotExist(err) {
		return fmt.Errorf("email blacklist file does not exist: %s", filepath)
	}

	if err != nil {
		return fmt.Errorf("error opening email blacklist file: %s", err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	var domain string
	blacklist = make(map[string]struct{})
	for scanner.Scan() {
		domain = scanner.Text()
		blacklist[domain] = struct{}{}
	}
	if err == io.EOF && domain != "" {
		blacklist[domain] = struct{}{}
	}

	return nil
}

func init() {
	err := fetchBlacklist()
	if err != nil {
		log.Printf("Failed to initalize email black list: %v", err)
	}
}
