package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	run()
}

func run() {
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	if awsAccessKey == "" {
		log.Fatalf("AWS_ACCESS_KEY_ID is required.")
	}
	awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if awsSecretKey == "" {
		log.Fatalf("AWS_SECRET_ACCESS_KEY is required.")
	}
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		log.Fatalf("AWS_REGION is required.")
	}
	s3Bucket := os.Getenv("BUCKET")
	if s3Bucket == "" {
		log.Fatalf("BUCKET is required.")
	}
	containerName := os.Getenv("CONTAINER")
	if containerName == "" {
		log.Fatalf("CONTAINER is required")
	}
	containerHash := os.Getenv("CONTAINER_HASH")
	if containerHash == "" {
		log.Fatalf("CONTAINER_HASH is required")
	}

	// Download gof3r binary - this is a S3 client CLI tool.
	s3BinFile, err := ioutil.TempFile("", "gof3r")
	if err != nil {
		log.Fatalf("error creating temp file for gof3r: %v", err)
	}
	defer os.Remove(s3BinFile.Name())

	const s3BinURL = "https://s3.amazonaws.com/nitrous-container-backups/gof3r"
	resp, err := http.Get(s3BinURL)
	if err != nil {
		log.Fatalf("error making HTTP GET to gof3r URL: %v", err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(s3BinFile, resp.Body); err != nil {
		log.Fatalf("error copying HTTP response to temp gof3r file: %v", err)
	}

	if err := s3BinFile.Close(); err != nil {
		log.Fatalf("error closing temp gof3r file: %v", err)
	}

	if err := os.Chmod(s3BinFile.Name(), 0755); err != nil {
		log.Fatalf("error chmod-ing temp gof3r file: %v", err)
	}

	// gzip writes to pipe, gof3r reads from pipe.
	pr, pw := io.Pipe()
	gw := gzip.NewWriter(pw)

	cpCmd := exec.Command("docker", "cp", containerName+":/home/nitrous", "-")
	cpCmd.Stdout = gw

	var cpStderr bytes.Buffer
	cpCmd.Stderr = &cpStderr

	s3Key := containerHash + ".tar.gz"
	s3Cmd := exec.Command(s3BinFile.Name(), "put",
		"--bucket", s3Bucket,
		"--key", s3Key,
		"--endpoint", "s3-accelerate.amazonaws.com",
		"--acl", "public-read",
		"--no-md5")
	s3Cmd.Stdin = pr

	var s3Stderr bytes.Buffer
	s3Cmd.Stderr = &s3Stderr

	if err := cpCmd.Start(); err != nil {
		log.Fatalf("error starting docker cp: %v", err)
	}

	if err := s3Cmd.Start(); err != nil {
		log.Fatalf("error starting gof3r: %v", err)
	}

	if err := cpCmd.Wait(); err != nil {
		log.Fatalf("error waiting for docker cp to exit: %v, stderr: %s", err, cpStderr.String())
	}

	gw.Close()
	pw.Close()

	if err := s3Cmd.Wait(); err != nil {
		log.Fatalf("error waiting for gof3r to exit: %v, stderr: %s", err, s3Stderr.String())
	}
}
