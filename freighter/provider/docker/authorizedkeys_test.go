package docker_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/util"

	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker keymanager", func() {
	var (
		keysPath string
	)

	BeforeEach(func() {
		f, err := ioutil.TempFile("", "keymanager-test")
		Expect(err).To(BeNil())
		keysPath = f.Name()
	})

	AfterEach(func() {
		os.Remove(keysPath)
	})

	addAndCheckKeys := func(ak *docker.AuthorizedKeys, keys ...string) {
		err := ak.Add(keys)
		Expect(err).To(BeNil())

		ks, err := ak.Read()
		Expect(err).To(BeNil())
		Expect(ks).To(ConsistOf(keys))
	}

	It(".Read()", func() {
		pubkey1 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com"
		pubkey2 := strings.TrimSpace(string(test.TestSSHPubKey))
		err := ioutil.WriteFile(keysPath,
			[]byte(fmt.Sprintf("%s\n%s", pubkey1, pubkey2)),
			0644)
		Expect(err).To(BeNil())

		ak := docker.NewAuthorizedKeys(keysPath)
		ks, err := ak.Read()
		Expect(err).To(BeNil())
		Expect(ks).To(ConsistOf([]string{pubkey1, pubkey2}))
	})

	It(".Write()", func() {
		ak := docker.NewAuthorizedKeys(keysPath)
		err := ak.Write([]string{"write", "doesn't", "validate"})
		Expect(err).To(BeNil())

		bs, err := ioutil.ReadFile(keysPath)
		Expect(err).To(BeNil())
		Expect(bs).To(Equal([]byte("write\ndoesn't\nvalidate\n")))
	})

	It(".Add() does not add invalid keys", func() {
		ak := docker.NewAuthorizedKeys(keysPath)
		err := ak.Add([]string{"ssh-rsa S4!#AFLJKN*F@NF12f"})
		Expect(err).To(MatchError(util.ErrInvalidPublicKey))

		ks, err := ak.Read()
		Expect(err).To(BeNil())
		Expect(ks).To(HaveLen(0))
	})

	It(".Add() allows valid keys", func() {
		keys := []string{ // rsa, dsa, ecdsa
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com",
			"ssh-dss AAAAB3NzaC1kc3MAAACBAKPiMSg6vvWZAK1a+Vj+yPH0kRpCDUqhNCyPJd8yIfvs8c+uGgnXkDG6vW3gamyxkFilxgLvqW89NnvmcBRoAXkrT5e8uiciLJ2CqJuqZPHFNf3T298xFleh/43X5dxgS6CdODzrkreFe8XD1R1DozMgChU8wGVpQNQWGKU/x7xXAAAAFQCa8OP3WB4V2mNA/8cN7fj4gxDbLwAAAIEAhxHz0xu/EAWvq+nZ5CXnWZ2g++t1IEJcJWnTTIQakU+aZV0DxQ4hesrFiXllBX/oUTgr8ZlqYgqTy4+lHpXIhesNy3k9OjjU8/dYm9zZZw3TWIL64dAEp9L0xNZ7MFgOVvDwpjNqjUrbDCIujk7iZ+J/hFFoTqRxSeciX+cbusQAAACBAIcGybaGJ2Z4E21jR0iD05kNhJm97CJHavhH6fgxkeecVdXU2+k/DjLHTz7UfhbAtg6XqE0s5PVUgh0P/stQRGyosTmasYE3hTwwOXYQ2k/HLqM/ZGwEXW2kLSpi2MROowQf25wQrWmJBjTQ8mjG5jm0KOCX2jFf0QN3qTzVovSc vagrant@ubuntu-14",
			"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBIC/WqXhoJC0FSaocOrx/uyneeqH32CUNbva4L6PdY7mIZ99mTTpdREowMo1T3pvY46JxwL6X3stuhZmIYNJEc0=",
		}
		ak := docker.NewAuthorizedKeys(keysPath)

		for i := 0; i < 3; i++ { // test duplicate removal
			addAndCheckKeys(ak, keys...)
		}
	})

	Describe(".Remove()", func() {
		It("deletes keys based on fingerprint", func() {
			pubkey1 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com"

			ak := docker.NewAuthorizedKeys(keysPath)
			addAndCheckKeys(ak, test.TestSSHPubKey, pubkey1)

			fprint, err := util.Md5Fingerprint(test.TestSSHPubKey)
			Expect(err).To(BeNil())
			err = ak.Remove(fprint)
			Expect(err).To(BeNil())

			ks, err := ak.Read()
			Expect(err).To(BeNil())
			Expect(ks).To(ConsistOf([]string{pubkey1}))
		})

		It("does not error when fingerprint is not found", func() {
			ak := docker.NewAuthorizedKeys(keysPath)
			addAndCheckKeys(ak, test.TestSSHPubKey)

			err := ak.Remove("nosuch")
			Expect(err).To(BeNil())

			ks, err := ak.Read()
			Expect(err).To(BeNil())
			Expect(ks).To(ConsistOf([]string{test.TestSSHPubKey}))
		})
	})

	Describe(".Clear()", func() {
		It("removes all keys", func() {
			pubkey1 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCxCeR/jJna6xXELDV8HF51QQPbj++JNDE4W/nqLyhzRTEDDnAfyp8gXW/EXXos8w5XXLak6eUDXE/QI6o8blX39RYoT47u45v4b2scN0ZiS5McmESo193KwWH3gXHbnHWywzPy7+GrRF3nHm/uidvp0tqh4ywqIZujaMjST1sIOQ== test@example.com"

			ak := docker.NewAuthorizedKeys(keysPath)
			addAndCheckKeys(ak, test.TestSSHPubKey, pubkey1)

			ks, err := ak.Read()
			Expect(err).To(BeNil())
			Expect(ks).To(HaveLen(2))

			err = ak.Clear()
			Expect(err).To(BeNil())

			ks, err = ak.Read()
			Expect(err).To(BeNil())
			Expect(ks).To(HaveLen(0))
		})

		It("works on non-existent file", func() {
			ak := docker.NewAuthorizedKeys(keysPath)
			err := ak.Clear()
			Expect(err).To(BeNil())

			ks, err := ak.Read()
			Expect(err).To(BeNil())
			Expect(ks).To(HaveLen(0))
		})
	})
})
