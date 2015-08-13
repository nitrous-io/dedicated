package helpscout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

var BaseURL = "https://api.helpscout.net/v1"

type HelpScout interface {
	CreateConversation(customer Person, subject string, mailbox MailboxRef, threads []*Thread) error
}

type Conversation struct {
	Customer Person     `json:"customer"`
	Subject  string     `json:"subject"`
	Mailbox  MailboxRef `json:"mailbox"`
	Threads  []*Thread  `json:"threads"`
}

type Thread struct {
	Type      string `json:"type"`
	CreatedBy Person `json:"createdBy"`
	Body      string `json:"body"`
}

type Person struct {
	ID        int64  `json:"id,omitempty"`
	Email     string `json:"email"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	Type      string `json:"type"`
}

type MailboxRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // Optional
}

func (c *Conversation) CreateConversation(customer Person, subject string, mailbox MailboxRef, threads []*Thread) error {
	reqURL := fmt.Sprintf("%s/conversations.json", BaseURL)

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

	req.SetBasicAuth(envvars.HelpScoutAPIKey, envvars.HelpScoutPassword)
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
		res.Body.Close()
		return fmt.Errorf("CreateConversation: Unable to create conversation: expected %d, got %d - %s", http.StatusCreated, res.StatusCode, body)
	}

	return nil
}

func NewCustomer(email, username, name string) Person {
	person := Person{
		Type:      "customer",
		Email:     email,
		Username:  username, // Optional
		FirstName: name,     // Optional
	}
	return person
}

func NewThread(typ string, author Person, body string) *Thread {
	thread := &Thread{
		Type:      typ,
		CreatedBy: author,
		Body:      body,
	}
	return thread
}
