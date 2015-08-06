package helpscout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

var baseURL = "https://api.helpscout.net/v1"

type HelpScout interface {
	CreateConversation(customer Person, subject string, mailbox MailboxRef, threads []*Thread) error
}

type Conversation struct {
	Customer Person
	Subject  string
	Mailbox  MailboxRef
	Threads  []*Thread
}

type Thread struct {
	Typ       string `json:"type"`
	CreatedBy Person
	Body      string
}

type Person struct {
	ID        int64
	Email     string
	Username  string
	FirstName string
	Typ       string `json:"type"`
}

type MailboxRef struct {
	ID   int
	Name string
}

func (c *Conversation) CreateConversation(customer Person, subject string, mailbox MailboxRef, threads []*Thread) error {
	reqURL := fmt.Sprintf("%s/conversations.json", baseURL)

	c = &Conversation{
		Customer: customer,
		Subject:  subject,
		Mailbox:  mailbox,
		Threads:  threads,
	}

	conversationParam, err := json.Marshal(c)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(conversationParam))
	if err != nil {
		return err
	}

	req.SetBasicAuth(envvars.HelpScoutAPIKey, "")
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("CreateConversation: ioutil.ReadAll response body err: %v", err)
		}
		defer res.Body.Close()
		return fmt.Errorf("CreateConversation: Unable to create conversation: expected %d, got %d - %s", http.StatusCreated, res.StatusCode, body)
	}

	return nil
}
